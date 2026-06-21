package capture

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	"os/exec"
	"strconv"

	"warframe-overlay-linux/internal/hypr"
)

// GrimCapturer shells out to `grim` to capture a monitor as a PPM stream on
// stdout. This is the simple, dependency-light fallback. It is SDR-correct but
// produces washed-out output when the monitor is in HDR, so callers should
// prefer a Wayland color-managed backend for HDR monitors.
type GrimCapturer struct {
	Bin string // defaults to "grim"
}

func (g *GrimCapturer) Name() string { return "grim" }

func (g *GrimCapturer) Capture(ctx context.Context, m hypr.Monitor) (*Frame, error) {
	bin := g.Bin
	if bin == "" {
		bin = "grim"
	}
	// -t ppm: uncompressed P6 (binary RGB), trivial to parse and lossless.
	// -o <name>: restrict capture to the target output.
	args := []string{"-t", "ppm"}
	if m.Name != "" {
		args = append(args, "-o", m.Name)
	}
	args = append(args, "-") // write to stdout
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("grim: %w (%s)", err, stderr.String())
	}
	img, err := decodePPM(stdout.Bytes())
	if err != nil {
		return nil, fmt.Errorf("grim: decode ppm: %w", err)
	}
	return &Frame{Image: img, Monitor: m, Backend: g.Name(), WasHDR: false}, nil
}

// decodePPM parses a binary P6 PPM into an *image.RGBA.
func decodePPM(data []byte) (*image.RGBA, error) {
	r := bufio.NewReader(bytes.NewReader(data))

	magic, err := readToken(r)
	if err != nil {
		return nil, err
	}
	if magic != "P6" {
		return nil, fmt.Errorf("unsupported PPM magic %q (want P6)", magic)
	}
	width, err := readIntToken(r)
	if err != nil {
		return nil, err
	}
	height, err := readIntToken(r)
	if err != nil {
		return nil, err
	}
	maxval, err := readIntToken(r)
	if err != nil {
		return nil, err
	}
	if maxval != 255 {
		return nil, fmt.Errorf("unsupported PPM maxval %d (want 255)", maxval)
	}
	// Exactly one whitespace byte separates the header from pixel data.
	if _, err := r.ReadByte(); err != nil {
		return nil, err
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	row := make([]byte, width*3)
	for y := 0; y < height; y++ {
		if _, err := readFull(r, row); err != nil {
			return nil, fmt.Errorf("pixel data at row %d: %w", y, err)
		}
		dst := img.Pix[y*img.Stride:]
		for x := 0; x < width; x++ {
			dst[x*4+0] = row[x*3+0]
			dst[x*4+1] = row[x*3+1]
			dst[x*4+2] = row[x*3+2]
			dst[x*4+3] = 255
		}
	}
	return img, nil
}

// readToken reads a whitespace-delimited token, skipping leading whitespace and
// '#' comment lines.
func readToken(r *bufio.Reader) (string, error) {
	var buf []byte
	// skip whitespace and comments
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == '#' {
			// consume to end of line
			for b != '\n' {
				if b, err = r.ReadByte(); err != nil {
					return "", err
				}
			}
			continue
		}
		if !isSpace(b) {
			buf = append(buf, b)
			break
		}
	}
	for {
		b, err := r.ReadByte()
		if err != nil {
			break
		}
		if isSpace(b) {
			_ = r.UnreadByte()
			break
		}
		buf = append(buf, b)
	}
	return string(buf), nil
}

func readIntToken(r *bufio.Reader) (int, error) {
	tok, err := readToken(r)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(tok)
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}
