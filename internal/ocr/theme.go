package ocr

import "image"

// Warframe renders relic reward names in the UI theme's accent colour. By
// detecting the active theme and keeping only pixels close to its primary or
// secondary colour, we isolate the text from a busy background — notably the
// full-body character render behind a Warframe-blueprint reward, which a plain
// luminance threshold (Otsu) cannot separate.
//
// Theme colours are ported from wfinfo-ng (src/theme.rs).
type theme struct {
	name       string
	pr, pg, pb int // primary accent colour
	sr, sg, sb int // secondary accent colour
}

var themes = []theme{
	{"Vitruvian", 190, 169, 102, 245, 227, 173},
	{"Stalker", 153, 31, 35, 255, 61, 51},
	{"Baruuk", 238, 193, 105, 236, 211, 162},
	{"Corpus", 35, 201, 245, 111, 229, 253},
	{"Fortuna", 57, 105, 192, 255, 115, 230},
	{"Grineer", 255, 189, 102, 255, 224, 153},
	{"Lotus", 36, 184, 242, 255, 241, 191},
	{"Nidus", 140, 38, 92, 245, 73, 93},
	{"Orokin", 20, 41, 29, 178, 125, 5},
	{"Tenno", 9, 78, 106, 6, 106, 74},
	{"HighContrast", 2, 127, 217, 255, 255, 0},
	{"Legacy", 255, 255, 255, 232, 213, 93},
	{"Equinox", 158, 159, 167, 232, 227, 227},
	{"DarkLotus", 140, 119, 147, 189, 169, 237},
	{"Zephyr", 253, 132, 2, 255, 53, 0},
}

const (
	// onThemeCutoff: a pixel within this L1 RGB distance of a theme's primary
	// colour counts as a vote for that theme during detection. Tight so that
	// only true accent-colour (text) pixels vote.
	onThemeCutoff = 60
	// textCutoff: a pixel within this L1 RGB distance of the detected theme's
	// primary or secondary colour is treated as text.
	textCutoff = 110
)

func l1(r, g, b, tr, tg, tb int) int {
	return abs(r-tr) + abs(g-tg) + abs(b-tb)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// detectTheme votes for the active UI theme over the given region: each pixel
// close to a theme's primary accent colour adds a vote, and the theme with the
// most votes wins. Subsamples for speed.
func detectTheme(img *image.RGBA, rect image.Rectangle) theme {
	rect = rect.Intersect(img.Bounds())
	votes := make([]int, len(themes))
	for y := rect.Min.Y; y < rect.Max.Y; y += 2 {
		row := img.Pix[y*img.Stride:]
		for x := rect.Min.X; x < rect.Max.X; x += 2 {
			p := row[x*4 : x*4+3]
			r, g, b := int(p[0]), int(p[1]), int(p[2])
			best, bestD := -1, 1<<30
			for i := range themes {
				d := l1(r, g, b, themes[i].pr, themes[i].pg, themes[i].pb)
				if d < bestD {
					bestD, best = d, i
				}
			}
			if best >= 0 && bestD < onThemeCutoff {
				votes[best]++
			}
		}
	}
	bestTheme, bestVotes := 0, -1
	for i, v := range votes {
		if v > bestVotes {
			bestVotes, bestTheme = v, i
		}
	}
	return themes[bestTheme]
}

// isText reports whether a pixel colour belongs to the theme's text accent.
func (t theme) isText(r, g, b int) bool {
	return l1(r, g, b, t.pr, t.pg, t.pb) < textCutoff ||
		l1(r, g, b, t.sr, t.sg, t.sb) < textCutoff
}
