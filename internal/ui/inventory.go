package ui

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget/material"

	"warframe-overlay-linux/internal/inventory"
)

// row is one entry in the flattened, filtered inventory list: either a category
// header or an item.
type row struct {
	header   bool
	category string
	count    int
	item     inventory.OwnedItem
}

// layoutInventory draws the inventory view: a header bar with a load button and
// status, a search field, and a scrollable categorized list.
func (a *App) layoutInventory(gtx layout.Context) layout.Dimensions {
	if a.loadBtn.Clicked(gtx) {
		a.startLoad()
	}

	a.mu.Lock()
	inv, loading, loadErr := a.inv, a.loading, a.loadErr
	a.mu.Unlock()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.inventoryHeader(gtx, inv, loading, loadErr)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unitDp(12), Bottom: unitDp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				ed := material.Editor(a.th, &a.search, "Search items…")
				ed.Color = a.th.Palette.Fg
				return widgetBox(gtx, rgb(0x1b1d24), unitDp(10), func(gtx layout.Context) layout.Dimensions {
					return ed.Layout(gtx)
				})
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if inv == nil {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					msg := "Load your inventory to begin."
					if loading {
						msg = "Loading…"
					}
					l := material.Body1(a.th, msg)
					l.Color = rgb(0x80838d)
					return l.Layout(gtx)
				})
			}
			return a.inventoryList(gtx, inv)
		}),
	)
}

func (a *App) inventoryHeader(gtx layout.Context, inv *inventory.Inventory, loading bool, loadErr error) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.H5(a.th, "Inventory").Layout(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unitDp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				var status string
				var col = rgb(0x80838d)
				switch {
				case loading:
					status = "Loading…"
				case loadErr != nil:
					status = loadErr.Error()
					col = rgb(0xe0664f)
				case inv != nil:
					total := 0
					for _, c := range inv.Categories() {
						total += len(c.Items)
					}
					status = fmt.Sprintf("%d items across %d categories", total, len(inv.Categories()))
				}
				l := material.Body2(a.th, status)
				l.Color = col
				return l.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			btn := material.Button(a.th, &a.loadBtn, buttonLabel(loading, a.cfg.InventoryFile))
			btn.Background = a.th.Palette.ContrastBg
			btn.Color = a.th.Palette.ContrastFg
			return btn.Layout(gtx)
		}),
	)
}

func buttonLabel(loading bool, file string) string {
	if loading {
		return "Loading…"
	}
	if file != "" {
		return "Reload (file)"
	}
	return "Load from game"
}

func (a *App) inventoryList(gtx layout.Context, inv *inventory.Inventory) layout.Dimensions {
	rows := filterRows(inv, a.search.Text())
	if len(rows) == 0 {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			l := material.Body1(a.th, "No items match your search.")
			l.Color = rgb(0x80838d)
			return l.Layout(gtx)
		})
	}
	return material.List(a.th, &a.invList).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		r := rows[i]
		if r.header {
			return layout.Inset{Top: unitDp(14), Bottom: unitDp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				t := material.Body1(a.th, fmt.Sprintf("%s  (%d)", r.category, r.count))
				t.Color = a.th.Palette.ContrastBg
				t.Font.Weight = 700
				return t.Layout(gtx)
			})
		}
		return layout.Inset{Top: unitDp(3), Bottom: unitDp(3), Left: unitDp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return material.Body2(a.th, r.item.Name).Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					rank := material.Caption(a.th, masteryLabel(r.item.XP))
					rank.Color = rgb(0x80838d)
					return rank.Layout(gtx)
				}),
			)
		})
	})
}

// filterRows flattens the categories into header+item rows, applying a
// case-insensitive substring filter on item names. A category header is shown
// only if it has at least one matching item.
func filterRows(inv *inventory.Inventory, query string) []row {
	q := strings.ToLower(strings.TrimSpace(query))
	var rows []row
	for _, c := range inv.Categories() {
		var matched []inventory.OwnedItem
		for _, it := range c.Items {
			if q == "" || strings.Contains(strings.ToLower(it.Name), q) {
				matched = append(matched, it)
			}
		}
		if len(matched) == 0 {
			continue
		}
		rows = append(rows, row{header: true, category: c.Name, count: len(matched)})
		for _, it := range matched {
			rows = append(rows, row{item: it})
		}
	}
	return rows
}

// masteryLabel describes an item's mastery progress from its accumulated XP.
// Equipment masters at rank 30; non-warframe items need 1,600,000 affinity total,
// warframes/companions 1,600,000 as well at rank 30 (we show a coarse "maxed"
// hint rather than an exact rank, which depends on item class).
func masteryLabel(xp int) string {
	const maxRankXP = 1600000
	if xp >= maxRankXP {
		return "rank 30"
	}
	pct := xp * 100 / maxRankXP
	return fmt.Sprintf("%d%%", pct)
}
