package capture

import (
	"context"
	"fmt"
	"log/slog"

	"warframe-overlay-linux/internal/hypr"
	"warframe-overlay-linux/internal/wlproto/extcapture"
)

// ExtImageCopyCapturer captures a monitor via ext-image-copy-capture-v1. This is
// the modern, color-management-aware capture path. Crucially, on Hyprland under
// HDR it advertises the output's *native* buffer format (10-bit, e.g.
// XBGR2101010 carrying PQ-encoded rec2020) rather than the broken 8-bit ARGB
// that wlr-screencopy and grim produce, letting color.go tonemap correctly.
type ExtImageCopyCapturer struct {
	Logger *slog.Logger
}

func (s *ExtImageCopyCapturer) Name() string { return "ext-image-copy" }

func (s *ExtImageCopyCapturer) Capture(ctx context.Context, m hypr.Monitor) (*Frame, error) {
	log := s.Logger
	if log == nil {
		log = slog.Default()
	}

	conn, err := connectWayland()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if conn.extSourceName == 0 || conn.extCopyName == 0 {
		return nil, fmt.Errorf("compositor lacks ext-image-copy-capture-v1")
	}
	if conn.shm == nil {
		return nil, fmt.Errorf("compositor lacks wl_shm")
	}

	srcMgr := extcapture.NewExtOutputImageCaptureSourceManagerV1(conn.display.Context())
	if err := bindGlobal(conn.registry, conn.extSourceName, "ext_output_image_capture_source_manager_v1", min32(conn.extSourceVer, 1), srcMgr); err != nil {
		return nil, fmt.Errorf("bind source manager: %w", err)
	}
	copyMgr := extcapture.NewExtImageCopyCaptureManagerV1(conn.display.Context())
	if err := bindGlobal(conn.registry, conn.extCopyName, "ext_image_copy_capture_manager_v1", min32(conn.extCopyVer, 1), copyMgr); err != nil {
		return nil, fmt.Errorf("bind copy manager: %w", err)
	}

	output, err := conn.outputByName(m.Name, m.X, m.Y)
	if err != nil {
		return nil, err
	}

	source, err := srcMgr.CreateSource(output)
	if err != nil {
		return nil, fmt.Errorf("create source: %w", err)
	}
	defer source.Destroy()

	// options=0: no cursor painting.
	session, err := copyMgr.CreateSession(source, 0)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	defer session.Destroy()

	var (
		width, height uint32
		shmFormats    []uint32
		sessionDone   bool
		stopped       bool
	)
	session.SetBufferSizeHandler(func(e extcapture.ExtImageCopyCaptureSessionV1BufferSizeEvent) {
		width, height = e.Width, e.Height
	})
	session.SetShmFormatHandler(func(e extcapture.ExtImageCopyCaptureSessionV1ShmFormatEvent) {
		shmFormats = append(shmFormats, e.Format)
	})
	session.SetDoneHandler(func(extcapture.ExtImageCopyCaptureSessionV1DoneEvent) { sessionDone = true })
	session.SetStoppedHandler(func(extcapture.ExtImageCopyCaptureSessionV1StoppedEvent) { stopped = true })

	if err := conn.dispatchUntil(func() bool { return sessionDone || stopped }); err != nil {
		return nil, err
	}
	if stopped {
		return nil, fmt.Errorf("capture session stopped before first frame")
	}
	if width == 0 || height == 0 || len(shmFormats) == 0 {
		return nil, fmt.Errorf("session advertised no usable buffer (size=%dx%d formats=%d)", width, height, len(shmFormats))
	}

	format := chooseFormat(shmFormats)
	log.Debug("ext-image-copy session", "size", fmt.Sprintf("%dx%d", width, height),
		"chosen", fmt.Sprintf("0x%08x (%s)", format, fourccName(format)),
		"offered", formatNames(shmFormats))

	stride := int(width) * 4 // all supported formats are 4 bytes/pixel
	size := stride * int(height)
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

	frame, err := session.CreateFrame()
	if err != nil {
		return nil, fmt.Errorf("create frame: %w", err)
	}
	defer frame.Destroy()

	var (
		ready      bool
		failed     bool
		failReason string
	)
	frame.SetReadyHandler(func(extcapture.ExtImageCopyCaptureFrameV1ReadyEvent) { ready = true })
	frame.SetFailedHandler(func(e extcapture.ExtImageCopyCaptureFrameV1FailedEvent) {
		failed = true
		failReason = extcapture.ExtImageCopyCaptureFrameV1FailureReason(e.Reason).String()
	})

	if err := frame.AttachBuffer(wlBuf); err != nil {
		return nil, fmt.Errorf("attach buffer: %w", err)
	}
	if err := frame.DamageBuffer(0, 0, int32(width), int32(height)); err != nil {
		return nil, fmt.Errorf("damage buffer: %w", err)
	}
	if err := frame.Capture(); err != nil {
		return nil, fmt.Errorf("capture: %w", err)
	}

	if err := conn.dispatchUntil(func() bool { return ready || failed }); err != nil {
		return nil, err
	}
	if failed {
		return nil, fmt.Errorf("frame capture failed: %s", failReason)
	}

	img, err := decodeFrame(buf.Data, int(width), int(height), stride, format, m)
	if err != nil {
		return nil, err
	}
	return &Frame{Image: img, Monitor: m, Backend: s.Name(), WasHDR: m.IsHDR()}, nil
}

// chooseFormat prefers a 10-bit native format (so HDR PQ data reaches color.go
// intact) and otherwise falls back to the first 8-bit format offered.
func chooseFormat(formats []uint32) uint32 {
	pref := []uint32{drmXBGR2101010, drmXRGB2101010, drmABGR2101010, drmARGB2101010}
	for _, want := range pref {
		for _, f := range formats {
			if f == want {
				return f
			}
		}
	}
	return formats[0]
}

func formatNames(formats []uint32) []string {
	out := make([]string, len(formats))
	for i, f := range formats {
		out[i] = fourccName(f)
	}
	return out
}
