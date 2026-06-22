package ocr

import (
	"image"
	"testing"
)

func fill(img *image.RGBA, r, g, b uint8) {
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = r, g, b, 255
	}
}

func TestDetectThemeFromAccentColor(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	// Fill with Baruuk's primary accent colour.
	fill(img, 238, 193, 105)
	got := detectTheme(img, img.Bounds())
	if got.name != "Baruuk" {
		t.Errorf("detectTheme = %q, want Baruuk", got.name)
	}
}

func TestIsTextMatchesAccentNotBackground(t *testing.T) {
	var baruuk theme
	for _, th := range themes {
		if th.name == "Baruuk" {
			baruuk = th
		}
	}
	// Near the primary accent -> text.
	if !baruuk.isText(240, 195, 108) {
		t.Error("expected near-accent pixel to be text")
	}
	// A dark blue background pixel -> not text.
	if baruuk.isText(20, 30, 90) {
		t.Error("expected background pixel not to be text")
	}
}
