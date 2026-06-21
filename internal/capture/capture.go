// Package capture grabs a single frame of a monitor on Hyprland/Wayland and
// returns it as a normalized sRGB image. Several backends exist with different
// HDR behavior; see grim.go (SDR fallback) and the Wayland backends for the
// color-managed paths.
package capture

import (
	"context"
	"image"

	"warframe-overlay-linux/internal/hypr"
)

// Frame is a captured, already-normalized-to-sRGB image plus provenance.
type Frame struct {
	Image   *image.RGBA
	Monitor hypr.Monitor
	// Backend names which capture path produced this frame ("grim",
	// "screencopy", "ext-image-copy").
	Backend string
	// WasHDR is true if the source monitor was in HDR and a PQ->sRGB transform
	// was applied.
	WasHDR bool
}

// Capturer grabs one frame of the given monitor.
type Capturer interface {
	Capture(ctx context.Context, m hypr.Monitor) (*Frame, error)
	// Name identifies the backend for logging.
	Name() string
}
