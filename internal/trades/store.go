package trades

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Store persists recorded trades plus the EE.log read offset, so each trade is
// recorded exactly once across app restarts.
type Store struct {
	path string

	mu     sync.Mutex
	Offset int64   `json:"offset"`
	Trades []Trade `json:"trades"`
}

// OpenStore loads (or creates) the trade store under dir.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "trades.json")}
	data, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(data, s)
	}
	return s, nil
}

// All returns a copy of the recorded trades, oldest first.
func (s *Store) All() []Trade {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Trade, len(s.Trades))
	copy(out, s.Trades)
	return out
}

// Append adds trades and persists, returning the new total.
func (s *Store) Append(ts ...Trade) int {
	s.mu.Lock()
	s.Trades = append(s.Trades, ts...)
	n := len(s.Trades)
	s.save()
	s.mu.Unlock()
	return n
}

// SetOffset records the EE.log read position and persists.
func (s *Store) SetOffset(off int64) {
	s.mu.Lock()
	s.Offset = off
	s.save()
	s.mu.Unlock()
}

// GetOffset returns the saved EE.log read position.
func (s *Store) GetOffset() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Offset
}

// save writes the store (caller holds the lock).
func (s *Store) save() {
	data, err := json.MarshalIndent(struct {
		Offset int64   `json:"offset"`
		Trades []Trade `json:"trades"`
	}{s.Offset, s.Trades}, "", "  ")
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, s.path)
	}
}

// Summary aggregates the recorded trades.
type Summary struct {
	Count      int
	PlatIn     int // platinum received (sales)
	PlatOut    int // platinum spent (purchases)
	NetPlat    int
	Cumulative []int // running net platinum after each trade, oldest first
}

// Summarize computes totals and the cumulative net-platinum series.
func Summarize(ts []Trade) Summary {
	var s Summary
	run := 0
	for _, t := range ts {
		s.Count++
		d := t.PlatDelta()
		if d > 0 {
			s.PlatIn += d
		} else {
			s.PlatOut += -d
		}
		run += d
		s.Cumulative = append(s.Cumulative, run)
	}
	s.NetPlat = run
	return s
}
