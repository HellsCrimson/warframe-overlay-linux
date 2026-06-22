// Package wfdata provides Warframe item metadata — the canonical name for each
// internal item type ("uniqueName"), plus which items are masterable. It maps
// the inventory's internal ItemType paths to display names and underpins the
// mastery view.
//
// Data comes from the community warframestat.us items API, cached to disk and
// stale-served when the network is unavailable.
package wfdata

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// itemsURLVar returns just the fields we need. Components are included so the
// mastery view can compute how many of an item's parts the player owns;
// imageName drives item thumbnails. It is a var so tests can point it at a local
// server.
var itemsURLVar = "https://api.warframestat.us/items/?only=uniqueName,name,masterable,productCategory,type,components,imageName"

// imageBase serves item images (redirects to the warframe-items CDN).
const imageBase = "https://cdn.warframestat.us/img/"

// Component is one ingredient of a buildable item's recipe.
type Component struct {
	Name       string `json:"name"`
	UniqueName string `json:"uniqueName"`
	ItemCount  int    `json:"itemCount"`
}

// IsPart reports whether this component is an acquirable item part (a relic/quest
// drop or buildable component) rather than a bulk resource. Resources live under
// /Lotus/Types/Items/; parts live under recipe/weapon paths.
func (c Component) IsPart() bool {
	return c.UniqueName != "" && !strings.Contains(c.UniqueName, "/Types/Items/")
}

// Item is the metadata for one game item.
type Item struct {
	UniqueName      string      `json:"uniqueName"`
	Name            string      `json:"name"`
	Masterable      bool        `json:"masterable"`
	ProductCategory string      `json:"productCategory"`
	Type            string      `json:"type"`
	ImageName       string      `json:"imageName"`
	Components      []Component `json:"components"`
}

// DB is a queryable item-metadata set.
type DB struct {
	byUnique map[string]Item
	byName   map[string]Item // normalized display name -> item
	all      []Item
}

// ImageURL returns the thumbnail URL for an internal item type, or "".
func (d *DB) ImageURL(uniqueName string) string {
	if d == nil {
		return ""
	}
	if it, ok := d.byUnique[uniqueName]; ok && it.ImageName != "" {
		return imageBase + it.ImageName
	}
	return ""
}

// ImageURLByName returns the thumbnail URL for a display name, or "".
func (d *DB) ImageURLByName(name string) string {
	if d == nil {
		return ""
	}
	if it, ok := d.byName[normalizeName(name)]; ok && it.ImageName != "" {
		return imageBase + it.ImageName
	}
	return ""
}

func normalizeName(s string) string {
	var b []rune
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			b = append(b, r+32)
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b = append(b, r)
		}
	}
	return string(b)
}

// Options configures Load.
type Options struct {
	CacheDir   string
	TTL        time.Duration
	HTTPClient *http.Client
	Logger     *slog.Logger
}

// Load fetches (or reads cached) item metadata, refreshing when the cache is
// older than TTL and stale-serving on network failure.
func Load(opts Options) (*DB, error) {
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.TTL == 0 {
		opts.TTL = 7 * 24 * time.Hour // item data changes rarely
	}
	if err := os.MkdirAll(opts.CacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	raw, err := fetchCached(opts, "items.json", itemsURLVar)
	if err != nil {
		return nil, fmt.Errorf("load items: %w", err)
	}
	var items []Item
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("decode items: %w", err)
	}
	db := &DB{byUnique: make(map[string]Item, len(items)), byName: make(map[string]Item, len(items)), all: items}
	for _, it := range items {
		if it.UniqueName != "" {
			db.byUnique[it.UniqueName] = it
		}
		if it.Name != "" {
			db.byName[normalizeName(it.Name)] = it
		}
	}
	opts.Logger.Info("item metadata loaded", "items", len(items))
	return db, nil
}

// Name returns the canonical display name for an internal item type, and whether
// it was found.
func (d *DB) Name(uniqueName string) (string, bool) {
	if d == nil {
		return "", false
	}
	it, ok := d.byUnique[uniqueName]
	if !ok || it.Name == "" {
		return "", false
	}
	return it.Name, true
}

// Masterable returns every masterable item (warframes, weapons, companions, …).
func (d *DB) Masterable() []Item {
	if d == nil {
		return nil
	}
	var out []Item
	for _, it := range d.all {
		if it.Masterable {
			out = append(out, it)
		}
	}
	return out
}

func fetchCached(opts Options, filename, url string) ([]byte, error) {
	path := filepath.Join(opts.CacheDir, filename)
	info, statErr := os.Stat(path)
	if statErr == nil && time.Since(info.ModTime()) < opts.TTL {
		return os.ReadFile(path)
	}
	data, err := download(opts.HTTPClient, url)
	if err != nil {
		if statErr == nil {
			opts.Logger.Warn("wfdata: using stale cache after fetch failure", "err", err)
			return os.ReadFile(path)
		}
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		opts.Logger.Warn("wfdata: failed to write cache", "err", err)
	}
	return data, nil
}

func download(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}
