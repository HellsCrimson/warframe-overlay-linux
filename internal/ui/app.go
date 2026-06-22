// Package ui implements the Gio control app: an AlecaFrame-style companion that
// starts with the player inventory and will grow mastery/trade/analytics views.
package ui

import (
	"context"
	"image/color"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"warframe-overlay-linux/internal/inventory"
)

// Config configures the app.
type Config struct {
	// InventoryFile, if set, loads inventory from a saved JSON file instead of
	// scraping the running game (development).
	InventoryFile string
}

// Run drives the Gio window event loop until the window is destroyed.
func Run(w *app.Window, cfg Config) error {
	a := newApp(cfg, w.Invalidate)
	if cfg.InventoryFile != "" {
		a.startLoad() // auto-load from file on startup
	}
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			a.Layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

type section struct {
	name  string
	ready bool
}

var sections = []section{
	{"Inventory", true},
	{"Mastery", false},
	{"Trades", false},
	{"Analytics", false},
}

// App holds all UI state.
type App struct {
	th         *material.Theme
	cfg        Config
	invalidate func()

	sel     int
	navBtns []widget.Clickable

	loadBtn widget.Clickable
	search  widget.Editor
	invList widget.List

	mu      sync.Mutex
	inv     *inventory.Inventory
	loading bool
	loadErr error
}

func newApp(cfg Config, invalidate func()) *App {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette = material.Palette{
		Bg:         rgb(0x14151a),
		Fg:         rgb(0xe6e6ea),
		ContrastBg: rgb(0xf2b134), // Warframe-ish gold accent
		ContrastFg: rgb(0x10100a),
	}
	a := &App{th: th, cfg: cfg, invalidate: invalidate}
	a.navBtns = make([]widget.Clickable, len(sections))
	a.search.SingleLine = true
	a.invList.Axis = layout.Vertical
	return a
}

// startLoad loads the inventory in the background (from file if configured, else
// by scraping the running game) and repaints when done.
func (a *App) startLoad() {
	a.mu.Lock()
	if a.loading {
		a.mu.Unlock()
		return
	}
	a.loading = true
	a.loadErr = nil
	a.mu.Unlock()

	go func() {
		var (
			inv *inventory.Inventory
			err error
		)
		if a.cfg.InventoryFile != "" {
			inv, err = inventory.LoadFile(a.cfg.InventoryFile)
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			inv, err = inventory.Load(ctx)
		}
		a.mu.Lock()
		a.inv, a.loadErr, a.loading = inv, err, false
		a.mu.Unlock()
		a.invalidate()
	}()
}

// Layout draws the whole window: a sidebar and the selected content view.
func (a *App) Layout(gtx layout.Context) layout.Dimensions {
	// Background fill.
	paintRect(gtx, a.th.Palette.Bg)

	// Handle nav clicks.
	for i := range a.navBtns {
		if a.navBtns[i].Clicked(gtx) && sections[i].ready {
			a.sel = i
		}
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutSidebar(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unitDp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				switch sections[a.sel].name {
				case "Inventory":
					return a.layoutInventory(gtx)
				default:
					return a.layoutPlaceholder(gtx, sections[a.sel].name)
				}
			})
		}),
	)
}

func (a *App) layoutPlaceholder(gtx layout.Context, name string) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		l := material.H6(a.th, name+" — coming soon")
		l.Color = rgb(0x8a8a93)
		return l.Layout(gtx)
	})
}

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{R: byte(v >> 16), G: byte(v >> 8), B: byte(v), A: 0xff}
}
