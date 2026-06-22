package trades

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// pollCount waits up to timeout for the store to reach at least want trades,
// returning the final count.
func pollCount(t *testing.T, store *Store, want int, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if n := len(store.All()); n >= want || time.Now().After(deadline) {
			return n
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestWatchSiblingEventNoDuplicate reproduces the armory bug: Warframe rewrites
// a *sibling* file next to EE.log on scene transitions, which the directory
// watch reports as a Create/Rename/Remove. That must not reset the read offset
// and re-record every past trade.
func TestWatchSiblingEventNoDuplicate(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "EE.log")
	if err := os.WriteFile(logPath, []byte(sample+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, logPath, store, func() {})

	if n := pollCount(t, store, 1, 2*time.Second); n != 1 {
		t.Fatalf("after initial scan: got %d trades, want 1", n)
	}

	// Touch a sibling file in the watched directory (what the armory does). With
	// the bug, the resulting Remove event reset the offset and re-scanned the
	// whole log within milliseconds, duplicating the trade.
	sib := filepath.Join(dir, "ee.cfg")
	if err := os.WriteFile(sib, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sib); err != nil {
		t.Fatal(err)
	}

	// Settle past a ticker cycle so any erroneous re-scan would have happened.
	time.Sleep(1300 * time.Millisecond)
	if n := len(store.All()); n != 1 {
		t.Fatalf("sibling-file event duplicated the trade: got %d, want 1", n)
	}

	// The watcher must still be live: a genuine appended trade is recorded.
	appendLog(t, logPath, sample+"\n")
	if n := pollCount(t, store, 2, 3*time.Second); n != 2 {
		t.Fatalf("after appending a second trade: got %d, want 2", n)
	}
}

// TestWatchTruncationRescans verifies a genuine in-place truncation (a new game
// session reusing the same file, so size drops below our read offset) is
// detected and the new content re-scanned.
func TestWatchTruncationRescans(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "EE.log")
	// Two trades so the offset advances well past a single-trade file.
	if err := os.WriteFile(logPath, []byte(sample+"\n"+sample+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, logPath, store, func() {})

	if n := pollCount(t, store, 2, 2*time.Second); n != 2 {
		t.Fatalf("after initial scan: got %d trades, want 2", n)
	}

	// Truncate to a smaller single-trade file: size now < offset, which must
	// trigger a re-scan from the top, recording the one trade in the new file.
	if err := os.WriteFile(logPath, []byte(sample+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if n := pollCount(t, store, 3, 3*time.Second); n != 3 {
		t.Fatalf("after truncation: got %d trades, want 3 (2 + 1 re-scanned)", n)
	}
}

func appendLog(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}
