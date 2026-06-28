// Command wfo-desktop is the warframe-overlay-linux companion app, built with
// Wails3 (Go backend + web frontend). It binds the shared domain packages as a
// Service and renders the Inventory / Mastery / Trades / Analytics views, while
// running the in-game relic-reward overlay as a background service.
package main

import (
	"embed"
	"flag"
	"log"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"warframe-overlay-linux/internal/config"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	cfg := config.Default()
	flag.StringVar(&cfg.EELogPath, "eelog", cfg.EELogPath, "path to Warframe EE.log")
	flag.StringVar(&cfg.Monitor, "monitor", "", "force capture/overlay on this output (e.g. DP-4); empty = auto")
	flag.StringVar(&cfg.CapturePNGDir, "dump", "", "directory to write captured frames as PNG for debugging")
	noOverlay := flag.Bool("no-overlay", false, "disable the on-screen relic overlay")
	invFile := flag.String("inventory-file", os.Getenv("WFO_INVENTORY_FILE"), "load inventory from a saved JSON file instead of the running game (dev)")
	tab := flag.String("tab", os.Getenv("WFO_TAB"), "initial tab to show (e.g. Mastery)")
	verbose := flag.Bool("v", false, "verbose logging")
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	cfg.NoOverlay = *noOverlay

	svc := NewService(serviceOptions{cfg: cfg, inventoryFile: *invFile, logger: logger})

	app := application.New(application.Options{
		Name:        "Warframe Companion",
		Description: "Inventory, mastery, trades and analytics for Warframe on Linux",
		// On exit, go invisible on warframe.market so the account isn't left
		// appearing online.
		OnShutdown: svc.Shutdown,
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Warframe Companion",
		Width:            1100,
		Height:           760,
		BackgroundColour: application.NewRGB(20, 21, 26),
		URL:              "/?tab=" + *tab,
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
