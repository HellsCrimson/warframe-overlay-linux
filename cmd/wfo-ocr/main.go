// Command wfo-ocr runs the OCR + pricing pipeline on a saved PNG, for tuning the
// reward-box geometry and threshold offline without launching the game.
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"log/slog"
	"os"
	"time"

	"warframe-overlay-linux/internal/config"
	"warframe-overlay-linux/internal/db"
	"warframe-overlay-linux/internal/ocr"
	"warframe-overlay-linux/internal/pricing"
)

func main() {
	in := flag.String("i", "", "input PNG/JPEG of a reward screen")
	n := flag.Int("n", 0, "number of reward columns (0 = max)")
	noPrices := flag.Bool("no-prices", false, "skip loading the price database")
	flag.Parse()
	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: wfo-ocr -i <image>")
		os.Exit(2)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	img, err := loadRGBA(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}

	var database *db.Database
	if !*noPrices {
		cfg := config.Default()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		_ = ctx
		database, err = db.Load(db.Options{CacheDir: cfg.CacheDir, TTL: cfg.DataTTL, Logger: log})
		if err != nil {
			log.Warn("price db unavailable", "err", err)
		}
	}

	engine, err := ocr.NewEngine()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ocr engine:", err)
		os.Exit(1)
	}
	defer engine.Close()

	names, err := engine.Recognize(img, *n)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ocr:", err)
		os.Exit(1)
	}
	fmt.Println("OCR names:", names)

	result := pricing.Evaluate(names, database)
	for i, rw := range result.Rewards {
		marker := "  "
		if i == result.BestIndex {
			marker = "▶ "
		}
		name := rw.OCRName
		if rw.Item != nil {
			name = rw.Item.DropName
		}
		fmt.Printf("%s%-34s %6.1f plat  %4d ducats\n", marker, name, rw.Plat(), rw.Ducats())
	}
}

func loadRGBA(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var src image.Image
	if isPNG(path) {
		src, err = png.Decode(f)
	} else {
		src, _, err = image.Decode(f)
	}
	if err != nil {
		return nil, err
	}
	if rgba, ok := src.(*image.RGBA); ok {
		return rgba, nil
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst, nil
}

func isPNG(path string) bool {
	return len(path) >= 4 && (path[len(path)-4:] == ".png" || path[len(path)-4:] == ".PNG")
}
