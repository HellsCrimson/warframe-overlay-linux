package main

import (
	"warframe-overlay-linux/internal/inventory"
	"warframe-overlay-linux/internal/mastery"
	"warframe-overlay-linux/internal/wfdata"
)

// setPartInfo is one part of a prime set and whether the player owns enough of
// it to build.
type setPartInfo struct {
	Name  string
	Owned bool
}

// setEntry describes the masterable item (prime set) a reward part belongs to:
// its name, whether it's mastered/crafted, and the full part checklist.
type setEntry struct {
	SetName  string
	Mastered bool // ranked to mastery (lifetime affinity at max)
	Crafted  bool // a copy is built/owned (or mastered)
	Parts    []setPartInfo
}

// setIndex maps a part key (see partKey) to the set it belongs to, so a relic
// reward or overlay item resolves to its parent prime's status and sibling parts.
type setIndex map[string]setEntry

// buildSetIndex indexes every masterable item by the parts that build it, so a
// reward part ("Rhino Prime Neuroptics Blueprint") resolves to its set ("Rhino
// Prime"), the set's mastered/crafted status, and the ownership of each sibling
// part. Reuses the loose partKey matching shared with the relic-farm index.
func buildSetIndex(names *wfdata.DB, inv *inventory.Inventory) setIndex {
	idx := setIndex{}
	if names == nil {
		return idx
	}
	owned := map[string]bool{} // currently-built items, by internal type
	for _, c := range inv.Categories() {
		for _, it := range c.Items {
			owned[it.Type] = true
		}
	}
	for _, m := range names.Masterable() {
		mastered := mastery.Rank(inv.MasteryXP(m.UniqueName), m.ProductCategory) >= mastery.MaxRank(m.ProductCategory)
		entry := setEntry{SetName: m.Name, Mastered: mastered, Crafted: mastered || owned[m.UniqueName]}
		for _, c := range m.Components {
			if !c.IsPart() {
				continue // bulk resources aren't tracked set parts
			}
			query := m.Name + " " + c.Name
			entry.Parts = append(entry.Parts, setPartInfo{
				Name:  query,
				Owned: inv.PartCount(query) >= maxInt(c.ItemCount, 1),
			})
		}
		if len(entry.Parts) == 0 {
			continue // no acquirable parts: nothing to resolve a reward against
		}
		for _, c := range m.Components {
			if !c.IsPart() {
				continue
			}
			idx[partKey(m.Name+" "+c.Name)] = entry
		}
	}
	return idx
}
