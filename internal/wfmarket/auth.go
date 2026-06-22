package wfmarket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
)

// Session is an authenticated warframe.market session.
type Session struct {
	Token    string // JWT for the Authorization header
	UserName string // in-game name ( for display)
}

// Login authenticates with email + password and returns a session, following
// warframe.market's flow: GET the API to obtain a JWT cookie, then POST that as
// the Authorization header to /auth/signin; the response's Authorization header
// is the session token.
func (c *Client) Login(email, password string) (*Session, error) {
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Timeout: c.http.Timeout, Jar: jar}

	// 1. Bootstrap a JWT cookie.
	bootReq, _ := http.NewRequest(http.MethodGet, authBaseURL, nil)
	marketHeaders(bootReq)
	bootResp, err := hc.Do(bootReq)
	if err != nil {
		return nil, fmt.Errorf("wfmarket: bootstrap: %w", err)
	}
	bootResp.Body.Close()
	jwt := ""
	for _, ck := range bootResp.Cookies() {
		if ck.Name == "JWT" {
			jwt = ck.Value
		}
	}

	// 2. Sign in.
	body, _ := json.Marshal(map[string]any{
		"email":     email,
		"password":  password,
		"auth_type": "header",
	})
	req, _ := http.NewRequest(http.MethodPost, authBaseURL+"/auth/signin", bytes.NewReader(body))
	marketHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	if jwt != "" {
		req.Header.Set("Authorization", jwt)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wfmarket: signin: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wfmarket: signin failed (%d): %s", resp.StatusCode, apiError(raw))
	}
	token := resp.Header.Get("Authorization")
	if token == "" {
		// Some responses deliver it as a Set-Cookie JWT instead.
		for _, ck := range resp.Cookies() {
			if ck.Name == "JWT" {
				token = "JWT " + ck.Value
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

// AddSellOrder posts a visible sell order for an item id at the given price.
func (c *Client) AddSellOrder(sess *Session, itemID string, platinum, quantity int) error {
	if sess == nil || sess.Token == "" {
		return fmt.Errorf("wfmarket: not signed in")
	}
	body, _ := json.Marshal(map[string]any{
		"item":       itemID,
		"order_type": "sell",
		"platinum":   platinum,
		"quantity":   quantity,
		"visible":    true,
		"rank":       0,
	})
	req, _ := http.NewRequest(http.MethodPost, ordersURLVar, bytes.NewReader(body))
	marketHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", sess.Token)
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
		Error   string                 `json:"error"`
		Payload map[string]interface{} `json:"payload"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != "" {
		return e.Error
	}
	if len(raw) > 200 {
		raw = raw[:200]
	}
	return string(raw)
}
