// Command wfo-capture grabs a single frame of a monitor and writes it to a PNG,
// for validating the capture/color pipeline (HDR vs SDR) independently of OCR.
package main

import (
	"context"
	"flag"
	"fmt"
	"image/png"
	"log/slog"
	"os"
	"time"

	"warframe-overlay-linux/internal/capture"
	"warframe-overlay-linux/internal/hypr"
)

func main() {
	monitor := flag.String("monitor", "", "output name to capture (e.g. DP-4); empty = auto")
	out := flag.String("o", "capture.png", "output PNG path")
	backend := flag.String("backend", "auto", "capture backend: auto|ext-image-copy|screencopy|grim")
	noToggle := flag.Bool("no-hdr-toggle", false, "disable the HDR->SDR capture workaround (debug)")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	hyprc := hypr.New()
	mon, err := hyprc.TargetMonitor(ctx, *monitor)
	if err != nil {
		fmt.Fprintln(os.Stderr, "select monitor:", err)
		os.Exit(1)
	}
	log.Info("target", "monitor", mon.Name, "hdr", mon.IsHDR(), "format", mon.CurrentFormat)

	toggleClient := hyprc
	if *noToggle {
		toggleClient = nil
	}
	cap, err := capture.ByName(*backend, toggleClient, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	log.Info("backend", "name", cap.Name())
	frame, err := cap.Capture(ctx, mon)
	if err != nil {
		fmt.Fprintln(os.Stderr, "capture:", err)
		os.Exit(1)
	}

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create:", err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, frame.Image); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
	log.Info("wrote", "path", *out, "backend", frame.Backend,
		"size", fmt.Sprintf("%dx%d", frame.Image.Bounds().Dx(), frame.Image.Bounds().Dy()))
}
