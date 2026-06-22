// Package wfmarket is a small warframe.market client: it maps item names to
// market slugs and fetches live prices, with light rate limiting and on-disk
// caching of the (large, slow-changing) item list.
package wfmarket

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// minInterval keeps us well under warframe.market's rate limit (~3 req/s).
const minInterval = 350 * time.Millisecond

// Endpoint URLs (vars so tests can point them at a local server).
var (
	itemsURLVar  = "https://api.warframe.market/v2/items"
	statsURLVar  = "https://api.warframe.market/v1/items/%s/statistics"
	authBaseURL  = "https://api.warframe.market/v1"
	ordersURLVar = "https://api.warframe.market/v1/profile/orders"
	// publicOrdersURLVar lists all public buy/sell orders for an item slug.
	publicOrdersURLVar = "https://api.warframe.market/v1/items/%s/orders"
)

// iconBase prefixes warframe.market relative image paths.
const iconBase = "https://warframe.market/static/assets/"

// marketItem holds the identifiers we need for an item.
type marketItem struct {
	ID    string
	Slug  string
	Thumb string
}

// Client talks to warframe.market.
type Client struct {
	http     *http.Client
	cacheDir string

	mu         sync.Mutex
	itemByName map[string]marketItem // normalized name -> {id, slug}
	lastReq    time.Time
}

// New returns a client caching the item list under cacheDir.
func New(cacheDir string) *Client {
	return &Client{http: &http.Client{Timeout: 25 * time.Second}, cacheDir: cacheDir}
}

type itemsResp struct {
	Data []struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
		I18n struct {
			En struct {
				Name  string `json:"name"`
				Thumb string `json:"thumb"`
			} `json:"en"`
		} `json:"i18n"`
	} `json:"data"`
}

// LoadItems fetches (or reads cached) the item list and builds the name->slug
// index. Safe to call repeatedly; it loads at most once.
func (c *Client) LoadItems() error {
	c.mu.Lock()
	already := c.itemByName != nil
	c.mu.Unlock()
	if already {
		return nil
	}

	raw, err := c.cachedItems()
	if err != nil {
		return err
	}
	var resp itemsResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("wfmarket: decode items: %w", err)
	}
	idx := make(map[string]marketItem, len(resp.Data))
	for _, it := range resp.Data {
		if it.Slug != "" && it.I18n.En.Name != "" {
			idx[normalize(it.I18n.En.Name)] = marketItem{ID: it.ID, Slug: it.Slug, Thumb: it.I18n.En.Thumb}
		}
	}
	c.mu.Lock()
	c.itemByName = idx
	c.mu.Unlock()
	return nil
}

func (c *Client) cachedItems() ([]byte, error) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(c.cacheDir, "wfmarket-items.json")
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < 7*24*time.Hour {
		return os.ReadFile(path)
	}
	resp, err := c.http.Get(itemsURLVar)
	if err != nil {
		// Fall back to any stale cache.
		if data, rerr := os.ReadFile(path); rerr == nil {
			return data, nil
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wfmarket: items status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(path, data, 0o644)
	return data, nil
}

// Slug returns the market slug for an item display name, if known.
func (c *Client) Slug(name string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.itemByName[normalize(name)]
	return it.Slug, ok
}

// ItemID returns the warframe.market internal item id for a display name.
func (c *Client) ItemID(name string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.itemByName[normalize(name)]
	return it.ID, ok && it.ID != ""
}

// IconURL returns the thumbnail URL for an item display name, or "".
func (c *Client) IconURL(name string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if it, ok := c.itemByName[normalize(name)]; ok && it.Thumb != "" {
		return iconBase + it.Thumb
	}
	return ""
}

type statsResp struct {
	Payload struct {
		StatisticsClosed struct {
			H48 []struct {
				Median   float64 `json:"median"`
				AvgPrice float64 `json:"avg_price"`
				MinPrice float64 `json:"min_price"`
			} `json:"48hours"`
		} `json:"statistics_closed"`
	} `json:"payload"`
}

// Price returns the most recent 48-hour median sell price for a market slug.
func (c *Client) Price(slug string) (int, error) {
	c.throttle()
	resp, err := c.http.Get(fmt.Sprintf(statsURLVar, slug))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("wfmarket: stats %s status %d", slug, resp.StatusCode)
	}
	var s statsResp
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return 0, err
	}
	h := s.Payload.StatisticsClosed.H48
	if len(h) == 0 {
		return 0, fmt.Errorf("wfmarket: no recent stats for %s", slug)
	}
	return int(h[len(h)-1].Median + 0.5), nil
}

// PriceByName resolves the slug and fetches the price for a display name.
func (c *Client) PriceByName(name string) (int, error) {
	slug, ok := c.Slug(name)
	if !ok {
		return 0, fmt.Errorf("wfmarket: no slug for %q", name)
	}
	return c.Price(slug)
}

// Order is one public sell listing for an item.
type Order struct {
	Seller   string
	Platinum int
	Quantity int
	Status   string // "ingame" | "online" | "offline"
}

type ordersResp struct {
	Payload struct {
		Orders []struct {
			Platinum  int    `json:"platinum"`
			Quantity  int    `json:"quantity"`
			OrderType string `json:"order_type"`
			Visible   bool   `json:"visible"`
			User      struct {
				IngameName string `json:"ingame_name"`
				Status     string `json:"status"`
			} `json:"user"`
		} `json:"orders"`
	} `json:"payload"`
}

// SellOrders returns public sell listings for an item display name — online
// sellers first, then cheapest first — capped at limit (0 = no cap).
func (c *Client) SellOrders(name string, limit int) ([]Order, error) {
	if err := c.LoadItems(); err != nil {
		return nil, err
	}
	slug, ok := c.Slug(name)
	if !ok {
		return nil, fmt.Errorf("wfmarket: no slug for %q", name)
	}
	c.throttle()
	resp, err := c.http.Get(fmt.Sprintf(publicOrdersURLVar, slug))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wfmarket: orders %s status %d", slug, resp.StatusCode)
	}
	var o ordersResp
	if err := json.NewDecoder(resp.Body).Decode(&o); err != nil {
		return nil, err
	}
	out := make([]Order, 0, len(o.Payload.Orders))
	for _, ord := range o.Payload.Orders {
		if ord.OrderType != "sell" || !ord.Visible || ord.User.IngameName == "" {
			continue
		}
		out = append(out, Order{
			Seller: ord.User.IngameName, Platinum: ord.Platinum,
			Quantity: ord.Quantity, Status: ord.User.Status,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ri, rj := statusRank(out[i].Status), statusRank(out[j].Status); ri != rj {
			return ri < rj
		}
		return out[i].Platinum < out[j].Platinum
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// statusRank orders sellers by how reachable they are: in-game, then online,
// then offline.
func statusRank(s string) int {
	switch s {
	case "ingame":
		return 0
	case "online":
		return 1
	default:
		return 2
	}
}

func (c *Client) throttle() {
	c.mu.Lock()
	wait := minInterval - time.Since(c.lastReq)
	c.lastReq = time.Now().Add(max(0, wait))
	c.mu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
}

// normalize lowercases and strips non-alphanumerics for name matching.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
