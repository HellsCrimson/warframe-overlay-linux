// Package logwatch tails Warframe's EE.log and emits a single debounced trigger
// when the relic/fissure reward-selection screen appears.
//
// EE.log is written by Proton (CRLF line endings). The leading number on each
// line is seconds-since-process-start, not wall-clock, so we never parse it for
// recency; instead we seek to the end of the file on startup and only react to
// newly appended lines. The file is truncated/recreated when the game relaunches,
// which we detect by size shrinking and by fsnotify rename/remove events.
package logwatch

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Markers that indicate the reward-selection screen is being shown. Matching any
// substring is enough; the game typically writes more than one per screen, which
// is why we debounce.
var rewardMarkers = []string{
	"Pause countdown done",
	"Got rewards",
	"Created /Lotus/Interface/ProjectionRewardChoice.swf",
}

// closeMarkers indicate the reward screen (or the surrounding menu) closed, used
// by the overlay to auto-dismiss. These are best-effort.
var closeMarkers = []string{
	"Script [Info]: ProjectionRewardChoice.lua: Reward chosen",
	"AvatarPaused: false",
}

// Event is emitted on the channel returned by Watch.
type Event struct {
	// Kind is "reward" when the selection screen appeared, "close" when it went
	// away.
	Kind string
	At   time.Time
}

// Options configures a Watcher.
type Options struct {
	Path string
	// PostTriggerDelay is applied before the reward Event is emitted, so the
	// screen has time to finish drawing.
	PostTriggerDelay time.Duration
	// Debounce coalesces the multiple markers written for a single screen.
	Debounce time.Duration
	Logger   *slog.Logger
	// Now is injectable for tests; defaults to time.Now.
	Now func() time.Time
}

// Watch tails the EE.log at opts.Path and sends Events on the returned channel
// until ctx is cancelled. The channel is closed when watching stops.
func Watch(ctx context.Context, opts Options) (<-chan Event, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Debounce == 0 {
		opts.Debounce = 10 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	out := make(chan Event, 4)
	go func() {
		defer close(out)
		w := &watcher{opts: opts, out: out}
		w.run(ctx)
	}()
	return out, nil
}

type watcher struct {
	opts        Options
	out         chan<- Event
	offset      int64
	carry       string // partial trailing line not yet terminated by '\n'
	lastTrigger time.Time
}

func (w *watcher) run(ctx context.Context) {
	log := w.opts.Logger
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error("logwatch: cannot create fsnotify watcher", "err", err)
		return
	}
	defer fsw.Close()

	// Open and seek to end so we don't replay history on startup.
	if err := w.openAtEnd(); err != nil {
		log.Warn("logwatch: EE.log not open yet, will retry", "path", w.opts.Path, "err", err)
	}
	w.addWatch(fsw)

	// A ticker provides a safety net for missed fsnotify events (the game
	// buffers writes) and drives reopen retries while the file is missing.
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-fsw.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Rename|fsnotify.Remove) != 0 {
				// Log rotated/recreated by a relaunch; reopen from start.
				w.reopen(fsw, true)
				continue
			}
			w.drain(ctx)
		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			log.Warn("logwatch: fsnotify error", "err", err)
		case <-tick.C:
			w.drain(ctx)
		}
	}
}

// openAtEnd opens the file and positions the read offset at EOF.
func (w *watcher) openAtEnd() error {
	f, err := os.Open(w.opts.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	w.offset = end
	w.carry = ""
	return nil
}

func (w *watcher) addWatch(fsw *fsnotify.Watcher) {
	// Watch the file if present, otherwise watch the directory so we learn when
	// it is (re)created.
	if err := fsw.Add(w.opts.Path); err != nil {
		if dir := dirOf(w.opts.Path); dir != "" {
			_ = fsw.Add(dir)
		}
	}
}

func (w *watcher) reopen(fsw *fsnotify.Watcher, fromStart bool) {
	if fromStart {
		w.offset = 0
		w.carry = ""
	}
	w.addWatch(fsw)
	w.drain(context.Background())
}

// drain reads any bytes appended since the last offset and scans them for
// markers.
func (w *watcher) drain(ctx context.Context) {
	f, err := os.Open(w.opts.Path)
	if err != nil {
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return
	}
	size := info.Size()
	if size < w.offset {
		// Truncated/rotated: start over from the beginning.
		w.offset = 0
		w.carry = ""
	}
	if size == w.offset {
		return
	}
	if _, err := f.Seek(w.offset, io.SeekStart); err != nil {
		return
	}

	reader := bufio.NewReader(f)
	rewardSeen := false
	closeSeen := false
	for {
		chunk, err := reader.ReadString('\n')
		if len(chunk) > 0 {
			w.offset += int64(len(chunk))
			if !strings.HasSuffix(chunk, "\n") {
				// Partial trailing line; keep it for next time.
				w.carry += chunk
				break
			}
			line := w.carry + chunk
			w.carry = ""
			line = strings.TrimRight(line, "\r\n")
			if containsAny(line, rewardMarkers) {
				rewardSeen = true
			}
			if containsAny(line, closeMarkers) {
				closeSeen = true
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				w.opts.Logger.Warn("logwatch: read error", "err", err)
			}
			break
		}
	}

	now := w.opts.Now()
	if closeSeen {
		w.emit(ctx, Event{Kind: "close", At: now})
	}
	if rewardSeen && now.Sub(w.lastTrigger) >= w.opts.Debounce {
		w.lastTrigger = now
		w.opts.Logger.Info("logwatch: reward screen detected", "delay", w.opts.PostTriggerDelay)
		// Apply the post-trigger delay before emitting, but don't block the
		// watch loop.
		go func() {
			if w.opts.PostTriggerDelay > 0 {
				t := time.NewTimer(w.opts.PostTriggerDelay)
				defer t.Stop()
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
			w.emit(ctx, Event{Kind: "reward", At: w.opts.Now()})
		}()
	}
}

func (w *watcher) emit(ctx context.Context, ev Event) {
	select {
	case <-ctx.Done():
	case w.out <- ev:
	}
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func dirOf(path string) string {
	if i := strings.LastIndexByte(path, '/'); i > 0 {
		return path[:i]
	}
	return ""
}
