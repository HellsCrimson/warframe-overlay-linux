package inventory

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// inventoryURL is DE's official mobile inventory endpoint (the same one the game
// client uses).
const inventoryURL = "https://mobile.warframe.com/api/inventory.php"

// FetchRaw downloads the raw inventory JSON for the given auth.
func FetchRaw(ctx context.Context, auth Auth) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, inventoryURL+auth.Query(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("inventory api: status %d: %s", resp.StatusCode, snippet(body))
	}
	return body, nil
}

func snippet(b []byte) string {
	if len(b) > 200 {
		return string(b[:200])
	}
	return string(b)
}
