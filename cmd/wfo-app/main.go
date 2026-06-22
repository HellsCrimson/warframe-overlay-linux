// Command wfo-app is the native (Gio) control app for warframe-overlay-linux.
// It is the growing AlecaFrame-style companion: it starts with the player
// inventory and will gain mastery tracking, trade message generation and trade
// analytics.
package main

import (
	"flag"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/unit"

	"warframe-overlay-linux/internal/ui"
)

func main() {
	invFile := flag.String("inventory-file", "", "load inventory from a saved JSON file instead of the running game (dev)")
	tab := flag.String("tab", "", "initial tab to show (e.g. Mastery)")
	flag.Parse()

	go func() {
		w := new(app.Window)
		w.Option(app.Title("Warframe Overlay — Companion"), app.Size(unit.Dp(1040), unit.Dp(720)))
		if err := ui.Run(w, ui.Config{InventoryFile: *invFile, StartTab: *tab}); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}
