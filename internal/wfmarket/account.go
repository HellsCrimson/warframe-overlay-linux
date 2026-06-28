package wfmarket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Endpoint URLs for authenticated account actions (vars so tests can repoint
// them at a local server).
var (
	myOrdersURLVar   = "https://api.warframe.market/v2/orders/my"
	closeOrderURLVar = "https://api.warframe.market/v2/order/%s/close"
)

// MyOrder is one of the authenticated user's own orders.
type MyOrder struct {
	ID       string
	ItemID   string
	Type     string // "sell" | "buy"
	Platinum int
	Quantity int
}

// MyOrders returns the authenticated user's current orders.
func (c *Client) MyOrders(sess *Session) ([]MyOrder, error) {
	if sess == nil || sess.Token == "" {
		return nil, fmt.Errorf("wfmarket: not signed in")
	}
	c.throttle()
	req, _ := http.NewRequest(http.MethodGet, myOrdersURLVar, nil)
	req.Header.Set("Authorization", "Bearer "+sess.Token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wfmarket: my orders (%d): %s", resp.StatusCode, apiError(raw))
	}
	var r struct {
		Data []struct {
			ID       string `json:"id"`
			ItemID   string `json:"itemId"`
			Type     string `json:"type"`
			Platinum int    `json:"platinum"`
			Quantity int    `json:"quantity"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	out := make([]MyOrder, 0, len(r.Data))
	for _, o := range r.Data {
		out = append(out, MyOrder{ID: o.ID, ItemID: o.ItemID, Type: o.Type, Platinum: o.Platinum, Quantity: o.Quantity})
	}
	return out, nil
}

// CloseOrder records a sale of qty units of an order. Per the v2 API, a partial
// close lowers the remaining quantity; a full close removes the order.
func (c *Client) CloseOrder(sess *Session, orderID string, qty int) error {
	if sess == nil || sess.Token == "" {
		return fmt.Errorf("wfmarket: not signed in")
	}
	c.throttle()
	body, _ := json.Marshal(map[string]any{"quantity": qty})
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf(closeOrderURLVar, orderID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sess.Token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("wfmarket: close order (%d): %s", resp.StatusCode, apiError(raw))
	}
	return nil
}
