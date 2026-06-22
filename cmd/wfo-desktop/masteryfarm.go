package main

import (
	"math"
	"sort"
	"strings"

	"warframe-overlay-linux/internal/db"
	"warframe-overlay-linux/internal/inventory"
	"warframe-overlay-linux/internal/mastery"
	"warframe-overlay-linux/internal/relics"
)

// Sort modes for the mastery view (values shared with the frontend selector).
const (
	sortNext   = "next"   // best to do next (status, then closest to complete)
	sortCost   = "cost"   // cheapest to finish buying the missing parts first
	sortRelics = "relics" // most farmable from relics the player already owns first
)

// relicDrop is one owned relic that can drop a wanted part.
type relicDrop struct {
	unique string  // relic internal type (to dedupe relics covering several parts)
	chance float64 // drop chance in percent
	count  int     // how many of this relic the player owns
}

// relicIndex maps a part key (see partKey) to the owned relics that drop it.
type relicIndex map[string][]relicDrop

// buildRelicIndex indexes the player's owned relics by the parts they can drop,
// so a wanted part can be looked up to find which owned relics yield it.
func buildRelicIndex(inv *inventory.Inventory, tables *relics.Tables) relicIndex {
	idx := relicIndex{}
	if inv == nil || tables == nil {
		return idx
	}
	for unique, count := range inv.Relics() {
		if count <= 0 {
			continue
		}
		for _, r := range tables.Rewards(unique) {
			if k := partKey(r.Part); k != "" {
				idx[k] = append(idx[k], relicDrop{unique: unique, chance: r.Chance, count: count})
			}
		}
	}
	return idx
}

// annotate fills in an item's build cost and owned-relic farmability from the
// price database and relic index, returning the enriched view item.
func annotate(it mastery.Item, view MasteryItem, prices *db.Database, idx relicIndex) MasteryItem {
	view.CostKnown = true
	relicTypes := map[string]int{} // distinct relic types covering a missing part -> count
	for _, p := range it.Parts {
		missing := p.Need - p.Have
		if missing <= 0 {
			continue
		}
		// Cost to buy the still-missing copies of this part.
		if prices != nil {
			if item := prices.FindPart(p.Query); item != nil {
				view.BuildCost += int(math.Round(item.Platinum)) * missing
			} else {
				view.CostKnown = false // no price for a needed part: cost is unknown
			}
		}
		// Farmability: owned relics that drop this part.
		for _, d := range idx[partKey(p.Query)] {
			view.RelicScore += d.chance / 100 * float64(d.count)
			if d.chance > view.BestChance {
				view.BestChance = d.chance
			}
			relicTypes[d.unique] = d.count
		}
	}
	for _, c := range relicTypes {
		view.RelicCount += c
	}
	return view
}

// sortMastery orders the view items by the chosen mode. The input is assumed to
// already be in "best next" order, which is preserved as the stable tiebreak.
func sortMastery(items []MasteryItem, mode string) {
	switch mode {
	case sortCost:
		// Cheapest to finish first; items with an unknown price sink to the bottom.
		sort.SliceStable(items, func(i, j int) bool {
			ci, cj := costKey(items[i]), costKey(items[j])
			return ci < cj
		})
	case sortRelics:
		// Most farmable from owned relics first; items no owned relic covers sink.
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].RelicScore > items[j].RelicScore
		})
	}
}

// costKey ranks an item for the cheapest-to-finish sort: known costs ascending,
// unknown-price items after all known ones.
func costKey(it MasteryItem) float64 {
	if !it.CostKnown {
		return math.Inf(1)
	}
	return float64(it.BuildCost)
}

// partKey normalizes a part or reward name to an order-independent token key,
// dropping "blueprint"/"component" so a recipe component ("Rhino Prime
// Neuroptics") matches its relic reward ("Rhino Prime Neuroptics Blueprint"). It
// keeps "prime" so prime and non-prime variants stay distinct, mirroring the
// price database's part signature.
func partKey(name string) string {
	fields := strings.Fields(strings.ToLower(name))
	out := fields[:0]
	for _, f := range fields {
		var b strings.Builder
		for _, r := range f {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
		switch t := b.String(); t {
		case "", "blueprint", "component":
		default:
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}
