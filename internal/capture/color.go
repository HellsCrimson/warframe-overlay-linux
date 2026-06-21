package capture

import (
	"fmt"
	"image"
	"math"

	"warframe-overlay-linux/internal/hypr"
)

// decodeFrame converts a raw captured buffer into a normalized sRGB *image.RGBA.
//
// For SDR 8-bit buffers this is a channel-order unpack. For HDR buffers (10-bit,
// PQ-encoded rec2020) it applies the ST.2084 EOTF, scales by the monitor's SDR
// reference white, converts rec2020->rec709 gamut, and re-encodes to sRGB. Only
// the transfer-function correctness matters for OCR; gamut is handled too so the
// dumped PNGs look right to a human.
func decodeFrame(data []byte, width, height, stride int, format uint32, m hypr.Monitor) (*image.RGBA, error) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	switch format {
	case wlShmXRGB8888, wlShmARGB8888, drmXRGB8888, drmARGB8888:
		unpack8(data, img, width, height, stride, orderBGRX)
		return img, nil
	case drmXBGR8888, drmABGR8888:
		unpack8(data, img, width, height, stride, orderRGBX)
		return img, nil
	case drmXBGR2101010, drmABGR2101010:
		unpack10(data, img, width, height, stride, order10BGR, m)
		return img, nil
	case drmXRGB2101010, drmARGB2101010:
		unpack10(data, img, width, height, stride, order10RGB, m)
		return img, nil
	default:
		return nil, fmt.Errorf("unsupported capture format 0x%08x (%s)", format, fourccName(format))
	}
}

type byteOrder int

const (
	orderBGRX byteOrder = iota // little-endian xRGB8888: mem bytes B,G,R,X
	orderRGBX                  // little-endian xBGR8888: mem bytes R,G,B,X
)

func unpack8(data []byte, img *image.RGBA, w, h, stride int, order byteOrder) {
	for y := 0; y < h; y++ {
		srow := data[y*stride:]
		drow := img.Pix[y*img.Stride:]
		for x := 0; x < w; x++ {
			s := srow[x*4 : x*4+4]
			var r, g, b byte
			switch order {
			case orderBGRX:
				b, g, r = s[0], s[1], s[2]
			case orderRGBX:
				r, g, b = s[0], s[1], s[2]
			}
			d := drow[x*4 : x*4+4]
			d[0], d[1], d[2], d[3] = r, g, b, 255
		}
	}
}

type chan10Order int

const (
	order10RGB chan10Order = iota // 'XR30': bits[9:0]=B,[19:10]=G,[29:20]=R
	order10BGR                    // 'XB30': bits[9:0]=R,[19:10]=G,[29:20]=B
)

func unpack10(data []byte, img *image.RGBA, w, h, stride int, order chan10Order, m hypr.Monitor) {
	hdr := m.IsHDR()
	refWhite := m.SDRMaxLuminance
	if refWhite <= 0 {
		refWhite = 203.0 // Hyprland's default SDR reference white in nits
	}

	for y := 0; y < h; y++ {
		srow := data[y*stride:]
		drow := img.Pix[y*img.Stride:]
		for x := 0; x < w; x++ {
			word := uint32(srow[x*4]) | uint32(srow[x*4+1])<<8 |
				uint32(srow[x*4+2])<<16 | uint32(srow[x*4+3])<<24
			c0 := word & 0x3FF
			c1 := (word >> 10) & 0x3FF
			c2 := (word >> 20) & 0x3FF
			var rc, gc, bc uint32
			switch order {
			case order10RGB:
				bc, gc, rc = c0, c1, c2
			case order10BGR:
				rc, gc, bc = c0, c1, c2
			}

			var r8, g8, b8 byte
			if hdr {
				r8, g8, b8 = hdrToSRGB8(rc, gc, bc, refWhite)
			} else {
				// 10-bit but SDR: values are already gamma-encoded; drop 2 LSBs.
				r8, g8, b8 = byte(rc>>2), byte(gc>>2), byte(bc>>2)
			}
			d := drow[x*4 : x*4+4]
			d[0], d[1], d[2], d[3] = r8, g8, b8, 255
		}
	}
}

// hdrToSRGB8 converts one PQ-encoded rec2020 pixel (10-bit codes) to 8-bit sRGB.
func hdrToSRGB8(rc, gc, bc uint32, refWhiteNits float64) (byte, byte, byte) {
	// 1. PQ EOTF: code -> normalized luminance (1.0 == 10000 nits).
	lr := pqEOTF(float64(rc) / 1023.0)
	lg := pqEOTF(float64(gc) / 1023.0)
	lb := pqEOTF(float64(bc) / 1023.0)

	// 2. Scale so SDR reference white (refWhiteNits) maps to 1.0.
	scale := 10000.0 / refWhiteNits
	lr *= scale
	lg *= scale
	lb *= scale

	// 3. rec2020 -> rec709 (linear) gamut conversion.
	r := 1.6605*lr - 0.5876*lg - 0.0728*lb
	g := -0.1246*lr + 1.1329*lg - 0.0083*lb
	b := -0.0182*lr - 0.1006*lg + 1.1187*lb

	// 4. Clip to SDR range (simple highlight clamp) then sRGB OETF.
	return srgb8(r), srgb8(g), srgb8(b)
}

// pqEOTF implements the SMPTE ST.2084 (PQ) electro-optical transfer function,
// returning normalized luminance in [0,1] where 1.0 corresponds to 10000 cd/m^2.
func pqEOTF(v float64) float64 {
	if v <= 0 {
		return 0
	}
	const (
		m1 = 2610.0 / 16384.0
		m2 = 2523.0 / 4096.0 * 128.0
		c1 = 3424.0 / 4096.0
		c2 = 2413.0 / 4096.0 * 32.0
		c3 = 2392.0 / 4096.0 * 32.0
	)
	vp := math.Pow(v, 1.0/m2)
	num := math.Max(vp-c1, 0)
	den := c2 - c3*vp
	if den <= 0 {
		return 0
	}
	return math.Pow(num/den, 1.0/m1)
}

// srgb8 clamps a linear value to [0,1], applies the sRGB OETF, and quantizes.
func srgb8(u float64) byte {
	if u <= 0 {
		return 0
	}
	if u >= 1 {
		return 255
	}
	var s float64
	if u <= 0.0031308 {
		s = 12.92 * u
	} else {
		s = 1.055*math.Pow(u, 1.0/2.4) - 0.055
	}
	v := int(s*255.0 + 0.5)
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return byte(v)
}
