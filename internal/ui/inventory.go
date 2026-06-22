package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/widget/material"

	"warframe-overlay-linux/internal/inventory"
	"warframe-overlay-linux/internal/mastery"
)

// friendlyErr turns the inventory module's typed errors into user-facing text.
func friendlyErr(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, inventory.ErrNotRunning):
		return "Warframe isn't running. Start the game and reload."
	case errors.Is(err, inventory.ErrPermission):
		return "Can't read the game's memory (permission)."
	case errors.Is(err, inventory.ErrAuthNotFound):
		return "Couldn't find your session — are you logged in?"
	default:
		return err.Error()
	}
}

func errorsIsPermission(err error) bool {
	return errors.Is(err, inventory.ErrPermission)
}

// row is one entry in the flattened, filtered inventory list: either a category
// header or an item.
type row struct {
	header    bool
	category  string
	count     int
	item      inventory.OwnedItem
	rankLabel string // e.g. "rank 30" or "★ 30" (mastered)
}

// layoutInventory draws the inventory view: a header bar with a load button and
// status, a search field, and a scrollable categorized list.
func (a *App) layoutInventory(gtx layout.Context) layout.Dimensions {
	if a.loadBtn.Clicked(gtx) {
		a.startLoad()
	}

	a.mu.Lock()
	inv, loading, loadErr, loadStart, names := a.inv, a.loading, a.loadErr, a.loadStart, a.names
	a.mu.Unlock()
	resolve := func(it inventory.OwnedItem) string {
		if names != nil {
			if n, ok := names.Name(it.Type); ok {
				return n
			}
		}
		return it.Name
	}

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
				return a.inventoryEmpty(gtx, loading, loadErr, loadStart)
			}
			return a.inventoryList(gtx, inv, resolve)
		}),
	)
}

// inventoryEmpty renders the centered state shown before any inventory is
// available: a loading message (with elapsed time and, after a while, a hint to
// grant memory-read permission), an error, or the initial prompt.
func (a *App) inventoryEmpty(gtx layout.Context, loading bool, loadErr error, loadStart time.Time) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var msg string
				col := rgb(0x9a9da6)
				switch {
				case loading:
					msg = "Getting your inventory…"
				case loadErr != nil:
					msg = friendlyErr(loadErr)
					col = rgb(0xe0815a)
				default:
					msg = "Start Warframe, then click “Load from game”."
				}
				l := material.Body1(a.th, msg)
				l.Color = col
				l.Alignment = text.Middle
				return l.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// Capability hint while a from-game load runs long, or on a
				// permission error.
				slow := loading && a.cfg.InventoryFile == "" && time.Since(loadStart) > 6*time.Second
				perm := errorsIsPermission(loadErr)
				if !slow && !perm {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: unitDp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					hint := material.Body2(a.th, capabilityHint)
					hint.Color = rgb(0x80838d)
					hint.Alignment = text.Middle
					return hint.Layout(gtx)
				})
			}),
		)
	})
}

const capabilityHint = "Reading the game's memory needs permission.\n" +
	"Run:  sudo sysctl kernel.yama.ptrace_scope=0\n" +
	"or grant the app CAP_SYS_PTRACE, then reload."

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
					status = friendlyErr(loadErr)
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

func (a *App) inventoryList(gtx layout.Context, inv *inventory.Inventory, resolve func(inventory.OwnedItem) string) layout.Dimensions {
	rows := filterRows(inv, a.search.Text(), resolve)
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
					lbl := material.Caption(a.th, r.rankLabel)
					if strings.HasPrefix(r.rankLabel, "★") {
						lbl.Color = rgb(0x6fae6a) // mastered
					} else {
						lbl.Color = rgb(0x80838d)
					}
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}

// filterRows flattens the categories into header+item rows, resolving each
// item's display name and applying a case-insensitive substring filter on it. A
// category header is shown only if it has at least one matching item.
func filterRows(inv *inventory.Inventory, query string, resolve func(inventory.OwnedItem) string) []row {
	q := strings.ToLower(strings.TrimSpace(query))
	var rows []row
	for _, c := range inv.Categories() {
		var matched []inventory.OwnedItem
		for _, it := range c.Items {
			it.Name = resolve(it)
			if q == "" || strings.Contains(strings.ToLower(it.Name), q) {
				matched = append(matched, it)
			}
		}
		if len(matched) == 0 {
			continue
		}
		rows = append(rows, row{header: true, category: c.Name, count: len(matched)})
		for _, it := range matched {
			rows = append(rows, row{item: it, rankLabel: rankLabel(it.XP, c.ProductCategory)})
		}
	}
	return rows
}

// rankLabel describes an item's mastery rank from its accumulated affinity,
// using the correct per-class curve. Mastered items get a star.
func rankLabel(xp int, productCategory string) string {
	r := mastery.Rank(xp, productCategory)
	if r >= mastery.MaxRank(productCategory) {
		return fmt.Sprintf("★ %d", r)
	}
	return fmt.Sprintf("rank %d", r)
}
