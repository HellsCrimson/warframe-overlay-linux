package wfmarket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
)

// Session is an authenticated warframe.market session.
type Session struct {
	Token    string // raw JWT (no scheme prefix)
	UserName string // in-game name (for display)
}

// Login authenticates with email + password and returns a session.
//
// warframe.market still uses the v1 signin flow (OAuth 2.0 is not yet public),
// guarded by a double-submit CSRF check: the anonymous JWT issued as a cookie
// must ALSO be echoed in the Authorization header. The flow is therefore:
//  1. GET a v1 endpoint to obtain the anonymous JWT cookie (set by middleware
//     even on a 404), captured by the cookie jar.
//  2. POST /auth/signin with that cookie (resent by the jar) AND the same token
//     as "Authorization: JWT <jwt>", auth_type=header.
//
// The authenticated token comes back in the Authorization response header (or a
// refreshed JWT cookie) and is stored raw, to be presented as a Bearer token to
// the v2 order endpoint.
func (c *Client) Login(email, password string) (*Session, error) {
	jar, _ := cookiejar.New(nil)
	// Reuse the identifying transport (User-Agent etc.) for the login flow too.
	hc := &http.Client{Timeout: c.http.Timeout, Jar: jar, Transport: c.http.Transport}

	// 1. Bootstrap the anonymous JWT cookie (set by middleware on any v1 path).
	bootReq, _ := http.NewRequest(http.MethodGet, authBaseURL, nil)
	marketHeaders(bootReq)
	bootResp, err := hc.Do(bootReq)
	if err != nil {
		return nil, fmt.Errorf("wfmarket: bootstrap: %w", err)
	}
	bootResp.Body.Close()
	jwt := ""
	for _, ck := range jar.Cookies(bootReq.URL) {
		if ck.Name == "JWT" {
			jwt = ck.Value
		}
	}
	if jwt == "" {
		return nil, fmt.Errorf("wfmarket: bootstrap returned no JWT cookie")
	}

	// 2. Sign in. The jar resends the JWT cookie; the Authorization header
	// echoes it to satisfy the CSRF check.
	body, _ := json.Marshal(map[string]any{
		"email":     email,
		"password":  password,
		"auth_type": "header",
	})
	req, _ := http.NewRequest(http.MethodPost, authBaseURL+"/auth/signin", bytes.NewReader(body))
	marketHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "JWT "+jwt)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wfmarket: signin: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wfmarket: signin failed (%d): %s", resp.StatusCode, apiError(raw))
	}

	token := jwtToken(resp.Header.Get("Authorization"))
	if token == "" {
		// Some responses deliver the authenticated token as a refreshed cookie.
		for _, ck := range jar.Cookies(req.URL) {
			if ck.Name == "JWT" && ck.Value != jwt {
				token = ck.Value
			}
		}
	}
	if token == "" {
		return nil, fmt.Errorf("wfmarket: signin returned no token")
	}

	var sr struct {
		Payload struct {
			User struct {
				IngameName string `json:"ingame_name"`
			} `json:"user"`
		} `json:"payload"`
	}
	_ = json.Unmarshal(raw, &sr)
	return &Session{Token: token, UserName: sr.Payload.User.IngameName}, nil
}

// jwtToken strips a "JWT "/"Bearer " scheme prefix, returning the raw token.
func jwtToken(authHeader string) string {
	t := strings.TrimSpace(authHeader)
	for _, scheme := range []string{"JWT ", "Bearer "} {
		if len(t) >= len(scheme) && strings.EqualFold(t[:len(scheme)], scheme) {
			return strings.TrimSpace(t[len(scheme):])
		}
	}
	return t
}

// AddSellOrder posts a visible sell order for an item id at the given price,
// via the v2 order endpoint with the session JWT as a Bearer token.
func (c *Client) AddSellOrder(sess *Session, itemID string, platinum, quantity int) error {
	if sess == nil || sess.Token == "" {
		return fmt.Errorf("wfmarket: not signed in")
	}
	body, _ := json.Marshal(map[string]any{
		"itemId":   itemID,
		"type":     "sell",
		"platinum": platinum,
		"quantity": quantity,
		"visible":  true,
	})
	req, _ := http.NewRequest(http.MethodPost, ordersURLVar, bytes.NewReader(body))
	marketHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sess.Token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("wfmarket: add order (%d): %s", resp.StatusCode, apiError(raw))
	}
	return nil
}

func marketHeaders(req *http.Request) {
	req.Header.Set("platform", "pc")
	req.Header.Set("language", "en")
	req.Header.Set("Accept", "application/json")
}

// apiError extracts warframe.market's error message from a response body.
func apiError(raw []byte) string {
	var e struct {
		Error   string         `json:"error"`
		Payload map[string]any `json:"payload"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != "" {
		return e.Error
	}
	if len(raw) > 200 {
		raw = raw[:200]
	}
	return string(raw)
}
