// Package config holds runtime configuration and well-known paths for the
// warframe-overlay-linux tool.
package config

import (
	"os"
	"path/filepath"
	"time"
)

// Config is the resolved runtime configuration for a single process run.
type Config struct {
	// EELogPath is the absolute path to Warframe's EE.log.
	EELogPath string

	// CacheDir is where downloaded price/item data is cached.
	CacheDir string

	// Monitor, when non-empty, forces capture/overlay onto this output
	// (e.g. "DP-4"). When empty the tool auto-selects the monitor that
	// Warframe is focused on.
	Monitor string

	// PostTriggerDelay is how long to wait after detecting the reward
	// screen in EE.log before capturing, to let the screen finish drawing.
	PostTriggerDelay time.Duration

	// DataTTL controls how stale cached price/item data may be before a
	// refresh is attempted. Stale data is still served if refresh fails.
	DataTTL time.Duration

	// EnableInventory turns on the (optional, unsanctioned) inventory module.
	EnableInventory bool

	// CapturePNGDir, when non-empty, writes each captured frame there as a
	// PNG for debugging the capture/color pipeline.
	CapturePNGDir string

	// NoOverlay disables the on-screen overlay (stdout output only).
	NoOverlay bool

	// OverlayDuration is how long the price overlay stays on screen.
	OverlayDuration time.Duration
}

// Default returns a Config populated with sensible defaults for this machine.
func Default() Config {
	return Config{
		EELogPath:        DefaultEELogPath(),
		CacheDir:         DefaultCacheDir(),
		PostTriggerDelay: 1500 * time.Millisecond,
		DataTTL:          24 * time.Hour,
		OverlayDuration:  8 * time.Second,
	}
}

// DefaultEELogPath returns the conventional Proton/Steam location of EE.log.
// Steam appid 230410 is Warframe.
func DefaultEELogPath() string {
	if p := os.Getenv("WFO_EELOG"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home,
		".local/share/Steam/steamapps/compatdata/230410/pfx/drive_c/users/steamuser/AppData/Local/Warframe/EE.log")
}

// DefaultCacheDir returns $XDG_CACHE_HOME/warframe-overlay-linux (or the
// ~/.cache fallback).
func DefaultCacheDir() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "warframe-overlay-linux")
}
