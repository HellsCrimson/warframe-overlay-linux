package inventory

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Inventory holds the player's owned prime-part counts (keyed by a normalized
// token signature so reward display names resolve to internal item types despite
// word-order and punctuation differences) plus structured owned-equipment
// categories for the app's collection view.
type Inventory struct {
	owned      map[string]int
	categories []Category
}

// invJSON is the slice of the inventory response we read: owned parts live in
// MiscItems (weapon parts) and Recipes (blueprints); equipment lists are decoded
// separately into the category fields.
type invJSON struct {
	MiscItems []invEntry `json:"MiscItems"`
	Recipes   []invEntry `json:"Recipes"`
}

type invEntry struct {
	ItemType  string `json:"ItemType"`
	ItemCount int    `json:"ItemCount"`
}

var pascalRe = regexp.MustCompile(`[A-Z][a-z0-9]*`)

// Parse builds an Inventory from a raw inventory.php JSON response.
func Parse(raw []byte) (*Inventory, error) {
	var j invJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, err
	}
	inv := &Inventory{owned: make(map[string]int)}
	add := func(entries []invEntry) {
		for _, e := range entries {
			if !strings.Contains(e.ItemType, "Prime") {
				continue
			}
			leaf := e.ItemType[strings.LastIndexByte(e.ItemType, '/')+1:]
			sig := signature(pascalRe.FindAllString(leaf, -1))
			if sig == "" {
				continue
			}
			inv.owned[sig] += e.ItemCount
		}
	}
	add(j.MiscItems)
	add(j.Recipes)

	// Structured equipment categories for the collection view.
	var eq equipJSON
	if err := json.Unmarshal(raw, &eq); err == nil {
		inv.categories = buildCategories(eq)
	}
	return inv, nil
}

// Categories returns the owned equipment grouped by kind (Warframes, Primary, …).
func (inv *Inventory) Categories() []Category {
	if inv == nil {
		return nil
	}
	return inv.categories
}

// Owned returns how many of the given reward (by display name) the player owns.
func (inv *Inventory) Owned(displayName string) int {
	if inv == nil {
		return 0
	}
	return inv.owned[signature(strings.Fields(displayName))]
}

// Len reports how many distinct owned prime parts were parsed.
func (inv *Inventory) Len() int { return len(inv.owned) }

// signature normalizes a set of word tokens to a canonical, order-independent
// key: lowercase, alphanumeric only, dropping the ubiquitous "prime" token (its
// position differs between display names and internal types) and any empty
// punctuation tokens (e.g. "&").
func signature(tokens []string) string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		t = keepAlnum(strings.ToLower(t))
		if t == "" || t == "prime" {
			continue
		}
		out = append(out, t)
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

func keepAlnum(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// LoadFile parses a saved inventory JSON response (for development/offline use
// when the game is not running).
func LoadFile(path string) (*Inventory, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

// Load performs the full pipeline: find the game, scrape auth, fetch and parse
// the inventory. Returns the typed errors (ErrNotRunning, ErrPermission,
// ErrAuthNotFound) so callers can give actionable messages.
func Load(ctx context.Context) (*Inventory, error) {
	pid, err := FindWarframePID()
	if err != nil {
		return nil, err
	}
	auth, err := ScrapeAuth(pid)
	if err != nil {
		return nil, err
	}
	raw, err := FetchRaw(ctx, auth)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}
