package main

import "sort"

// Relic sort modes (values shared with the frontend selector).
const (
	relicSortEra   = "era"   // by era (Lith→Requiem), then code, then refinement
	relicSortValue = "value" // by expected platinum per crack, richest first
	relicSortCount = "count" // by how many the player holds, most first
)

// RelicReward is one possible drop from a relic, valued and flagged with whether
// the player already owns it.
type RelicReward struct {
	Part     string  `json:"part"`
	Rarity   string  `json:"rarity"` // Common | Uncommon | Rare
	Chance   float64 `json:"chance"` // drop chance in percent
	Plat     int     `json:"plat"`   // warframe.market value of the part
	Ducats   int     `json:"ducats"`
	Owned    int     `json:"owned"`    // how many of this part the player already has
	Mastered bool    `json:"mastered"` // the part's prime set is mastered
	Crafted  bool    `json:"crafted"`  // a copy of the set is built/owned
	SetName  string  `json:"setName"`  // the prime set this part belongs to
	Icon     string  `json:"icon"`     // thumbnail URL for the part
}

// RelicRow is one owned relic variant with its drop table.
type RelicRow struct {
	Name       string        `json:"name"`       // "Lith D1"
	Era        string        `json:"era"`        // Lith | Meso | Neo | Axi | Requiem
	Refinement string        `json:"refinement"` // Intact | Exceptional | Flawless | Radiant
	Count      int           `json:"count"`
	Value      int           `json:"value"` // expected platinum per crack (Σ chance×plat)
	Rewards    []RelicReward `json:"rewards"`
}

// RelicsView is the relics tab payload.
type RelicsView struct {
	Total int        `json:"total"` // total relics held
	Types int        `json:"types"` // distinct relic variants held
	Items []RelicRow `json:"items"`
}

// GetRelics lists the relics the player owns with each relic's drop table, the
// platinum/ducat value and ownership of every reward, and the expected platinum
// per crack. sortMode is "era" (default), "value" or "count".
func (s *Service) GetRelics(sortMode string) RelicsView {
	s.mu.Lock()
	inv, prices, tables, names := s.inv, s.prices, s.relics, s.names
	s.mu.Unlock()
	if inv == nil || tables == nil {
		return RelicsView{}
	}

	setIdx := s.setIndex()

	var view RelicsView
	for unique, count := range inv.Relics() {
		if count <= 0 {
			continue
		}
		rel, ok := tables.Get(unique)
		if !ok {
			continue // a relic with no known drop table (e.g. brand new) is skipped
		}
		view.Total += count
		view.Types++

		row := RelicRow{Name: rel.Name, Era: rel.Era, Refinement: rel.Refinement, Count: count}
		var expected float64
		for _, rw := range rel.Rewards {
			rr := RelicReward{Part: rw.Part, Rarity: rw.Rarity, Chance: rw.Chance, Owned: inv.Owned(rw.Part)}
			if e, ok := setIdx[partKey(rw.Part)]; ok {
				rr.Mastered, rr.Crafted, rr.SetName = e.Mastered, e.Crafted, e.SetName
			}
			rr.Icon = partIconURL(rw.Part, false, names, prices, s.market)
			if prices != nil {
				if item := prices.FindPart(rw.Part); item != nil {
					rr.Plat = int(item.Platinum + 0.5)
					rr.Ducats = item.Ducats
				}
			}
			expected += rw.Chance / 100 * float64(rr.Plat)
			row.Rewards = append(row.Rewards, rr)
		}
		// Most valuable drop first, ties broken by rarer (lower chance) first.
		sort.SliceStable(row.Rewards, func(i, j int) bool {
			if row.Rewards[i].Plat != row.Rewards[j].Plat {
				return row.Rewards[i].Plat > row.Rewards[j].Plat
			}
			return row.Rewards[i].Chance < row.Rewards[j].Chance
		})
		row.Value = int(expected + 0.5)
		view.Items = append(view.Items, row)
	}

	sortRelicRows(view.Items, sortMode)
	return view
}

var eraOrder = map[string]int{"Lith": 0, "Meso": 1, "Neo": 2, "Axi": 3, "Requiem": 4}
var refinementOrder = map[string]int{"Intact": 0, "Exceptional": 1, "Flawless": 2, "Radiant": 3}

func eraRank(e string) int {
	if r, ok := eraOrder[e]; ok {
		return r
	}
	return len(eraOrder)
}

// sortRelics orders the relic rows by the chosen mode (era is the default).
func sortRelicRows(rows []RelicRow, mode string) {
	switch mode {
	case relicSortValue:
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Value > rows[j].Value })
	case relicSortCount:
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Count > rows[j].Count })
	default: // by era, then code, then refinement
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := rows[i], rows[j]
			if ra, rb := eraRank(a.Era), eraRank(b.Era); ra != rb {
				return ra < rb
			}
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return refinementOrder[a.Refinement] < refinementOrder[b.Refinement]
		})
	}
}
