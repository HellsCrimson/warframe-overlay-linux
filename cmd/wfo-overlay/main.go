// Command wfo-overlay shows a sample price overlay on a monitor for a few
// seconds, to verify the layer-shell + pangocairo rendering on screen.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"warframe-overlay-linux/internal/hypr"
	"warframe-overlay-linux/internal/items"
	"warframe-overlay-linux/internal/overlay"
)

func main() {
	monitor := flag.String("monitor", "", "output to show on (e.g. DP-4); empty = auto")
	secs := flag.Int("secs", 5, "seconds to show the overlay")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*secs+5)*time.Second)
	defer cancel()

	hyprc := hypr.New()
	mon, err := hyprc.TargetMonitor(ctx, *monitor)
	if err != nil {
		log.Error("select monitor", "err", err)
		os.Exit(1)
	}

	// Sample labels positioned under each reward column for this monitor.
	cols := items.RewardColumns(mon.Width, mon.Height, 4)
	names := []string{"Bronco Prime Receiver", "Braton Prime Stock", "Cobra & Crane Prime Hilt", "Bronco Prime Blueprint"}
	prices := []string{"12p · 15d", "8p · 25d", "45p · 45d", "5p · 15d"}
	best := 2
	var labels []overlay.Label
	for i, c := range cols {
		labels = append(labels, overlay.Label{
			Name:    names[i],
			Price:   prices[i],
			CenterX: (c.Min.X + c.Max.X) / 2,
			Top:     c.Max.Y + 8,
			Best:    i == best,
		})
	}

	log.Info("showing overlay", "monitor", mon.Name, "secs", *secs)
	if err := overlay.Show(ctx, mon, labels, time.Duration(*secs)*time.Second, log); err != nil {
		log.Error("overlay", "err", err)
		os.Exit(1)
	}
	log.Info("overlay closed")
}
