// Package db loads and caches the Warframe item price/ducat database (the same
// data WFInfo uses) and fuzzy-matches OCR'd reward names against it.
//
// Primary source is the WFInfo aggregate endpoints on api.warframestat.us. That
// service is intermittently unavailable, so loads are cached to disk and stale
// cache is served when the network fails.
package db

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agnivade/levenshtein"
)

// Source URLs. Vars (not consts) so tests can point them at a local server.
var (
	pricesURLVar        = "https://api.warframestat.us/wfinfo/prices/"
	filteredItemsURLVar = "https://api.warframestat.us/wfinfo/filtered_items/"
)

// Item is a single tradeable reward part with its plat and ducat value.
type Item struct {
	Name     string  // canonical name as it appears in price data
	DropName string  // name as shown on the reward screen (OCR target)
	Platinum float64 // warframe.market custom average
	Ducats   int
}

// Database is the in-memory, queryable item set.
type Database struct {
	items  []Item
	byPart map[string]int // part signature -> index into items
}

// FindPart resolves a (possibly suffix-less) part name to its canonical item,
// matching loosely on a sorted-token signature that ignores "blueprint"/
// "component" — so "Mesa Prime Chassis" resolves to "Mesa Prime Chassis
// Blueprint". Returns nil when unknown.
func (d *Database) FindPart(name string) *Item {
	if d == nil || d.byPart == nil {
		return nil
	}
	if i, ok := d.byPart[partSig(name)]; ok {
		return &d.items[i]
	}
	return nil
}

