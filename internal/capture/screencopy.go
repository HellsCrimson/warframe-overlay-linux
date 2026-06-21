package capture

import (
	"context"
	"fmt"
	"log/slog"

	"warframe-overlay-linux/internal/hypr"
	"warframe-overlay-linux/internal/wlproto/screencopy"

	"github.com/rajveermalviya/go-wayland/wayland/client"
)

// ScreencopyCapturer captures a monitor via wlr-screencopy-unstable-v1. Unlike
// grim it allocates a buffer of the output's *native* format (e.g. 10-bit
// XBGR2101010 under HDR) and tonemaps the raw PQ data to sRGB itself, avoiding
// the broken HDR->8bit conversion that leaves grim's output near-black.
type ScreencopyCapturer struct {
	Logger *slog.Logger
}

func (s *ScreencopyCapturer) Name() string { return "screencopy" }

func (s *ScreencopyCapturer) Capture(ctx context.Context, m hypr.Monitor) (*Frame, error) {
	log := s.Logger
	if log == nil {
		log = slog.Default()
	}

	conn, err := connectWayland()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if conn.screencopyName == 0 {
		return nil, fmt.Errorf("compositor does not advertise zwlr_screencopy_manager_v1")
	}
	if conn.shm == nil {
		return nil, fmt.Errorf("compositor does not advertise wl_shm")
	}

	mgr := screencopy.NewZwlrScreencopyManagerV1(conn.display.Context())
	if err := bindGlobal(conn.registry, conn.screencopyName, "zwlr_screencopy_manager_v1", min32(conn.screencopyVer, 3), mgr); err != nil {
		return nil, fmt.Errorf("bind screencopy manager: %w", err)
	}

	output, err := conn.outputByName(m.Name, m.X, m.Y)
	if err != nil {
		return nil, err
	}

	frame, err := mgr.CaptureOutput(0, output)
	if err != nil {
		return nil, fmt.Errorf("capture_output: %w", err)
	}

	var (
		gotBuffer  bool
		bufferDone bool
		ready      bool
		failed     bool
		format     uint32
		width      uint32
		height     uint32
		stride     uint32
	)
	frame.SetBufferHandler(func(e screencopy.ZwlrScreencopyFrameV1BufferEvent) {
		// First shm buffer option wins; ignore subsequent ones.
		if !gotBuffer {
			format, width, height, stride = e.Format, e.Width, e.Height, e.Stride
			gotBuffer = true
		}
	})
	frame.SetBufferDoneHandler(func(screencopy.ZwlrScreencopyFrameV1BufferDoneEvent) { bufferDone = true })
	frame.SetReadyHandler(func(screencopy.ZwlrScreencopyFrameV1ReadyEvent) { ready = true })
	frame.SetFailedHandler(func(screencopy.ZwlrScreencopyFrameV1FailedEvent) { failed = true })

	// Wait for the buffer parameters (and buffer_done on v3+).
	if err := conn.dispatchUntil(func() bool { return (gotBuffer && bufferDone) || failed }); err != nil {
		return nil, err
	}
	if failed {
		return nil, fmt.Errorf("screencopy frame failed before copy")
	}

	log.Debug("screencopy buffer", "format", fmt.Sprintf("0x%08x (%s)", format, fourccName(format)),
		"size", fmt.Sprintf("%dx%d", width, height), "stride", stride)

	size := int(stride) * int(height)
	buf, err := newShmBuffer(size)
	if err != nil {
		return nil, err
	}
	defer buf.Close()

	pool, err := conn.shm.CreatePool(buf.fd, int32(size))
	if err != nil {
		return nil, fmt.Errorf("create shm pool: %w", err)
	}
	defer pool.Destroy()

	wlBuf, err := pool.CreateBuffer(0, int32(width), int32(height), int32(stride), format)
	if err != nil {
		return nil, fmt.Errorf("create buffer: %w", err)
	}
	defer wlBuf.Destroy()

	if err := frame.Copy(wlBuf); err != nil {
		return nil, fmt.Errorf("frame copy: %w", err)
	}

	if err := conn.dispatchUntil(func() bool { return ready || failed }); err != nil {
		return nil, err
	}
	if failed {
		return nil, fmt.Errorf("screencopy frame failed during copy")
	}

	img, err := decodeFrame(buf.Data, int(width), int(height), int(stride), format, m)
	if err != nil {
		return nil, err
	}
	return &Frame{Image: img, Monitor: m, Backend: s.Name(), WasHDR: m.IsHDR()}, nil
}

// drmFormat constants we may encounter (wl_shm format == DRM fourcc, except the
// two ARGB/XRGB 8888 specials which wl_shm renumbers to 0/1).
const (
	wlShmARGB8888  = 0          // [31:0] A:R:G:B 8:8:8:8
	wlShmXRGB8888  = 1          // [31:0] x:R:G:B 8:8:8:8
	drmXRGB2101010 = 0x30335258 // 'XR30' [31:0] x:R:G:B 2:10:10:10
	drmXBGR2101010 = 0x30334258 // 'XB30' [31:0] x:B:G:R 2:10:10:10
	drmARGB2101010 = 0x30335241 // 'AR30'
	drmABGR2101010 = 0x30334241 // 'AB30'
	drmARGB8888    = 0x34325241 // 'AR24'
	drmXRGB8888    = 0x34325258 // 'XR24'
	drmABGR8888    = 0x34324241 // 'AB24'
	drmXBGR8888    = 0x34324258 // 'XB24'
)

// fourccName renders a DRM fourcc as its 4-char tag for logging.
func fourccName(f uint32) string {
	switch f {
	case wlShmARGB8888:
		return "ARGB8888"
	case wlShmXRGB8888:
		return "XRGB8888"
	}
	b := []byte{byte(f), byte(f >> 8), byte(f >> 16), byte(f >> 24)}
	for i, c := range b {
		if c < 32 || c > 126 {
			b[i] = '?'
		}
	}
	return string(b)
}

var _ = client.NewOutput // keep client import referenced for interface clarity
