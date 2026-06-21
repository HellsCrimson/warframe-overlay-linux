package overlay

import (
	"image"
	"image/png"
	"os"
	"testing"
)

// TestRenderPNG renders sample labels and writes a PNG (composited over dark
// gray so transparency is visible) for manual inspection.
func TestRenderPNG(t *testing.T) {
	w, h := 1000, 200
	stride := w * 4
	buf := make([]byte, stride*h)
	labels := []Label{
		{Name: "Bronco Prime Receiver", Price: "12p · 15d", CenterX: 180, Top: 60, Best: false},
		{Name: "Braton Prime Stock", Price: "8p · 25d", CenterX: 400, Top: 60, Best: false},
		{Name: "Cobra & Crane Prime Hilt", Price: "45p · 45d", CenterX: 620, Top: 60, Best: true},
		{Name: "Bronco Prime Blueprint", Price: "5p · 15d", CenterX: 840, Top: 60, Best: false},
	}
	draw(buf, w, h, stride, labels)

	// wl_shm ARGB8888 little-endian => bytes B,G,R,A (premultiplied).
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	const bg = 30
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*stride + x*4
			b, g, r, a := int(buf[i]), int(buf[i+1]), int(buf[i+2]), int(buf[i+3])
			o := y*out.Stride + x*4
			out.Pix[o+0] = byte(r + bg*(255-a)/255)
			out.Pix[o+1] = byte(g + bg*(255-a)/255)
			out.Pix[o+2] = byte(b + bg*(255-a)/255)
			out.Pix[o+3] = 255
		}
	}
	f, err := os.Create("/tmp/overlay-test.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		t.Fatal(err)
	}
	t.Log("wrote /tmp/overlay-test.png")
}
