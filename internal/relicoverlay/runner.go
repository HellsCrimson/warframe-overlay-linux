// Package relicoverlay runs the in-game relic-reward overlay pipeline: it watches
// EE.log, captures the reward screen, OCRs the rewards, prices them, and shows a
// click-through overlay highlighting the best pick. It is started as a background
// service by the desktop app.
package relicoverlay

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"warframe-overlay-linux/internal/capture"
	"warframe-overlay-linux/internal/db"
	"warframe-overlay-linux/internal/hypr"
	"warframe-overlay-linux/internal/items"
	"warframe-overlay-linux/internal/logwatch"
	"warframe-overlay-linux/internal/ocr"
	"warframe-overlay-linux/internal/overlay"
	"warframe-overlay-linux/internal/pricing"
)

// Options configures the overlay runner.
type Options struct {
	EELogPath        string
	Monitor          string        // force capture output (empty = auto)
	DumpDir          string        // write captured frames here for debugging (empty = off)
	NoOverlay        bool          // skip the on-screen overlay
	OverlayDuration  time.Duration // how long the overlay stays up
	PostTriggerDelay time.Duration // settle delay after the reward marker
	CacheDir         string
	DataTTL          time.Duration
	Logger           *slog.Logger
	// Enrich decorates a reward (by its drop name) with ownership, mastery/craft
	// status and prime-set membership. May be nil (overlay shows price only).
	Enrich func(dropName string) RewardInfo
}

// RewardInfo is the decoration for one reward: ownership, whether the item is
// mastered/crafted, and the other parts of its prime set.
type RewardInfo struct {
	Owned      int
	OwnedKnown bool
	Mastered   bool
	Crafted    bool
	SetName    string
	SetParts   []SetPart
}

// SetPart is one part of a reward's prime set and whether the player owns it.
type SetPart struct {
	Name  string
	Owned bool
}

// Run watches EE.log and shows the overlay until ctx is cancelled.
func Run(ctx context.Context, opts Options) error {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	if opts.OverlayDuration == 0 {
		opts.OverlayDuration = 8 * time.Second
	}
	if opts.PostTriggerDelay == 0 {
		opts.PostTriggerDelay = 1500 * time.Millisecond
	}

	hyprc := hypr.New()
	database, err := db.Load(db.Options{CacheDir: opts.CacheDir, TTL: opts.DataTTL, Logger: log})
	if err != nil {
		log.Warn("relic overlay: price database unavailable", "err", err)
	}
	cap := capture.SelectBackend(ctx, hyprc, log)
	log.Info("relic overlay: capture backend", "backend", cap.Name())

	events, err := logwatch.Watch(ctx, logwatch.Options{
		Path:             opts.EELogPath,
		PostTriggerDelay: opts.PostTriggerDelay,
		Logger:           log,
	})
	if err != nil {
		return fmt.Errorf("logwatch: %w", err)
	}
	log.Info("relic overlay: watching EE.log", "path", opts.EELogPath)

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
				if err := pipeline(ctx, opts, hyprc, cap, database, log); err != nil {
					log.Error("relic overlay: pipeline failed", "err", err)
				}
			}()
		}
	}
}

func pipeline(ctx context.Context, opts Options, hyprc *hypr.Client, cap capture.Capturer, database *db.Database, log *slog.Logger) error {
	pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	mon, err := hyprc.TargetMonitor(pctx, opts.Monitor)
	if err != nil {
		return fmt.Errorf("select monitor: %w", err)
	}
	log.Info("relic overlay: capturing", "monitor", mon.Name, "hdr", mon.IsHDR())

	frame, err := cap.Capture(pctx, mon)
	if err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	if opts.DumpDir != "" {
		if p, err := capture.DumpPNG(opts.DumpDir, frame); err != nil {
			log.Warn("dump png failed", "err", err)
		} else {
			log.Info("relic overlay: frame dumped", "path", p)
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
		log.Warn("relic overlay: no reward names recognized")
		return nil
	}

	result := pricing.Evaluate(names, database)
	logResult(log, result)

	if !opts.NoOverlay {
		labels := buildLabels(frame.Image.Bounds().Dx(), frame.Image.Bounds().Dy(), result, opts.Enrich)
		go func() {
			if err := overlay.Show(ctx, mon, labels, opts.OverlayDuration, log); err != nil {
				log.Warn("relic overlay: show failed", "err", err)
			}
		}()
	}
	return nil
}

func buildLabels(w, h int, r pricing.Result, enrich func(string) RewardInfo) []overlay.Label {
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
		value := "no match"
		if rw.Item != nil {
			value = fmt.Sprintf("%.0fp · %dd", rw.Plat(), rw.Ducats())
		}
		c := cols[i]
		label := overlay.Label{
			Name: name, Price: value,
			CenterX: (c.Min.X + c.Max.X) / 2, Top: c.Max.Y + 8,
			Best: i == r.BestIndex,
		}
		if enrich != nil && rw.Item != nil {
			info := enrich(rw.Item.DropName)
			label.OwnedKnown = info.OwnedKnown
			label.Owned = info.Owned
			label.Mastered = info.Mastered
			label.Crafted = info.Crafted
			label.SetName = info.SetName
			for _, p := range info.SetParts {
				label.SetParts = append(label.SetParts, overlay.SetPart{Name: p.Name, Owned: p.Owned})
			}
		}
		labels = append(labels, label)
	}
	return labels
}

func logResult(log *slog.Logger, r pricing.Result) {
	for i, rw := range r.Rewards {
		name := rw.OCRName
		if rw.Item != nil {
			name = rw.Item.DropName
		}
		log.Info("relic reward",
			"name", name, "plat", rw.Plat(), "ducats", rw.Ducats(), "best", i == r.BestIndex)
	}
}
