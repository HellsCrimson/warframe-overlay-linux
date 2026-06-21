// Package hypr is a thin client over `hyprctl -j` used to discover monitor
// geometry, HDR/color state, and the currently focused window. This is our
// reliable oracle for whether a capture target is in HDR and how to scale the
// reward-box geometry.
package hypr

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Monitor mirrors the subset of `hyprctl monitors -j` fields we care about.
type Monitor struct {
	ID                    int     `json:"id"`
	Name                  string  `json:"name"`
	Width                 int     `json:"width"`
	Height                int     `json:"height"`
	RefreshRate           float64 `json:"refreshRate"`
	X                     int     `json:"x"`
	Y                     int     `json:"y"`
	Scale                 float64 `json:"scale"`
	Focused               bool    `json:"focused"`
	CurrentFormat         string  `json:"currentFormat"`
	ColorManagementPreset string  `json:"colorManagementPreset"`
	// SDR luminance fields (present under HDR) used as tonemap reference white.
	SDRMaxLuminance float64 `json:"sdrMaxLuminance"`
	SDRBrightness   float64 `json:"sdrBrightness"`
	SDRSaturation   float64 `json:"sdrSaturation"`
	SDRMinLuminance float64 `json:"sdrMinLuminance"`
}

// IsHDR reports whether this monitor is currently in an HDR color-management
// mode, which means screen-capture buffers need PQ->sRGB conversion.
func (m Monitor) IsHDR() bool {
	preset := strings.ToLower(m.ColorManagementPreset)
	if strings.Contains(preset, "hdr") {
		return true
	}
	// Fall back to the pixel format: 10-bit/fp16 formats imply a wide/HDR
	// pipeline even if the preset string is unexpected.
	switch strings.ToUpper(m.CurrentFormat) {
	case "XBGR2101010", "XRGB2101010", "ABGR2101010", "ARGB2101010":
		return true
	}
	return false
}

// Client invokes hyprctl. The binary path is configurable for testing.
type Client struct {
	Bin string
}

// New returns a Client using the `hyprctl` found on PATH.
func New() *Client { return &Client{Bin: "hyprctl"} }

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	bin := c.Bin
	if bin == "" {
		bin = "hyprctl"
	}
	out, err := exec.CommandContext(ctx, bin, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("hyprctl %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// Monitors returns all monitors known to Hyprland.
func (c *Client) Monitors(ctx context.Context) ([]Monitor, error) {
	out, err := c.run(ctx, "-j", "monitors")
	if err != nil {
		return nil, err
	}
	var mons []Monitor
	if err := json.Unmarshal(out, &mons); err != nil {
		return nil, fmt.Errorf("decode monitors: %w", err)
	}
	return mons, nil
}

// ActiveWindow mirrors the subset of `hyprctl activewindow -j` we need to pick
// the monitor Warframe is on.
type ActiveWindow struct {
	Class      string `json:"class"`
	Title      string `json:"title"`
	Monitor    int    `json:"monitor"`
	At         [2]int `json:"at"`
	Size       [2]int `json:"size"`
	Fullscreen int    `json:"fullscreen"`
}

// ActiveWindow returns the currently focused window, or a zero value with a nil
// error if nothing is focused.
func (c *Client) ActiveWindow(ctx context.Context) (ActiveWindow, error) {
	out, err := c.run(ctx, "-j", "activewindow")
	if err != nil {
		return ActiveWindow{}, err
	}
	var w ActiveWindow
	if len(out) == 0 || string(out) == "{}\n" {
		return ActiveWindow{}, nil
	}
	if err := json.Unmarshal(out, &w); err != nil {
		return ActiveWindow{}, fmt.Errorf("decode activewindow: %w", err)
	}
	return w, nil
}

// TargetMonitor chooses which monitor to capture. Selection order:
//  1. forced name, if set and present;
//  2. the monitor index reported by the focused Warframe window;
//  3. the focused monitor;
//  4. the first monitor.
func (c *Client) TargetMonitor(ctx context.Context, forcedName string) (Monitor, error) {
	mons, err := c.Monitors(ctx)
	if err != nil {
		return Monitor{}, err
	}
	if len(mons) == 0 {
		return Monitor{}, fmt.Errorf("no monitors reported by hyprctl")
	}
	if forcedName != "" {
		for _, m := range mons {
			if m.Name == forcedName {
				return m, nil
			}
		}
		return Monitor{}, fmt.Errorf("forced monitor %q not found", forcedName)
	}

	if w, err := c.ActiveWindow(ctx); err == nil && isWarframe(w) {
		for _, m := range mons {
			if m.ID == w.Monitor {
				return m, nil
			}
		}
	}
	for _, m := range mons {
		if m.Focused {
			return m, nil
		}
	}
	return mons[0], nil
}

// Is10Bit reports whether the monitor's current pixel format is 10-bit, used to
// restore the right bitdepth after a temporary color-mode change.
func (m Monitor) Is10Bit() bool {
	switch strings.ToUpper(m.CurrentFormat) {
	case "XBGR2101010", "XRGB2101010", "ABGR2101010", "ARGB2101010":
		return true
	}
	return false
}

// SetMonitorColorMode reconfigures a monitor's color-management preset (e.g.
// "srgb" or "hdr") and bit depth via `hyprctl keyword monitor`, preserving its
// resolution, refresh rate, position and scale. This is used to temporarily drop
// an HDR output to SDR for a correct screen capture, since Hyprland (as of
// 0.55.x) exposes no 10-bit capture path and converts HDR outputs to near-black.
func (c *Client) SetMonitorColorMode(ctx context.Context, m Monitor, cmPreset string, bitdepth int) error {
	spec := fmt.Sprintf("%s,%dx%d@%.3f,%dx%d,%g,cm,%s,bitdepth,%d",
		m.Name, m.Width, m.Height, m.RefreshRate, m.X, m.Y, m.Scale, cmPreset, bitdepth)
	if _, err := c.run(ctx, "keyword", "monitor", spec); err != nil {
		return fmt.Errorf("set monitor color mode (%s): %w", spec, err)
	}
	return nil
}

func isWarframe(w ActiveWindow) bool {
	c := strings.ToLower(w.Class)
	t := strings.ToLower(w.Title)
	return strings.Contains(c, "warframe") || strings.Contains(t, "warframe")
}
