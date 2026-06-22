package logwatch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\r\n"); err != nil {
		t.Fatal(err)
	}
}

func TestRewardTriggerOnceAndNoReplay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "EE.log")
	// Pre-existing history must NOT replay on startup.
	appendLine(t, path, "12.000 Sys [Info]: Created /Lotus/Interface/ProjectionRewardChoice.swf")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := Watch(ctx, Options{
		Path:             path,
		PostTriggerDelay: 10 * time.Millisecond,
		Debounce:         500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Give the watcher a moment to seek to end.
	time.Sleep(100 * time.Millisecond)

	// Three co-firing markers for one screen -> exactly one reward event.
	appendLine(t, path, "20.0 Sys [Info]: Created /Lotus/Interface/ProjectionRewardChoice.swf")
	appendLine(t, path, "20.2 Sys [Info]: Created /Lotus/Interface/ProjectionRewardChoice.swf")

	got := collect(t, events, 1500*time.Millisecond)
	if rewardCount(got) != 1 {
		t.Fatalf("expected exactly 1 reward event, got %d (%v)", rewardCount(got), got)
	}
}

func TestSecondScreenAfterDebounce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "EE.log")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, _ := Watch(ctx, Options{
		Path:             path,
		PostTriggerDelay: 10 * time.Millisecond,
		Debounce:         200 * time.Millisecond,
	})
	time.Sleep(100 * time.Millisecond)

	appendLine(t, path, "1.0 Sys [Info]: Created /Lotus/Interface/ProjectionRewardChoice.swf")
	time.Sleep(400 * time.Millisecond) // exceed debounce window
	appendLine(t, path, "2.0 Sys [Info]: Created /Lotus/Interface/ProjectionRewardChoice.swf")

	got := collect(t, events, 1500*time.Millisecond)
	if rewardCount(got) != 2 {
		t.Fatalf("expected 2 reward events across debounce, got %d", rewardCount(got))
	}
}

func collect(t *testing.T, events <-chan Event, within time.Duration) []Event {
	t.Helper()
	var out []Event
	deadline := time.After(within)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			return out
		}
	}
}

func rewardCount(evs []Event) int {
	n := 0
	for _, e := range evs {
		if e.Kind == "reward" {
			n++
		}
	}
	return n
}
