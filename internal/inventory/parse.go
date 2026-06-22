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
	parts      map[string]int // loose-signature part counts (for recipe matching)
	masteryXP  map[string]int // lifetime affinity per item type (mastery source)
	relics     map[string]int // owned void relics, by internal ItemType -> count
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
	inv := &Inventory{owned: make(map[string]int), parts: make(map[string]int), relics: make(map[string]int)}
	add := func(entries []invEntry) {
		for _, e := range entries {
			// Void relics live in MiscItems too, keyed by their projection type
			// (e.g. ".../Projections/T1VoidProjectionDBronze"). Record them by
			// internal type so they can be matched to drop tables; they are not
			// "Prime" parts, so handle them before the Prime-only filter below.
			if strings.Contains(e.ItemType, "VoidProjection") {
				inv.relics[e.ItemType] += e.ItemCount
				continue
			}
			if !strings.Contains(e.ItemType, "Prime") {
				continue
			}
			tokens := pascalRe.FindAllString(e.ItemType[strings.LastIndexByte(e.ItemType, '/')+1:], -1)
			if sig := signature(tokens); sig != "" {
				inv.owned[sig] += e.ItemCount
			}
			if ps := partSignature(tokens); ps != "" {
				inv.parts[ps] += e.ItemCount
			}
		}
	}
	add(j.MiscItems)
	add(j.Recipes)

	// Structured equipment categories for the collection view.
	var eq equipJSON
	if err := json.Unmarshal(raw, &eq); err == nil {
		inv.categories = buildCategories(eq)
	}

	// XPInfo: lifetime affinity per item type. This is the authoritative mastery
	// source — it persists after an item is sold and is recorded once per type,
	// so it (not the current equipment list) determines what's been mastered.
	var xp struct {
		XPInfo []equipEntry `json:"XPInfo"`
	}
	if err := json.Unmarshal(raw, &xp); err == nil {
		inv.masteryXP = make(map[string]int, len(xp.XPInfo))
		for _, e := range xp.XPInfo {
			if e.ItemType != "" && e.XP > inv.masteryXP[e.ItemType] {
				inv.masteryXP[e.ItemType] = e.XP
			}
		}
	}
	return inv, nil
}

// MasteryXP returns the player's lifetime affinity for an item type (0 if never
// leveled). This is the value that determines mastery, surviving the item being
// sold and deduplicated across copies.
func (inv *Inventory) MasteryXP(uniqueName string) int {
	if inv == nil {
		return 0
	}
	return inv.masteryXP[uniqueName]
}

// Relics returns the player's owned void relics keyed by internal item type
// (e.g. "/Lotus/Types/Game/Projections/T1VoidProjectionDBronze") with how many of
// each are held. The key matches the "uniqueName" in the relic drop tables, so a
// refined relic (…Silver/Gold/Platinum) maps to its refined drop chances.
func (inv *Inventory) Relics() map[string]int {
	if inv == nil {
		return nil
	}
	return inv.relics
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

// PartCount returns how many of a named prime part the player owns, matching
// loosely so a build-recipe component resolves to the owned drop: it ignores
// "prime", "blueprint" and "component" (warframe components are internally
// "…Component" but owned as "…Blueprint") and word order. E.g.
// PartCount("Rhino Prime Chassis") matches an owned "Rhino Prime Chassis
// Blueprint".
func (inv *Inventory) PartCount(name string) int {
	if inv == nil {
		return 0
	}
	return inv.parts[partSignature(strings.Fields(name))]
}

// partSignature drops "blueprint" and "component" (warframe components are
// internally "…Component" but owned as "…Blueprint") and sorts the remaining
// tokens for word-order independence. Unlike signature() it KEEPS "prime", so a
// non-prime part ("Braton Blueprint") does not collide with its prime variant
// ("Braton Prime Blueprint").
func partSignature(tokens []string) string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		t = keepAlnum(strings.ToLower(t))
		switch t {
		case "", "blueprint", "component":
			continue
		}
		out = append(out, t)
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

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
