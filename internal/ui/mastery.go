package ui

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/widget/material"

	"warframe-overlay-linux/internal/mastery"
)

// layoutMastery draws the Mastery view: a summary of progress and a
// best-to-do-next list of actionable items.
func (a *App) layoutMastery(gtx layout.Context) layout.Dimensions {
	res := a.masteryResult()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.H5(a.th, "Mastery").Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if res == nil {
				return layout.Inset{Top: unitDp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					l := material.Body2(a.th, "Load your inventory to see mastery progress.")
					l.Color = rgb(0x80838d)
					return l.Layout(gtx)
				})
			}
			return a.masterySummary(gtx, res.Summary)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if res == nil {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unitDp(10), Bottom: unitDp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				cb := material.CheckBox(a.th, &a.hideNotStarted, "Hide not-yet-started items")
				cb.Color = rgb(0xc4c6cf)
				cb.IconColor = a.th.Palette.ContrastBg
				return cb.Layout(gtx)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if res == nil {
				return layout.Dimensions{}
			}
			return a.masteryListView(gtx, res)
		}),
	)
}

func (a *App) masterySummary(gtx layout.Context, s mastery.Summary) layout.Dimensions {
	chip := func(label string, n int, col uint32) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unitDp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				t := material.Body2(a.th, fmt.Sprintf("%d %s", n, label))
				t.Color = rgb(col)
				return t.Layout(gtx)
			})
		})
	}
	return layout.Inset{Top: unitDp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			chip("mastered", s.Mastered, 0x6fae6a),
			chip("to rank up", s.BuiltUnranked, 0xf2b134),
			chip("ready to build", s.ReadyToBuild, 0x5fc7a0),
			chip("collecting", s.PartsPartial, 0x5c9bd6),
			chip("not started", s.NotStarted, 0x80838d),
		)
	})
}

func (a *App) masteryListView(gtx layout.Context, res *mastery.Result) layout.Dimensions {
	items := res.Items
	if a.hideNotStarted.Value {
		filtered := items[:0:0]
		for _, it := range items {
			if it.Status != mastery.NotStarted {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	if len(items) == 0 {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			l := material.Body1(a.th, "Nothing actionable — everything owned is mastered. 🎉")
			l.Color = rgb(0x80838d)
			return l.Layout(gtx)
		})
	}
	return material.List(a.th, &a.masteryList).Layout(gtx, len(items), func(gtx layout.Context, i int) layout.Dimensions {
		return a.masteryRow(gtx, items[i])
	})
}

func (a *App) masteryRow(gtx layout.Context, it mastery.Item) layout.Dimensions {
	return layout.Inset{Top: unitDp(4), Bottom: unitDp(4), Left: unitDp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.Body1(a.th, it.Name).Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				detail := masteryDetail(it)
				if detail == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Right: unitDp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					t := material.Caption(a.th, detail)
					t.Color = rgb(0x80838d)
					return t.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return statusBadge(gtx, a.th, it.Status)
			}),
		)
	})
}

func masteryDetail(it mastery.Item) string {
	switch it.Status {
	case mastery.BuiltUnranked:
		return fmt.Sprintf("rank %d / %d", it.Rank, it.MaxRank)
	case mastery.ReadyToBuild, mastery.PartsPartial, mastery.NotStarted:
		if it.PartsTotal > 0 {
			return fmt.Sprintf("%d / %d parts", it.PartsOwned, it.PartsTotal)
		}
	}
	return ""
}

func statusBadge(gtx layout.Context, th *material.Theme, s mastery.Status) layout.Dimensions {
	var col uint32
	switch s {
	case mastery.BuiltUnranked:
		col = 0xf2b134
	case mastery.ReadyToBuild:
		col = 0x5fc7a0
	case mastery.PartsPartial:
		col = 0x5c9bd6
	default:
		col = 0x6a6d77
	}
	label := material.Caption(th, s.String())
	label.Color = rgb(col)
	return label.Layout(gtx)
}
