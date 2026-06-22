// Package ocr turns a captured reward-screen frame into the recognized reward
// item-name strings. It is intentionally color-agnostic: each reward column is
// converted to grayscale and binarized with Otsu's method before being handed to
// Tesseract. This keeps OCR robust to imperfect HDR->sRGB color reconstruction,
// since only luminance/contrast matters.
package ocr

import (
	"fmt"
	"image"
	"strings"

	"warframe-overlay-linux/internal/items"

	"github.com/otiai10/gosseract/v2"
)

// Engine wraps a Tesseract client. It is NOT safe for concurrent use; create one
// per pipeline run.
type Engine struct {
	client *gosseract.Client
}

// NewEngine builds an OCR engine configured for short single-line item names.
func NewEngine() (*Engine, error) {
	c := gosseract.NewClient()
	if err := c.SetLanguage("eng"); err != nil {
		c.Close()
		return nil, err
	}
	// PSM 6 = a single uniform block of text. Item names can wrap to two lines
	// (e.g. "Caliban Prime Chassis Blueprint"); normalize() later collapses the
	// newline into a space so the full name is recovered.
	if err := c.SetPageSegMode(gosseract.PSM_SINGLE_BLOCK); err != nil {
		c.Close()
		return nil, err
	}
	// Reward names use letters, spaces and the occasional '&'/digit.
	_ = c.SetWhitelist("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz &-0123456789")
	return &Engine{client: c}, nil
}

// Close releases the underlying Tesseract client.
func (e *Engine) Close() error {
	if e.client != nil {
		return e.client.Close()
	}
	return nil
}

// Recognize crops the up-to-MaxRewards reward columns from img and returns the
// recognized (trimmed) name strings, skipping columns that produce empty text.
// n is the expected number of rewards; pass 0 to scan the maximum.
func (e *Engine) Recognize(img *image.RGBA, n int) ([]string, error) {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	if n <= 0 {
		n = items.MaxRewards
	}
	cols := items.RewardColumns(w, h, n)

	// Detect the UI theme once over the whole name strip (more text => a stronger
	// signal) and isolate each name by its accent colour.
	activeTheme := detectTheme(img, items.RewardStrip(w, h))

	var names []string
	for _, rect := range cols {
		bin := binarizeColumnTheme(img, rect, activeTheme)
		text, err := e.recognizeImage(bin)
		if err != nil {
			return nil, err
		}
		text = normalize(text)
		if text != "" {
			names = append(names, text)
		}
	}
	return names, nil
}

func (e *Engine) recognizeImage(img image.Image) (string, error) {
	png, err := encodePNG(img)
	if err != nil {
		return "", err
	}
	if err := e.client.SetImageFromBytes(png); err != nil {
		return "", fmt.Errorf("tesseract set image: %w", err)
	}
	return e.client.Text()
}

// normalize trims whitespace and collapses internal runs of spaces, matching the
// downstream item-name matcher's expectations.
func normalize(s string) string {
	s = strings.TrimSpace(s)
	return strings.Join(strings.Fields(s), " ")
}
