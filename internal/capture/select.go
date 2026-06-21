package capture

import (
	"context"
	"fmt"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"warframe-overlay-linux/internal/hypr"
)

// SelectBackend chooses the capture backend. It prefers wlr-screencopy (which
// captures the native pixel format and tonemaps HDR correctly) when the
// compositor advertises it, and falls back to grim otherwise.
//
// TODO(phase2): add the ext-image-copy-capture-v1 + color-management path as the
// top preference once the cross-protocol bindings are generated.
func SelectBackend(ctx context.Context, hyprc *hypr.Client, log *slog.Logger) Capturer {
	return withHDRToggle(autoBackend(log), hyprc, log)
}

// autoBackend probes the compositor's globals once and picks the best capture
// path: ext-image-copy-capture (HDR-correct) > wlr-screencopy > grim.
func autoBackend(log *slog.Logger) Capturer {
	conn, err := connectWayland()
	if err != nil {
		log.Debug("wayland connect failed; using grim", "err", err)
		return &GrimCapturer{}
	}
	hasExt := conn.extSourceName != 0 && conn.extCopyName != 0 && conn.shm != nil
	hasScreencopy := conn.screencopyName != 0 && conn.shm != nil
	conn.Close()

	switch {
	case hasExt:
		log.Debug("using ext-image-copy-capture backend")
		return &ExtImageCopyCapturer{Logger: log}
	case hasScreencopy:
		log.Debug("using wlr-screencopy backend (no ext-image-copy; HDR may be wrong)")
		return &ScreencopyCapturer{Logger: log}
	default:
		log.Debug("using grim backend")
		return &GrimCapturer{}
	}
}

// withHDRToggle wraps a backend so HDR outputs are momentarily switched to SDR
// for capture (Hyprland exposes no working HDR capture path). It is a no-op
// passthrough for SDR monitors. Returns the inner backend unchanged if hyprc is
// nil (no way to drive the toggle).
func withHDRToggle(inner Capturer, hyprc *hypr.Client, log *slog.Logger) Capturer {
	if hyprc == nil {
		return inner
	}
	return &HDRToggleCapturer{Inner: inner, Hypr: hyprc, Logger: log}
}

// ByName returns a specific backend by name, for the -backend dev flag. When
// hyprc is non-nil the backend is wrapped with the HDR-toggle workaround. Pass
// the "-raw" forms (handled by the caller) to bypass the wrapper for debugging.
func ByName(name string, hyprc *hypr.Client, log *slog.Logger) (Capturer, error) {
	var inner Capturer
	switch name {
	case "", "auto":
		inner = autoBackend(log)
	case "ext-image-copy", "extimagecopy":
		inner = &ExtImageCopyCapturer{Logger: log}
	case "screencopy":
		inner = &ScreencopyCapturer{Logger: log}
	case "grim":
		inner = &GrimCapturer{}
	default:
		return nil, fmt.Errorf("unknown capture backend %q", name)
	}
	return withHDRToggle(inner, hyprc, log), nil
}

// DumpPNG writes a captured frame to dir as a timestamped PNG and returns the
// path. Used by the -dump flag and the wfo-capture dev tool.
func DumpPNG(dir string, frame *Frame) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("capture-%s-%s.png",
		frame.Monitor.Name,
		time.Now().Format("20060102-150405.000"))
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := png.Encode(f, frame.Image); err != nil {
		return "", err
	}
	return path, nil
}
