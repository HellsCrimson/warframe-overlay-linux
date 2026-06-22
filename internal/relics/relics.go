// Package relics provides void-relic drop tables — which prime part each relic
// can drop and at what rarity/probability. It powers the mastery view's
// "farmable from relics I own" ordering.
//
// Data comes from the community warframestat.us items API. The full catalogue is
// large, so only the distilled relic→rewards map is cached to disk (and
// stale-served when the network is unavailable), mirroring the wfdata/db caches.
package relics

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

// itemsURLVar is the warframestat items endpoint, trimmed to the relic fields we
// need. It is a var so tests can point it at a local server. The endpoint has no
// working category filter, so relics are selected client-side by type.
var itemsURLVar = "https://api.warframestat.us/items/?only=uniqueName,name,type,rewards"

// Reward is one possible drop from a relic.
type Reward struct {
	Part   string  `json:"part"`   // reward item display name, e.g. "Rhino Prime Neuroptics Blueprint"
	Rarity string  `json:"rarity"` // "Common" | "Uncommon" | "Rare"
	Chance float64 `json:"chance"` // drop chance in percent for this relic's refinement
}

// Relic is one relic variant: its display name and drop table.
type Relic struct {
	// Name is the era + code without refinement, e.g. "Lith D1".
	Name string `json:"name"`
	// Era is "Lith" | "Meso" | "Neo" | "Axi" | "Requiem" (the first name token).
	Era string `json:"era"`
	// Refinement is "Intact" | "Exceptional" | "Flawless" | "Radiant" (or "").
	Refinement string   `json:"refinement"`
	Rewards    []Reward `json:"rewards"`
}

// refinements are the relic-quality suffixes, in increasing order of refinement.
var refinements = map[string]bool{"Intact": true, "Exceptional": true, "Flawless": true, "Radiant": true}

// parseRelic splits a warframestat relic name ("Lith D1 Intact") into its era +
// code ("Lith D1") and refinement ("Intact").
func parseRelic(fullName string) (name, era, refinement string) {
	fields := strings.Fields(fullName)
	if len(fields) == 0 {
		return fullName, "", ""
	}
	if last := fields[len(fields)-1]; refinements[last] {
		refinement = last
		fields = fields[:len(fields)-1]
	}
	return strings.Join(fields, " "), fields[0], refinement
}

// Tables maps each relic's internal type to its drop table.
type Tables struct {
	byRelic map[string]Relic
}

// Rewards returns the drop table for a relic (by internal item type), or nil.
func (t *Tables) Rewards(uniqueName string) []Reward {
	if t == nil {
		return nil
	}
	return t.byRelic[uniqueName].Rewards
}

// Get returns the full relic for an internal item type, and whether it was found.
func (t *Tables) Get(uniqueName string) (Relic, bool) {
	if t == nil {
		return Relic{}, false
	}
	r, ok := t.byRelic[uniqueName]
	return r, ok
}

// Len reports how many relics have drop tables.
func (t *Tables) Len() int {
	if t == nil {
		return 0
	}
	return len(t.byRelic)
}

// Options configures Load.
type Options struct {
	CacheDir   string
	TTL        time.Duration
	HTTPClient *http.Client
	Logger     *slog.Logger
}

// rawItem is the slice of an items-API entry we read.
type rawItem struct {
	UniqueName string `json:"uniqueName"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Rewards    []struct {
		Rarity string  `json:"rarity"`
		Chance float64 `json:"chance"`
		Item   struct {
			Name string `json:"name"`
		} `json:"item"`
	} `json:"rewards"`
}

// Load returns relic drop tables, refreshing from the API when the cache is older
// than TTL and stale-serving on network failure. Only the distilled relic→rewards
// map is cached (the raw catalogue is several MB).
func Load(opts Options) (*Tables, error) {
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.TTL == 0 {
		opts.TTL = 7 * 24 * time.Hour // relic tables change rarely (new prime access)
	}
	if err := os.MkdirAll(opts.CacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	path := filepath.Join(opts.CacheDir, "relics.json")
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < opts.TTL {
		if t, err := loadCache(path); err == nil {
			return t, nil
		}
	}

	data, err := download(opts.HTTPClient, itemsURLVar)
	if err != nil {
		if t, cerr := loadCache(path); cerr == nil {
			opts.Logger.Warn("relics: using stale cache after fetch failure", "err", err)
			return t, nil
		}
		return nil, fmt.Errorf("load relics: %w", err)
	}
	tables, err := parseItems(data)
	if err != nil {
		if t, cerr := loadCache(path); cerr == nil {
			opts.Logger.Warn("relics: using stale cache after parse failure", "err", err)
			return t, nil
		}
		return nil, err
	}
	if b, err := json.Marshal(tables.byRelic); err == nil {
		if err := os.WriteFile(path, b, 0o644); err != nil {
			opts.Logger.Warn("relics: failed to write cache", "err", err)
		}
	}
	opts.Logger.Info("relic tables loaded", "relics", tables.Len())
	return tables, nil
}

// parseItems distils the raw items catalogue into the relic→rewards map.
func parseItems(data []byte) (*Tables, error) {
	var items []rawItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode items: %w", err)
	}
	byRelic := make(map[string]Relic)
	for _, it := range items {
		if it.Type != "Relic" || it.UniqueName == "" || len(it.Rewards) == 0 {
			continue
		}
		rewards := make([]Reward, 0, len(it.Rewards))
		for _, r := range it.Rewards {
			if r.Item.Name == "" {
				continue
			}
			rewards = append(rewards, Reward{Part: r.Item.Name, Rarity: r.Rarity, Chance: r.Chance})
		}
		if len(rewards) > 0 {
			name, era, refinement := parseRelic(it.Name)
			byRelic[it.UniqueName] = Relic{Name: name, Era: era, Refinement: refinement, Rewards: rewards}
		}
	}
	if len(byRelic) == 0 {
		return nil, fmt.Errorf("no relics in items data")
	}
	return &Tables{byRelic: byRelic}, nil
}

func loadCache(path string) (*Tables, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var byRelic map[string]Relic
	if err := json.Unmarshal(b, &byRelic); err != nil {
		return nil, err
	}
	if len(byRelic) == 0 {
		return nil, fmt.Errorf("empty relic cache")
	}
	return &Tables{byRelic: byRelic}, nil
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
