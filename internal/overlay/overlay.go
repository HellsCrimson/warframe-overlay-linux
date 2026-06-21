// Package overlay draws a click-through price overlay on top of the game using
// wlr-layer-shell, with text rendered via pangocairo (see cairo.go).
//
// Each Show call opens its own short-lived Wayland connection, paints the labels
// on the overlay layer of the target output, keeps them up for a duration, then
// tears down. A separate connection per overlay keeps its event loop independent
// of the capture path, as designed.
package overlay

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"warframe-overlay-linux/internal/hypr"
	"warframe-overlay-linux/internal/wlproto/layershell"

	"github.com/rajveermalviya/go-wayland/wayland/client"
	"golang.org/x/sys/unix"
)

// Show paints labels over monitor m and keeps the overlay up for dur (or until
// ctx is cancelled), then removes it.
func Show(ctx context.Context, m hypr.Monitor, labels []Label, dur time.Duration, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	o, err := connect(m)
	if err != nil {
		return err
	}
	defer o.close()

	if err := o.build(labels); err != nil {
		return err
	}
	log.Debug("overlay shown", "monitor", m.Name, "labels", len(labels), "dur", dur)

	// Keep the connection serviced in the background; tear down on timeout/ctx.
	dispErr := make(chan struct{})
	go func() {
		for {
			if err := o.display.Context().Dispatch(); err != nil {
				close(dispErr)
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
	case <-time.After(dur):
	case <-dispErr:
	}
	return nil
}

type overlay struct {
	display    *client.Display
	registry   *client.Registry
	shm        *client.Shm
	compositor *client.Compositor
	layerShell *layershell.ZwlrLayerShellV1
	output     *client.Output

	surface      *client.Surface
	layerSurface *layershell.ZwlrLayerSurfaceV1
	buf          *shmBuf

	shmName, shmVer     uint32
	compName, compVer   uint32
	layerName, layerVer uint32
	monitorName         string
	monX, monY          int
	outputs             []*boundOutput
}

type boundOutput struct {
	output  *client.Output
	name    string
	x, y    int32
	hasName bool
}

func connect(m hypr.Monitor) (*overlay, error) {
	display, err := client.Connect("")
	if err != nil {
		return nil, fmt.Errorf("overlay: wayland connect: %w", err)
	}
	o := &overlay{display: display, monitorName: m.Name, monX: m.X, monY: m.Y}
	display.SetErrorHandler(func(e client.DisplayErrorEvent) {
		fmt.Printf("overlay protocol error: code=%d msg=%s\n", e.Code, e.Message)
	})

	reg, err := display.GetRegistry()
	if err != nil {
		o.close()
		return nil, err
	}
	o.registry = reg
	reg.SetGlobalHandler(func(e client.RegistryGlobalEvent) {
		switch e.Interface {
		case "wl_shm":
			o.shmName, o.shmVer = e.Name, e.Version
		case "wl_compositor":
			o.compName, o.compVer = e.Name, e.Version
		case "zwlr_layer_shell_v1":
			o.layerName, o.layerVer = e.Name, e.Version
		case "wl_output":
			o.bindOutput(e.Name, e.Version)
		}
	})
	if err := o.roundtrip(); err != nil {
		o.close()
		return nil, err
	}

	if o.shmName == 0 || o.compName == 0 || o.layerName == 0 {
		o.close()
		return nil, fmt.Errorf("overlay: compositor lacks wl_shm/wl_compositor/zwlr_layer_shell_v1")
	}
	o.shm = client.NewShm(display.Context())
	if err := bindGlobal(reg, o.shmName, "wl_shm", min32(o.shmVer, 1), o.shm); err != nil {
		o.close()
		return nil, err
	}
	o.compositor = client.NewCompositor(display.Context())
	if err := bindGlobal(reg, o.compName, "wl_compositor", min32(o.compVer, 4), o.compositor); err != nil {
		o.close()
		return nil, err
	}
	o.layerShell = layershell.NewZwlrLayerShellV1(display.Context())
	if err := bindGlobal(reg, o.layerName, "zwlr_layer_shell_v1", min32(o.layerVer, 4), o.layerShell); err != nil {
		o.close()
		return nil, err
	}
	// Second roundtrip for wl_output name/geometry events.
	if err := o.roundtrip(); err != nil {
		o.close()
		return nil, err
	}
	out, err := o.outputFor(m.Name, m.X, m.Y)
	if err != nil {
		o.close()
		return nil, err
	}
	o.output = out
	return o, nil
}

func (o *overlay) bindOutput(name, version uint32) {
	out := client.NewOutput(o.display.Context())
	if err := bindGlobal(o.registry, name, "wl_output", min32(version, 4), out); err != nil {
		return
	}
	bo := &boundOutput{output: out}
	out.SetNameHandler(func(e client.OutputNameEvent) { bo.name = e.Name; bo.hasName = true })
	out.SetGeometryHandler(func(e client.OutputGeometryEvent) { bo.x, bo.y = e.X, e.Y })
	o.outputs = append(o.outputs, bo)
}

func (o *overlay) outputFor(name string, x, y int) (*client.Output, error) {
	for _, b := range o.outputs {
		if b.hasName && b.name == name {
			return b.output, nil
		}
	}
	for _, b := range o.outputs {
		if int(b.x) == x && int(b.y) == y {
			return b.output, nil
		}
	}
	if len(o.outputs) == 1 {
		return o.outputs[0].output, nil
	}
	return nil, fmt.Errorf("overlay: no wl_output matched %q", name)
}

// build creates the layer surface, makes it click-through, waits for the
// configure, renders the labels and commits.
func (o *overlay) build(labels []Label) error {
	surface, err := o.compositor.CreateSurface()
	if err != nil {
		return err
	}
	o.surface = surface

	ls, err := o.layerShell.GetLayerSurface(surface, o.output,
		uint32(layershell.ZwlrLayerShellV1LayerOverlay), "wfo-overlay")
	if err != nil {
		return err
	}
	o.layerSurface = ls

	// Anchor to all edges so the surface fills the output; size 0,0 means "use
	// the anchored extent". Exclusive zone -1 => render over everything, reserve
	// nothing.
	anchorAll := uint32(layershell.ZwlrLayerSurfaceV1AnchorTop |
		layershell.ZwlrLayerSurfaceV1AnchorBottom |
		layershell.ZwlrLayerSurfaceV1AnchorLeft |
		layershell.ZwlrLayerSurfaceV1AnchorRight)
	_ = ls.SetAnchor(anchorAll)
	_ = ls.SetExclusiveZone(-1)
	_ = ls.SetSize(0, 0)
	_ = ls.SetKeyboardInteractivity(uint32(layershell.ZwlrLayerSurfaceV1KeyboardInteractivityNone))

	// Empty input region => click-through.
	if region, err := o.compositor.CreateRegion(); err == nil {
		_ = surface.SetInputRegion(region)
		_ = region.Destroy()
	}

	var (
		configured    bool
		width, height uint32
	)
	ls.SetConfigureHandler(func(e layershell.ZwlrLayerSurfaceV1ConfigureEvent) {
		_ = ls.AckConfigure(e.Serial)
		width, height = e.Width, e.Height
		configured = true
	})
	ls.SetClosedHandler(func(layershell.ZwlrLayerSurfaceV1ClosedEvent) {})

	if err := surface.Commit(); err != nil {
		return err
	}
	// Wait for the configure with our size.
	for !configured {
		if err := o.display.Context().Dispatch(); err != nil {
			return err
		}
	}
	if width == 0 || height == 0 {
		return fmt.Errorf("overlay: compositor gave zero size")
	}

	if err := o.render(int(width), int(height), labels); err != nil {
		return err
	}
	return nil
}

func (o *overlay) render(width, height int, labels []Label) error {
	stride := width * 4
	size := stride * height
	buf, err := newShmBuf(size)
	if err != nil {
		return err
	}
	o.buf = buf

	draw(buf.data, width, height, stride, labels)

	pool, err := o.shm.CreatePool(buf.fd, int32(size))
	if err != nil {
		return err
	}
	defer pool.Destroy()
	// wl_shm ARGB8888 (value 0) is premultiplied, matching cairo ARGB32.
	wlBuf, err := pool.CreateBuffer(0, int32(width), int32(height), int32(stride), 0)
	if err != nil {
		return err
	}
	if err := o.surface.Attach(wlBuf, 0, 0); err != nil {
		return err
	}
	if err := o.surface.DamageBuffer(0, 0, int32(width), int32(height)); err != nil {
		return err
	}
	return o.surface.Commit()
}

func (o *overlay) roundtrip() error {
	cb, err := o.display.Sync()
	if err != nil {
		return err
	}
	done := false
	cb.SetDoneHandler(func(client.CallbackDoneEvent) { done = true })
	for !done {
		if err := o.display.Context().Dispatch(); err != nil {
			return err
		}
	}
	return nil
}

func (o *overlay) close() {
	if o.layerSurface != nil {
		_ = o.layerSurface.Destroy()
	}
	if o.surface != nil {
		_ = o.surface.Destroy()
	}
	if o.buf != nil {
		o.buf.close()
	}
	if o.display != nil {
		_ = o.display.Context().Close()
	}
}

// --- minimal shm + bind helpers (kept local to avoid coupling to capture) ---

type shmBuf struct {
	fd   int
	data []byte
}

func newShmBuf(size int) (*shmBuf, error) {
	fd, err := unix.MemfdCreate("wfo-overlay", unix.MFD_CLOEXEC)
	if err != nil {
		return nil, err
	}
	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		unix.Close(fd)
		return nil, err
	}
	data, err := unix.Mmap(fd, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	return &shmBuf{fd: fd, data: data}, nil
}

func (b *shmBuf) close() {
	if b.data != nil {
		_ = unix.Munmap(b.data)
		b.data = nil
	}
	if b.fd >= 0 {
		_ = unix.Close(b.fd)
		b.fd = -1
	}
}

func bindGlobal(reg *client.Registry, name uint32, iface string, version uint32, p client.Proxy) error {
	strLen := len(iface) + 1
	padded := client.PaddedLen(strLen)
	bufLen := 8 + 4 + (4 + padded) + 4 + 4
	buf := make([]byte, bufLen)
	l := 0
	client.PutUint32(buf[l:l+4], reg.ID())
	l += 4
	client.PutUint32(buf[l:l+4], uint32(bufLen<<16))
	l += 4
	client.PutUint32(buf[l:l+4], name)
	l += 4
	client.PutUint32(buf[l:l+4], uint32(strLen))
	l += 4
	copy(buf[l:l+padded], iface)
	l += padded
	client.PutUint32(buf[l:l+4], version)
	l += 4
	client.PutUint32(buf[l:l+4], p.ID())
	return reg.Context().WriteMsg(buf, nil)
}

func min32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
