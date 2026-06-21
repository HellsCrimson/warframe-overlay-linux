// Command wfo is the warframe-overlay-linux daemon: it watches EE.log, captures
// the relic reward screen, OCRs the rewards, prices them, and (Phase 3+) shows
// an overlay. Phase 1 prints results to stdout.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"warframe-overlay-linux/internal/capture"
	"warframe-overlay-linux/internal/config"
	"warframe-overlay-linux/internal/db"
	"warframe-overlay-linux/internal/hypr"
	"warframe-overlay-linux/internal/items"
	"warframe-overlay-linux/internal/logwatch"
	"warframe-overlay-linux/internal/ocr"
	"warframe-overlay-linux/internal/overlay"
	"warframe-overlay-linux/internal/pricing"
)

func main() {
	cfg := config.Default()
	flag.StringVar(&cfg.EELogPath, "eelog", cfg.EELogPath, "path to Warframe EE.log")
	flag.StringVar(&cfg.Monitor, "monitor", "", "force capture on this output (e.g. DP-4); empty = auto")
	flag.StringVar(&cfg.CapturePNGDir, "dump", "", "directory to write captured frames as PNG for debugging")
	flag.BoolVar(&cfg.NoOverlay, "no-overlay", false, "disable the on-screen overlay (stdout only)")
	flag.DurationVar(&cfg.OverlayDuration, "overlay-duration", cfg.OverlayDuration, "how long the overlay stays up")
	verbose := flag.Bool("v", false, "verbose logging")
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, cfg, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	hyprc := hypr.New()

	database, err := db.Load(db.Options{CacheDir: cfg.CacheDir, TTL: cfg.DataTTL, Logger: log})
	if err != nil {
		log.Warn("price database unavailable; names will be shown without prices", "err", err)
	}

	cap := capture.SelectBackend(ctx, hyprc, log)
	log.Info("capture backend selected", "backend", cap.Name())

	events, err := logwatch.Watch(ctx, logwatch.Options{
		Path:             cfg.EELogPath,
		PostTriggerDelay: cfg.PostTriggerDelay,
		Logger:           log,
	})
	if err != nil {
		return fmt.Errorf("logwatch: %w", err)
	}
	log.Info("watching EE.log", "path", cfg.EELogPath)

	var inflight sync.Mutex
	busy := false
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			if ev.Kind != "reward" {
				continue
			}
			inflight.Lock()
			if busy {
				inflight.Unlock()
				log.Debug("pipeline busy, coalescing trigger")
				continue
			}
			busy = true
			inflight.Unlock()

			go func() {
				defer func() {
					inflight.Lock()
					busy = false
					inflight.Unlock()
				}()
				if err := pipeline(ctx, cfg, hyprc, cap, database, log); err != nil {
					log.Error("pipeline failed", "err", err)
				}
			}()
		}
	}
}

func pipeline(ctx context.Context, cfg config.Config, hyprc *hypr.Client, cap capture.Capturer, database *db.Database, log *slog.Logger) error {
	pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	mon, err := hyprc.TargetMonitor(pctx, cfg.Monitor)
	if err != nil {
		return fmt.Errorf("select monitor: %w", err)
	}
	log.Info("capturing", "monitor", mon.Name, "hdr", mon.IsHDR(), "size", fmt.Sprintf("%dx%d", mon.Width, mon.Height))

	frame, err := cap.Capture(pctx, mon)
	if err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	if cfg.CapturePNGDir != "" {
		if p, err := capture.DumpPNG(cfg.CapturePNGDir, frame); err != nil {
			log.Warn("dump png failed", "err", err)
		} else {
			log.Info("frame dumped", "path", p)
		}
	}

	engine, err := ocr.NewEngine()
	if err != nil {
		return fmt.Errorf("ocr engine: %w", err)
	}
	defer engine.Close()

	names, err := engine.Recognize(frame.Image, 0)
	if err != nil {
		return fmt.Errorf("ocr: %w", err)
	}
	if len(names) == 0 {
		log.Warn("no reward names recognized")
		return nil
	}

	result := pricing.Evaluate(names, database)
	printResult(result)

	if !cfg.NoOverlay {
		labels := buildLabels(frame.Image.Bounds().Dx(), frame.Image.Bounds().Dy(), result)
		// Use the root ctx (not the capture-scoped pctx) so the overlay outlives
		// this pipeline run; show it without blocking re-triggers.
		go func() {
			if err := overlay.Show(ctx, mon, labels, cfg.OverlayDuration, log); err != nil {
				log.Warn("overlay failed", "err", err)
			}
		}()
	}
	return nil
}

// buildLabels positions one price label under each reward column.
func buildLabels(w, h int, r pricing.Result) []overlay.Label {
	cols := items.RewardColumns(w, h, len(r.Rewards))
	labels := make([]overlay.Label, 0, len(r.Rewards))
	for i, rw := range r.Rewards {
		if i >= len(cols) {
			break
		}
		name := rw.OCRName
		if rw.Item != nil {
			name = rw.Item.DropName
		}
		var value string
		if rw.Item != nil {
			value = fmt.Sprintf("%.0fp · %dd", rw.Plat(), rw.Ducats())
		} else {
			value = "no match"
		}
		c := cols[i]
		labels = append(labels, overlay.Label{
			Name:    name,
			Price:   value,
			CenterX: (c.Min.X + c.Max.X) / 2,
			Top:     c.Max.Y + 8,
			Best:    i == r.BestIndex,
		})
	}
	return labels
}

func printResult(r pricing.Result) {
	fmt.Println("── Relic rewards ─────────────────────────")
	for i, rw := range r.Rewards {
		marker := "  "
		if i == r.BestIndex {
			marker = "▶ "
		}
		name := rw.OCRName
		if rw.Item != nil {
			name = rw.Item.DropName
		}
		fmt.Printf("%s%-34s %6.1f plat  %4d ducats\n", marker, name, rw.Plat(), rw.Ducats())
	}
	fmt.Println("──────────────────────────────────────────")
}