// partSig is a sorted-token signature dropping "blueprint"/"component" (but
// keeping "prime", so prime and non-prime variants stay distinct).
func partSig(name string) string {
	fields := strings.Fields(strings.ToLower(name))
	out := fields[:0]
	for _, f := range fields {
		var b strings.Builder
		for _, r := range f {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
		t := b.String()
		if t == "" || t == "blueprint" || t == "component" {
			continue
		}
		out = append(out, t)
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// priceItem matches one element of prices.json. custom_avg arrives as a JSON
// string (e.g. "78.6") in the live data, so it is parsed leniently.
type priceItem struct {
	Name      string    `json:"name"`
	CustomAvg jsonFloat `json:"custom_avg"`
}

// jsonFloat unmarshals from either a JSON number or a quoted numeric string.
type jsonFloat float64

func (f *jsonFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*f = jsonFloat(v)
	return nil
}

// filteredItems matches the relevant parts of filtered_items.json.
type filteredItems struct {
	Eqmt map[string]struct {
		Type  string `json:"type"`
		Parts map[string]struct {
			Ducats int `json:"ducats"`
		} `json:"parts"`
	} `json:"eqmt"`
	// IgnoredItems holds non-prime rewards like "Forma Blueprint" that have no
	// market/ducat value but still appear on the reward screen; we include them
	// so they match (and correctly show as worthless "skip" rewards).
	IgnoredItems map[string]json.RawMessage `json:"ignored_items"`
}

// Options configures Load.
type Options struct {
	CacheDir   string
	TTL        time.Duration
	HTTPClient *http.Client
	Logger     *slog.Logger
}

// Load returns a Database, fetching fresh data when the cache is older than TTL
// and falling back to (stale) cache on network failure.
func Load(opts Options) (*Database, error) {
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if err := os.MkdirAll(opts.CacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	pricesRaw, err := fetchCached(opts, "prices.json", pricesURLVar)
	if err != nil {
		return nil, fmt.Errorf("load prices: %w", err)
	}
	filteredRaw, err := fetchCached(opts, "filtered_items.json", filteredItemsURLVar)
	if err != nil {
		return nil, fmt.Errorf("load filtered_items: %w", err)
	}

	var prices []priceItem
	if err := json.Unmarshal(pricesRaw, &prices); err != nil {
		return nil, fmt.Errorf("decode prices: %w", err)
	}
	priceByName := make(map[string]float64, len(prices))
	for _, p := range prices {
		priceByName[p.Name] = float64(p.CustomAvg)
	}

	var fi filteredItems
	if err := json.Unmarshal(filteredRaw, &fi); err != nil {
		return nil, fmt.Errorf("decode filtered_items: %w", err)
	}

	db := &Database{}
	for name := range fi.IgnoredItems {
		db.items = append(db.items, Item{Name: name, DropName: name})
	}
	for _, eq := range fi.Eqmt {
		warframeLike := eq.Type == "Warframes" || eq.Type == "Archwing"
		for partName, part := range eq.Parts {
			plat, ok := priceByName[partName]
			if !ok {
				plat = priceByName[partName+" Blueprint"]
			}
			dropName := partName
			// Warframe component parts drop as "<Part> Blueprint".
			if warframeLike && isComponent(partName) && !strings.HasSuffix(partName, "Blueprint") {
				dropName = partName + " Blueprint"
			}
			db.items = append(db.items, Item{
				Name:     partName,
				DropName: dropName,
				Platinum: plat,
				Ducats:   part.Ducats,
			})
		}
	}
	db.byPart = make(map[string]int, len(db.items))
	for i := range db.items {
		if sig := partSig(db.items[i].DropName); sig != "" {
			db.byPart[sig] = i
		}
	}
	opts.Logger.Info("db loaded", "items", len(db.items))
	return db, nil
}

func isComponent(name string) bool {
	for _, suf := range []string{"Systems", "Neuroptics", "Chassis", "Harness", "Wings"} {
		if strings.HasSuffix(name, suf) {
			return true
		}
	}
	return false
}

// fetchCached returns the cached file contents, refreshing from url when the
// cache is missing or older than opts.TTL. On network error a stale cache is
// returned if present.
func fetchCached(opts Options, filename, url string) ([]byte, error) {
	path := filepath.Join(opts.CacheDir, filename)
	info, statErr := os.Stat(path)
	fresh := statErr == nil && time.Since(info.ModTime()) < opts.TTL
	if fresh {
		return os.ReadFile(path)
	}

	data, err := download(opts.HTTPClient, url)
	if err != nil {
		if statErr == nil {
			opts.Logger.Warn("db: using stale cache after fetch failure", "file", filename, "err", err)
			return os.ReadFile(path)
		}
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		opts.Logger.Warn("db: failed to write cache", "file", filename, "err", err)
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
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// Match resolves an OCR'd reward name to a database item. OCR of the relic
// screen often surrounds the real name with icon/border noise (e.g.
// "4 - U 2 X Forma Blueprint"), so matching proceeds in two passes:
//
//  1. Containment: the longest item name that appears as a normalized substring
//     of the OCR text wins. This shrugs off leading/trailing garbage.
//  2. Fuzzy: nearest item by Levenshtein distance over the whole string, within
//     a length-proportional threshold, for names mangled inside (character
//     confusions) rather than merely surrounded by noise.
func (d *Database) Match(needle string) *Item {
	norm := normalizeName(needle)
	if len(norm) < 3 {
		return nil
	}

	// Pass 1: substring containment, longest (most specific) name wins.
	var contained *Item
	containedLen := 0
	for i := range d.items {
		cand := normalizeName(d.items[i].DropName)
		// Require a reasonably long name to avoid spurious matches of short
		// fragments inside noise.
		if len(cand) >= 8 && len(cand) > containedLen && strings.Contains(norm, cand) {
			containedLen = len(cand)
			contained = &d.items[i]
		}
	}
	if contained != nil {
		return contained
	}

	// Pass 2: partial (windowed) fuzzy match. For each candidate, find the
	// best-aligned window of the OCR text and score by edit distance there. This
	// tolerates leading/trailing noise AND in-name character confusions (e.g.
	// "...Calibah Prime Blueprint" with an OCR 'h'), while still rejecting items
	// that simply are not in the database (a new prime won't align well to any
	// existing "... Prime Handle").
	var best *Item
	bestDist := 1 << 30
	for i := range d.items {
		cand := normalizeName(d.items[i].DropName)
		if len(cand) < 5 {
			continue
		}
		dist := partialDistance(cand, norm)
		// Normalize ties toward longer (more specific) names.
		if dist < bestDist || (dist == bestDist && best != nil && len(cand) > len(normalizeName(best.DropName))) {
			bestDist = dist
			best = &d.items[i]
		}
	}
	if best == nil {
		return nil
	}
	// Strict threshold. Items sharing a long suffix ("... Prime Handle",
	// "... Prime Blueprint") differ only in the leading weapon/frame name, and
	// partial matching credits the shared suffix; a tight bound ensures a wrong
	// distinctive prefix (e.g. an OCR'd "Afentis" against "Bo Prime Handle")
	// fails rather than mispricing, while still tolerating a stray OCR character
	// in a real name.
	threshold := max(len(normalizeName(best.DropName))/7, 1)
	if bestDist > threshold {
		return nil
	}
	return best
}

// partialDistance returns the minimum Levenshtein distance between cand and any
// substring window of hay of length len(cand) (with a little slack). When cand
// is longer than hay it falls back to a whole-string comparison.
func partialDistance(cand, hay string) int {
	if len(cand) >= len(hay) {
		return levenshtein.ComputeDistance(cand, hay)
	}
	best := 1 << 30
	win := len(cand)
	for start := 0; start+win <= len(hay); start++ {
		d := levenshtein.ComputeDistance(cand, hay[start:start+win])
		if d < best {
			best = d
			if best == 0 {
				break
			}
		}
	}
	return best
}

// Len reports how many items were loaded.
func (d *Database) Len() int { return len(d.items) }

// Items returns all known tradeable items (name, drop name, platinum, ducats).
func (d *Database) Items() []Item {
	if d == nil {
		return nil
	}
	return d.items
}

// normalizeName lowercases and strips non-alphanumeric characters for matching.
func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
