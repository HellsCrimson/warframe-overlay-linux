// Package mastery computes mastery progress: each masterable item's rank from
// its affinity, whether it's owned/built/mastered, and — for items not yet owned
// — how many of its parts the player has, to suggest what's best to rank next.
package mastery

import (
	"math"
	"sort"
	"strings"

	"warframe-overlay-linux/internal/inventory"
	"warframe-overlay-linux/internal/wfdata"
)

// Status classifies a masterable item for the player.
type Status int

const (
	NotStarted    Status = iota // not owned, no parts collected
	PartsPartial                // not owned, some parts collected
	ReadyToBuild                // not owned, all parts collected
	BuiltUnranked               // owned but below max rank — just level it
	Mastered                    // owned and at max rank
)

func (s Status) String() string {
	switch s {
	case Mastered:
		return "Mastered"
	case BuiltUnranked:
		return "Built — rank up"
	case ReadyToBuild:
		return "Ready to build"
	case PartsPartial:
		return "Collecting parts"
	default:
		return "Not started"
	}
}

// Item is one masterable item's computed state.
type Item struct {
	Name       string
	Category   string
	UniqueName string
	Status     Status
	Rank       int
	MaxRank    int
	PartsOwned int
	PartsTotal int
}

// Summary counts items by state.
type Summary struct {
	Total, Mastered, BuiltUnranked, ReadyToBuild, PartsPartial, NotStarted int
}

// Result is the full mastery computation.
type Result struct {
	Summary Summary
	// Items is every non-mastered masterable item, sorted best-to-do-next.
	Items []Item
}

// classOf returns the per-rank affinity coefficient and max rank for a product
// category. Warframes/companions/archwing/necramech use 1000×rank²; weapons use
// 500×rank². Most cap at rank 30; Necramechs cap at 40.
func classOf(category string) (perRank, maxRank int) {
	switch category {
	case "Suits", "SpaceSuits", "Sentinels", "MoaPets", "KubrowPets":
		return 1000, 30
	case "MechSuits":
		return 1000, 40
	default: // LongGuns, Pistols, Melee, SpaceGuns, SpaceMelee, SentinelWeapons, OperatorAmps, Hoverboards
		return 500, 30
	}
}

// Rank returns the current mastery rank for an item given its accumulated
// affinity (XP), capped at the category's max rank.
func Rank(xp int, category string) int {
	perRank, maxRank := classOf(category)
	if xp <= 0 {
		return 0
	}
	return min(int(math.Sqrt(float64(xp)/float64(perRank))), maxRank)
}

// MaxRank returns the rank at which an item is fully mastered.
func MaxRank(category string) int { _, m := classOf(category); return m }

// Compute evaluates every masterable item against the player's inventory.
//
// Mastery is determined from lifetime affinity (inventory XPInfo), NOT the
// current equipment list: mastery is permanent once earned, so an item that was
// mastered and then sold still counts, and a low-rank duplicate copy doesn't
// un-master it. Current ownership only decides whether an unmastered item can be
// ranked up (you have a copy) or still needs to be acquired.
func Compute(masterable []wfdata.Item, inv *inventory.Inventory) Result {
	owned := map[string]bool{}
	for _, c := range inv.Categories() {
		for _, it := range c.Items {
			owned[it.Type] = true
		}
	}

	var res Result
	for _, m := range masterable {
		res.Summary.Total++
		_, maxRank := classOf(m.ProductCategory)
		it := Item{
			Name:       m.Name,
			Category:   m.ProductCategory,
			UniqueName: m.UniqueName,
			MaxRank:    maxRank,
			Rank:       Rank(inv.MasteryXP(m.UniqueName), m.ProductCategory),
		}

		if it.Rank >= maxRank {
			it.Status = Mastered
			res.Summary.Mastered++
			continue // mastered items aren't actionable
		}

		if owned[m.UniqueName] {
			it.Status = BuiltUnranked
			res.Summary.BuiltUnranked++
		} else {
			ownedParts, total := countParts(m, inv)
			it.PartsOwned, it.PartsTotal = ownedParts, total
			switch {
			case total > 0 && ownedParts >= total:
				it.Status = ReadyToBuild
				res.Summary.ReadyToBuild++
			case ownedParts > 0:
				it.Status = PartsPartial
				res.Summary.PartsPartial++
			default:
				it.Status = NotStarted
				res.Summary.NotStarted++
			}
		}
		res.Items = append(res.Items, it)
	}

	sortBestNext(res.Items)
	return res
}

// countParts returns how many of an item's acquirable parts the player owns and
// the total number of parts.
func countParts(m wfdata.Item, inv *inventory.Inventory) (owned, total int) {
	for _, c := range m.Components {
		if !c.IsPart() {
			continue // skip bulk resources (Orokin Cell, Forma, …)
		}
		total++
		if inv.PartCount(m.Name+" "+c.Name) > 0 {
			owned++
		}
	}
	return owned, total
}

// sortBestNext orders items by what's most worth doing next: built-but-unranked
// first (just level them), then ready-to-build, then collecting-parts (closest to
// complete first), then not-started; ties broken by name.
func sortBestNext(items []Item) {
	rank := func(s Status) int {
		switch s {
		case BuiltUnranked:
			return 0
		case ReadyToBuild:
			return 1
		case PartsPartial:
			return 2
		default:
			return 3
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if ra, rb := rank(a.Status), rank(b.Status); ra != rb {
			return ra < rb
		}
		// Among part-collecting items, fewer missing parts first.
		if a.Status == PartsPartial && b.Status == PartsPartial {
			am, bm := a.PartsTotal-a.PartsOwned, b.PartsTotal-b.PartsOwned
			if am != bm {
				return am < bm
			}
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}
