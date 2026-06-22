package ui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/widget/material"
)

const sidebarWidth = 190

// layoutSidebar draws the left navigation column.
func (a *App) layoutSidebar(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Dp(sidebarWidth)
	gtx.Constraints.Max.X = gtx.Dp(sidebarWidth)
	// Sidebar background.
	fillRect(gtx, gtx.Constraints.Max, rgb(0x1b1d24))

	return layout.Inset{Top: unitDp(14), Bottom: unitDp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(sections)+1)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unitDp(16), Bottom: unitDp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				t := material.H6(a.th, "Companion")
				t.Color = a.th.Palette.ContrastBg
				return t.Layout(gtx)
			})
		}))
		for i := range sections {
			i := i
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.navBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.navItem(gtx, i)
				})
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (a *App) navItem(gtx layout.Context, i int) layout.Dimensions {
	sec := sections[i]
	selected := i == a.sel

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(40)}
			if selected {
				return fillRect(gtx, size, rgb(0x2a2d38))
			}
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unitDp(16), Top: unitDp(9), Bottom: unitDp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := sec.name
				if !sec.ready {
					label += "  ·  soon"
				}
				t := material.Body1(a.th, label)
				switch {
				case selected:
					t.Color = a.th.Palette.ContrastBg
				case !sec.ready:
					t.Color = rgb(0x60636e)
				default:
					t.Color = rgb(0xc4c6cf)
				}
				return t.Layout(gtx)
			})
		}),
	)
}
