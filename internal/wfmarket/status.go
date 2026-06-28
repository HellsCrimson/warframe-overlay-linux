package wfmarket

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// wsURL is the warframe.market realtime endpoint (var so tests can repoint it).
var wsURL = "wss://ws.warframe.market/socket"

// warframe.market only advertises a presence status while the user holds a live
// websocket connection — there is no REST endpoint. SetStatus therefore opens a
// persistent authenticated socket, pushes the desired status, and a keepalive
// goroutine re-asserts it (reconnecting if the socket drops) until CloseStatus.

// SetStatus sets the user's presence (online | ingame | invisible) and keeps it
// active. The first call opens the websocket and starts the keepalive loop;
// later calls update the desired status over the existing connection.
func (c *Client) SetStatus(sess *Session, status string) error {
	if sess == nil || sess.Token == "" {
		return fmt.Errorf("wfmarket: not signed in")
	}
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	if c.statusToken != sess.Token {
		c.closeStatusLocked() // a new login invalidates the old connection
		c.statusToken = sess.Token
	}
	c.statusDesired = status
	if !c.statusKeepalive {
		c.statusKeepalive = true
		go c.statusKeepaliveLoop()
	}
	slog.Info("wfmarket: setting presence status", "status", status)
	return c.ensureStatusLocked(status)
}

// CloseStatus drops the presence connection, which makes the account appear
// offline. Used on logout and shutdown.
func (c *Client) CloseStatus() {
	c.statusMu.Lock()
	c.statusDesired = ""
	c.statusToken = ""
	c.closeStatusLocked()
	c.statusMu.Unlock()
}

// statusKeepaliveLoop periodically re-asserts the desired status, which doubles
// as a keepalive and transparently reconnects if the socket dropped. It runs for
// the lifetime of the client; it no-ops while no status is desired.
func (c *Client) statusKeepaliveLoop() {
	t := time.NewTicker(90 * time.Second)
	defer t.Stop()
	for range t.C {
		c.statusMu.Lock()
		if c.statusDesired != "" && c.statusToken != "" {
			_ = c.ensureStatusLocked(c.statusDesired)
		}
		c.statusMu.Unlock()
	}
}

// ensureStatusLocked makes sure a connection exists and writes the status,
// redialing once if the existing connection has gone stale. Caller holds statusMu.
func (c *Client) ensureStatusLocked(status string) error {
	if c.statusConn == nil {
		if err := c.dialStatusLocked(c.statusToken); err != nil {
			return err
		}
	}
	if err := c.writeStatusLocked(status); err != nil {
		c.closeStatusLocked()
		if err := c.dialStatusLocked(c.statusToken); err != nil {
			return err
		}
		return c.writeStatusLocked(status)
	}
	return nil
}

// dialStatusLocked opens the websocket and authenticates it. warframe.market's
// websocket is authenticated NOT via the handshake but by sending a
// @wfm|cmd/auth/signIn command with the v1 JWT and awaiting :ok. Caller holds
// statusMu.
func (c *Client) dialStatusLocked(token string) error {
	if token == "" {
		return fmt.Errorf("wfmarket: not signed in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{"wfm"},
		HTTPHeader:   http.Header{"User-Agent": {userAgent}},
	})
	if err != nil {
		return fmt.Errorf("wfmarket: ws dial: %w", err)
	}
	conn.SetReadLimit(1 << 20)
	if err := authWS(ctx, conn, token); err != nil {
		conn.Close(websocket.StatusPolicyViolation, "auth failed")
		return err
	}
	c.statusConn = conn
	slog.Debug("wfmarket: presence websocket connected & authenticated")
	go c.readStatusLoop(conn) // drain server messages so ping/pong is handled
	return nil
}

// authWS performs the websocket sign-in handshake: send the JWT, then read until
// the auth result arrives (skipping unrelated broadcasts) or the timeout fires.
func authWS(ctx context.Context, conn *websocket.Conn, token string) error {
	msg, _ := json.Marshal(map[string]any{
		"route":   "@wfm|cmd/auth/signIn",
		"id":      "wfo-auth",
		"payload": map[string]any{"token": token},
	})
	if err := conn.Write(ctx, websocket.MessageText, msg); err != nil {
		return fmt.Errorf("wfmarket: ws auth send: %w", err)
	}
	rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		_, data, err := conn.Read(rctx)
		if err != nil {
			return fmt.Errorf("wfmarket: ws auth read: %w", err)
		}
		var m struct {
			Route string `json:"route"`
		}
		_ = json.Unmarshal(data, &m)
		switch {
		case m.Route == "@wfm|cmd/auth/signIn:ok":
			return nil
		case strings.HasPrefix(m.Route, "@wfm|cmd/auth/signIn:"),
			strings.Contains(m.Route, "error"), strings.Contains(m.Route, "protect"):
			return fmt.Errorf("wfmarket: ws auth rejected: %s", truncate(data, 300))
			// Anything else (e.g. @wfm|event/reports/online broadcasts): keep reading.
		}
	}
}

// writeStatusLocked sends a status/set command. Caller holds statusMu (so writes
// are serialized, as the websocket requires). The reader goroutine handles reads
// concurrently.
func (c *Client) writeStatusLocked(status string) error {
	msg, _ := json.Marshal(map[string]any{
		"route":   "@wfm|cmd/status/set",
		"id":      "wfo-status",
		"payload": map[string]any{"status": status},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.statusConn.Write(ctx, websocket.MessageText, msg); err != nil {
		return fmt.Errorf("wfmarket: ws status: %w", err)
	}
	return nil
}

func (c *Client) closeStatusLocked() {
	if c.statusConn != nil {
		_ = c.statusConn.Close(websocket.StatusNormalClosure, "")
		c.statusConn = nil
	}
}

// readStatusLoop drains incoming frames (replies, broadcasts, pings) until the
// connection errors, then clears it so the next keepalive tick reconnects. It
// logs server error routes (e.g. @wfm|protect/error → app.errors.userNotVerified)
// so a silently-rejected status command is visible.
func (c *Client) readStatusLoop(conn *websocket.Conn) {
	for {
		_, data, err := conn.Read(context.Background())
		if err != nil {
			c.statusMu.Lock()
			if c.statusConn == conn {
				c.statusConn = nil
			}
			c.statusMu.Unlock()
			return
		}
		var m struct {
			Route string `json:"route"`
		}
		_ = json.Unmarshal(data, &m)
		switch {
		case strings.Contains(m.Route, "error"), strings.Contains(m.Route, "protect"):
			slog.Warn("wfmarket: presence websocket server error", "route", m.Route, "msg", truncate(data, 400))
		default:
			slog.Debug("wfmarket: presence websocket message", "route", m.Route)
		}
	}
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n])
	}
	return string(b)
}
