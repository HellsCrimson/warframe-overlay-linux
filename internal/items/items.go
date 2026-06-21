// Package items computes the on-screen rectangles of the 1-4 reward names on the
// relic reward-selection screen, ported from wfinfo-ng's geometry. The constants
// are defined against a 1920x1080 reference and scaled to the captured frame.
package items

import "image"

// Reference geometry constants (pixels at 1920x1080), from wfinfo-ng src/ocr.rs.
const (
	pixelRewardWidth = 968.0

	// Vertical band of the item-name text, as offsets in pixels ABOVE the
	// screen vertical center, at the 1080p reference. This isolates the item
	// name line(s) and excludes the icon above and the divider + player-name row
	// below, which otherwise add OCR noise. Calibrated against real 1440p relic
	// captures; the band is tall enough to include two-line names (e.g.
	// "Caliban Prime Chassis Blueprint").
	nameBandTopOffset = 126.0
	nameBandBotOffset = 81.0
)

// MaxRewards is the most reward choices Warframe shows at once.
const MaxRewards = 4

// Scaling returns the UI scale factor for a frame of size w x h relative to the
// 1920x1080 reference, matching wfinfo-ng: letterboxing is accounted for by
// using whichever axis is the limiting one for a 16:9 UI.
func Scaling(w, h int) float64 {
	if w*9 > h*16 {
		return float64(h) / 1080.0
	}
	return float64(w) / 1920.0
}

// RewardStrip returns the bounding rectangle of the whole reward-name strip
// (covering all reward columns) within a frame of size w x h. This is the region
// wfinfo-ng pre-crops before locating individual names.
func RewardStrip(w, h int) image.Rectangle {
	s := Scaling(w, h)
	width := float64(w)
	height := float64(h)
	mostWidth := pixelRewardWidth * s
	mostLeft := width/2.0 - mostWidth/2.0
	mostTop := height/2.0 - nameBandTopOffset*s
	mostBot := height/2.0 - nameBandBotOffset*s

	return image.Rect(
		int(mostLeft),
		int(mostTop),
		int(mostLeft+mostWidth),
		int(mostBot),
	)
}

// RewardColumns splits the reward strip into n equal-width column rectangles,
// one per reward choice, centered like Warframe lays them out. n must be in
// [1, MaxRewards].
func RewardColumns(w, h, n int) []image.Rectangle {
	if n < 1 {
		n = 1
	}
	if n > MaxRewards {
		n = MaxRewards
	}
	strip := RewardStrip(w, h)
	sw := strip.Dx()
	sh := strip.Dy()

	// Warframe centers n columns; total span scales with n. Replicate
	// wfinfo-ng's behavior of distributing names across the full strip width
	// for the maximum count and narrowing as n shrinks, keeping them centered.
	colW := sw / MaxRewards
	totalW := colW * n
	startX := strip.Min.X + (sw-totalW)/2

	cols := make([]image.Rectangle, 0, n)
	for i := 0; i < n; i++ {
		x0 := startX + i*colW
		cols = append(cols, image.Rect(x0, strip.Min.Y, x0+colW, strip.Min.Y+sh))
	}
	return cols
}
