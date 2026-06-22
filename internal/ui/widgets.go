package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func unitDp(v float32) unit.Dp { return unit.Dp(v) }

// paintRect fills the entire current constraints area with a solid colour.
func paintRect(gtx layout.Context, c color.NRGBA) {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

// fillRect fills a rectangle of the given size with a colour and returns its
// dimensions, for backgrounds behind other content.
func fillRect(gtx layout.Context, size image.Point, c color.NRGBA) layout.Dimensions {
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{Size: size}
}

// widgetBox draws w inside a padded panel with a solid background colour.
func widgetBox(gtx layout.Context, bg color.NRGBA, pad unit.Dp, w layout.Widget) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return fillRect(gtx, gtx.Constraints.Min, bg)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(pad).Layout(gtx, w)
		}),
	)
}
