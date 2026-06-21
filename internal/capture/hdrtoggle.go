package capture

import (
	"context"
	"log/slog"
	"time"

	"warframe-overlay-linux/internal/hypr"
)

// HDRToggleCapturer wraps another backend to work around Hyprland's broken HDR
// screen capture (as of 0.55.x): every capture protocol only exposes 8-bit
// ARGB/XRGB buffers for an HDR output, and the compositor's HDR->8bit conversion
// crushes the image to near-black. The only reliable fix is to momentarily drop
// the output to SDR (cm,srgb), capture a correct 8-bit frame, then restore HDR.
//
// For SDR monitors this is a transparent passthrough to the inner backend.
//
// Cost: switching color mode triggers a brief (~0.5s) modeset flicker on the
// target output. That is acceptable for the relic reward screen, which is
// captured ~1.5s after it appears and stays up for many seconds.
type HDRToggleCapturer struct {
	Inner  Capturer
	Hypr   *hypr.Client
	Logger *slog.Logger
	Settle time.Duration // delay after switching to SDR before capturing
}

func (h *HDRToggleCapturer) Name() string { return h.Inner.Name() + "+hdr-toggle" }

func (h *HDRToggleCapturer) Capture(ctx context.Context, m hypr.Monitor) (*Frame, error) {
	log := h.Logger
	if log == nil {
		log = slog.Default()
	}
	if !m.IsHDR() {
		return h.Inner.Capture(ctx, m)
	}

	settle := h.Settle
	if settle == 0 {
		settle = 500 * time.Millisecond
	}

	// Record original state for restore.
	origPreset := m.ColorManagementPreset
	if origPreset == "" {
		origPreset = "hdr"
	}
	origBitdepth := 8
	if m.Is10Bit() {
		origBitdepth = 10
	}

	restore := func() {
		// Use a fresh context so restore still runs if ctx was cancelled.
		rctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.Hypr.SetMonitorColorMode(rctx, m, origPreset, origBitdepth); err != nil {
			log.Error("FAILED TO RESTORE HDR; run `hyprctl keyword monitor` to fix", "monitor", m.Name, "err", err)
		} else {
			log.Debug("restored HDR", "monitor", m.Name, "preset", origPreset, "bitdepth", origBitdepth)
		}
	}

	log.Debug("temporarily switching output to SDR for capture", "monitor", m.Name)
	if err := h.Hypr.SetMonitorColorMode(ctx, m, "srgb", 8); err != nil {
		return nil, err
	}
	defer restore()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(settle):
	}

	frame, err := h.Inner.Capture(ctx, m)
	if err != nil {
		return nil, err
	}
	frame.WasHDR = true
	return frame, nil
}
