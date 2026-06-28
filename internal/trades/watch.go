package trades

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watch tails the EE.log at path, parsing completed trades into the store. It
// resumes from the store's saved offset (so a trade is recorded once), and
// re-scans from the start when the log is truncated/rotated (a new game session).
// onTrade is called with the trades recorded from each newly appended log
// chunk, but NOT for the initial catch-up scan at startup (those trades are
// already in the store; replaying them would, e.g., re-close market orders).
// Blocks until ctx is done.
func Watch(ctx context.Context, path string, store *Store, onTrade func(newTrades []Trade)) {
	w := &watcher{path: path, store: store, parser: &Parser{}, onTrade: onTrade}
	w.offset = store.GetOffset()
	w.run(ctx)
}

type watcher struct {
	path    string
	store   *Store
	parser  *Parser
	onTrade func(newTrades []Trade)
	offset  int64
	carry   string
	live    bool // false during the initial catch-up scan
}

func (w *watcher) run(ctx context.Context) {
	w.drain()     // initial scan (resumes from saved offset; 0 on first run)
	w.live = true // subsequent drains carry live trades

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer fsw.Close()
	_ = fsw.Add(w.path)
	if dir := dirOf(w.path); dir != "" {
		_ = fsw.Add(dir)
	}
	cleanPath := filepath.Clean(w.path)

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-fsw.Events:
			if !ok {
				return
			}
			// We also watch the parent directory (to notice the log being
			// recreated after rotation), so we receive events for sibling files
			// too. Warframe rewrites other files in that directory on scene
			// transitions (e.g. entering/leaving the arsenal); acting on those
			// would reset our read offset and re-scan the whole log, recording
			// every past trade again. Only react to events for EE.log itself.
			if filepath.Clean(ev.Name) != cleanPath {
				continue
			}
			if ev.Op&(fsnotify.Rename|fsnotify.Remove|fsnotify.Create) != 0 {
				// The log was rotated/recreated: read the new file from the top.
				// (Plain truncation in place is handled by the size check in drain.)
				w.offset, w.carry = 0, ""
				_ = fsw.Add(w.path)
			}
			w.drain()
		case <-fsw.Errors:
		case <-tick.C:
			w.drain()
		}
	}
}

// drain reads newly appended complete lines, feeds the parser, and records any
// completed trades.
func (w *watcher) drain() {
	f, err := os.Open(w.path)
	if err != nil {
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return
	}
	if info.Size() < w.offset { // truncated/rotated
		w.offset, w.carry = 0, ""
	}
	if info.Size() == w.offset {
		return
	}
	if _, err := f.Seek(w.offset, io.SeekStart); err != nil {
		return
	}

	r := bufio.NewReader(f)
	var found []Trade
	for {
		chunk, err := r.ReadString('\n')
		if len(chunk) > 0 {
			w.offset += int64(len(chunk))
			if !strings.HasSuffix(chunk, "\n") {
				w.carry += chunk // partial line; wait for the rest
				break
			}
			line := strings.TrimRight(w.carry+chunk, "\r\n")
			w.carry = ""
			if t := w.parser.Line(line); t != nil {
				found = append(found, *t)
			}
		}
		if err != nil {
			break
		}
	}

	if len(found) > 0 {
		w.store.Append(found...)
	}
	w.store.SetOffset(w.offset)
	if len(found) > 0 && w.live && w.onTrade != nil {
		w.onTrade(found)
	}
}

func dirOf(path string) string {
	if i := strings.LastIndexByte(path, '/'); i > 0 {
		return path[:i]
	}
	return ""
}

var _ = errors.Is // reserved for future error handling
