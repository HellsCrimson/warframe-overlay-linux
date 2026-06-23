package main

import (
	"strings"

	"warframe-overlay-linux/internal/db"
	"warframe-overlay-linux/internal/inventory"
	"warframe-overlay-linux/internal/wfdata"
	"warframe-overlay-linux/internal/wfmarket"
)

// maxCraftDepth caps the recipe recursion. Real recipes are shallow (item →
// component blueprints → raw resources); the cap is a safety net against any
// pathological self-referential data.
const maxCraftDepth = 5

// CraftNode is one node of an item's crafting tree: the target item at the root,
// its recipe components as children, and (for buildable components) their own
// recipes nested beneath. Need is the total quantity required at this position
// (parent need × recipe count); Have is how many the player owns.
type CraftNode struct {
	Name       string      `json:"name"`
	Icon       string      `json:"icon"`
	Need       int         `json:"need"`
	Have       int         `json:"have"`
	Enough     bool        `json:"enough"`     // Have >= Need
	IsResource bool        `json:"isResource"` // bulk resource vs. an acquirable part
	Children   []CraftNode `json:"children"`
}

// GetCraftingTree returns the recipe tree for the named item: each component
// with how many the recipe needs and how many the player owns, recursing into
// buildable components (e.g. Excalibur → Chassis blueprint → Ferrite/Rubedo).
// Resource drop-locations and build times are intentionally not included.
func (s *Service) GetCraftingTree(itemName string) CraftNode {
	s.mu.Lock()
	inv, names, prices, market := s.inv, s.names, s.prices, s.market
	s.mu.Unlock()
	if names == nil {
		return CraftNode{Name: itemName}
	}
	root, ok := names.ByName(itemName)
	if !ok {
		return CraftNode{Name: itemName}
	}
	leafIcon := func(name string, resource bool) string {
		return partIconURL(name, resource, names, prices, market)
	}
	return buildCraftNode(names, inv, root, 1, 0, leafIcon)
}

// buildCraftNode constructs the node for an item needed `need` times, recursing
// into buildable components that the player still needs. leafIcon resolves a
// distinct thumbnail for leaf components.
func buildCraftNode(names *wfdata.DB, inv *inventory.Inventory, it wfdata.Item, need, depth int, leafIcon func(string, bool) string) CraftNode {
	icon := names.ImageURL(it.UniqueName)
	if icon == "" {
		icon = names.ImageURLByName(it.Name)
	}
	n := CraftNode{Name: it.Name, Icon: icon, Need: need}
	n.Have = inv.CountByType(it.UniqueName)
	n.Enough = n.Have >= n.Need

	for _, c := range it.Components {
		childNeed := need * maxInt(c.ItemCount, 1)
		have := componentHave(inv, it, c)
		// Recurse only into a buildable sub-component the player still needs.
		// Base resources (Orokin Cell, Morphics, …) also carry a craft recipe in
		// the dataset, but once the player owns enough we show them as a satisfied
		// leaf rather than expanding how to farm-craft them.
		if sub, ok := names.ByUnique(c.UniqueName); ok && len(sub.Components) > 0 && have < childNeed && depth+1 < maxCraftDepth {
			n.Children = append(n.Children, buildCraftNode(names, inv, sub, childNeed, depth+1, leafIcon))
			continue
		}
		name := leafName(names, it, c)
		resource := isResource(inv, c)
		n.Children = append(n.Children, CraftNode{
			Name:       name,
			Icon:       leafIcon(name, resource),
			Need:       childNeed,
			Have:       have,
			Enough:     have >= childNeed,
			IsResource: resource,
		})
	}
	return n
}

// componentHave counts how many of a component the player owns. Resources and
// directly-typed items match by exact internal type; prime parts (whose owned
// "…Blueprint" type differs from the recipe's component type) match loosely by
// name, as the mastery view does. The larger of the two wins.
func componentHave(inv *inventory.Inventory, parent wfdata.Item, c wfdata.Component) int {
	have := inv.CountByType(c.UniqueName)
	if c.IsPart() {
		if pc := inv.PartCount(parent.Name + " " + c.Name); pc > have {
			have = pc
		}
	}
	return have
}

// leafName names a leaf component, preferring the component item's own canonical
// name; for unresolved equipment parts it qualifies by the parent ("Mesa Prime"
// + "Chassis"), and resources keep their bare name ("Ferrite").
func leafName(names *wfdata.DB, parent wfdata.Item, c wfdata.Component) string {
	if it, ok := names.ByUnique(c.UniqueName); ok && it.Name != "" {
		return it.Name
	}
	if c.IsPart() {
		return parent.Name + " " + c.Name
	}
	return c.Name
}

// partIconURL resolves the icon for a part or resource by display name. Equipment
// parts use warframe.market's component-type "subIcon" (the clean prime_chassis /
// prime_barrel / blueprint icon); resources use warframestat's per-resource
// images. warframe.market names warframe components WITH a "Blueprint" suffix
// ("Ivara Prime Chassis Blueprint") but weapon parts without one ("Soma Prime
// Barrel"); the price DB's DropName matches that convention, so it's tried first.
func partIconURL(name string, resource bool, names *wfdata.DB, prices *db.Database, market *wfmarket.Client) string {
	if !resource && market != nil {
		var cands []string
		if prices != nil {
			if it := prices.FindPart(name); it != nil {
				cands = append(cands, it.DropName, it.Name)
			}
		}
		cands = append(cands, name, name+" Blueprint")
		for _, c := range cands {
			if u := marketSubOrThumb(market, c); u != "" {
				return u
			}
		}
	}
	if names != nil {
		if u := names.ImageURLByName(name); u != "" {
			return u
		}
	}
	if market != nil {
		if u := marketSubOrThumb(market, name); u != "" {
			return u
		}
	}
	// Last resort: drop a trailing "Blueprint" and retry (e.g. "Forma Blueprint"
	// has no item image but "Forma" does).
	if base, ok := strings.CutSuffix(name, " Blueprint"); ok {
		if names != nil {
			if u := names.ImageURLByName(base); u != "" {
				return u
			}
		}
		if u := marketSubOrThumb(market, base); u != "" {
			return u
		}
	}
	return ""
}

// marketSubOrThumb prefers the component-type subIcon, falling back to the
// per-item thumbnail.
func marketSubOrThumb(market *wfmarket.Client, name string) string {
	if market == nil || name == "" {
		return ""
	}
	if u := market.SubIconURL(name); u != "" {
		return u
	}
	return market.IconURL(name)
}

// isResource reports whether a component is a bulk crafting resource rather than
// an acquirable equipment part. The path heuristic misses a few resources that
// live outside /Types/Items/ (Orokin Cell, Neurodes, Morphics), so a component
// the player owns by exact internal type is also treated as a resource (prime
// parts never match by exact type).
func isResource(inv *inventory.Inventory, c wfdata.Component) bool {
	return !c.IsPart() || inv.CountByType(c.UniqueName) > 0
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
