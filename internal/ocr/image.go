package ocr

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// binarizeColumn crops rect from img, converts it to grayscale, picks a global
// threshold with Otsu's method, and returns a black-text-on-white *image.Gray
// suitable for Tesseract. Warframe renders light text, so after thresholding we
// invert when the foreground appears to be the lighter class.
func binarizeColumn(img *image.RGBA, rect image.Rectangle) *image.Gray {
	rect = rect.Intersect(img.Bounds())
	w := rect.Dx()
	h := rect.Dy()
	if w == 0 || h == 0 {
		return image.NewGray(image.Rect(0, 0, 1, 1))
	}
	gray := image.NewGray(image.Rect(0, 0, w, h))

	var hist [256]int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(rect.Min.X+x, rect.Min.Y+y).RGBA()
			// Rec.601 luma; inputs are 16-bit from RGBA().
			lum := (299*(r>>8) + 587*(g>>8) + 114*(b>>8)) / 1000
			v := uint8(lum)
			gray.SetGray(x, y, color.Gray{Y: v})
			hist[v]++
		}
	}

	threshold := otsu(hist[:], w*h)

	// Warframe text is light on a darker panel, so the text (foreground) is the
	// minority bright class. We want black text on white for Tesseract.
	var bright int
	for v := threshold + 1; v < 256; v++ {
		bright += hist[v]
	}
	textIsBright := bright*2 < w*h

	out := image.NewGray(image.Rect(0, 0, w, h))
	for i, v := range gray.Pix {
		isBright := int(v) > threshold
		fg := isBright == textIsBright
		if fg {
			out.Pix[i] = 0
		} else {
			out.Pix[i] = 255
		}
	}
	return out
}

// binarizeColumnTheme crops rect and keeps only pixels matching the detected UI
// theme's accent colour as text (black on white). This isolates relic names from
// busy backgrounds (notably the character render behind a Warframe blueprint).
// If too few pixels match — e.g. theme detection was wrong — it falls back to the
// luminance-based binarizeColumn.
func binarizeColumnTheme(img *image.RGBA, rect image.Rectangle, t theme) *image.Gray {
	rect = rect.Intersect(img.Bounds())
	w := rect.Dx()
	h := rect.Dy()
	if w == 0 || h == 0 {
		return image.NewGray(image.Rect(0, 0, 1, 1))
	}
	out := image.NewGray(image.Rect(0, 0, w, h))
	textPixels := 0
	for y := 0; y < h; y++ {
		srow := img.Pix[(rect.Min.Y+y)*img.Stride:]
		for x := 0; x < w; x++ {
			p := srow[(rect.Min.X+x)*4:]
			if t.isText(int(p[0]), int(p[1]), int(p[2])) {
				out.Pix[y*out.Stride+x] = 0
				textPixels++
			} else {
				out.Pix[y*out.Stride+x] = 255
			}
		}
	}
	// Sanity: a real name covers at least a fraction of a percent of the band.
	if textPixels*1000 < w*h { // < 0.1%
		return binarizeColumn(img, rect)
	}
	return out
}

// otsu returns the grayscale threshold maximizing between-class variance.
func otsu(hist []int, total int) int {
	if total == 0 {
		return 127
	}
	var sum float64
	for t := 0; t < 256; t++ {
		sum += float64(t) * float64(hist[t])
	}
	var sumB, wB, maxVar float64
	threshold := 127
	for t := 0; t < 256; t++ {
		wB += float64(hist[t])
		if wB == 0 {
			continue
		}
		wF := float64(total) - wB
		if wF == 0 {
			break
		}
		sumB += float64(t) * float64(hist[t])
		mB := sumB / wB
		mF := (sum - sumB) / wF
		between := wB * wF * (mB - mF) * (mB - mF)
		if between > maxVar {
			maxVar = between
			threshold = t
		}
	}
	return threshold
}

// encodePNG encodes an image as PNG bytes for handing to gosseract.
func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
