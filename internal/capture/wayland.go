package capture

import (
	"fmt"

	"github.com/rajveermalviya/go-wayland/wayland/client"
)

// waylandConn wraps a Wayland display connection plus the globals we need for
// capture. It enumerates the registry on connect and resolves a wl_output by its
// connector name (e.g. "DP-4").
type waylandConn struct {
	display  *client.Display
	registry *client.Registry
	shm      *client.Shm

	// global names (registry ids) discovered during enumeration.
	shmName        uint32
	shmVersion     uint32
	screencopyName uint32
	screencopyVer  uint32
	extSourceName  uint32 // ext_output_image_capture_source_manager_v1
	extSourceVer   uint32
	extCopyName    uint32 // ext_image_copy_capture_manager_v1
	extCopyVer     uint32

	outputs []*boundOutput
}

type boundOutput struct {
	output  *client.Output
	name    string // connector name from wl_output.name (v4+)
	x, y    int32  // logical position from geometry
	hasName bool
}

// connectWayland opens the Wayland display and enumerates globals.
func connectWayland() (*waylandConn, error) {
	display, err := client.Connect("")
	if err != nil {
		return nil, fmt.Errorf("wayland connect: %w", err)
	}
	c := &waylandConn{display: display}
	display.SetErrorHandler(func(e client.DisplayErrorEvent) {
		// Surface protocol errors during development.
		fmt.Printf("wayland protocol error: object=%d code=%d msg=%s\n", e.ObjectId, e.Code, e.Message)
	})

	reg, err := display.GetRegistry()
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("get registry: %w", err)
	}
	c.registry = reg

	reg.SetGlobalHandler(func(e client.RegistryGlobalEvent) {
		switch e.Interface {
		case "wl_shm":
			c.shmName, c.shmVersion = e.Name, e.Version
		case "zwlr_screencopy_manager_v1":
			c.screencopyName, c.screencopyVer = e.Name, e.Version
		case "ext_output_image_capture_source_manager_v1":
			c.extSourceName, c.extSourceVer = e.Name, e.Version
		case "ext_image_copy_capture_manager_v1":
			c.extCopyName, c.extCopyVer = e.Name, e.Version
		case "wl_output":
			c.bindOutput(e.Name, e.Version)
		}
	})

	// First roundtrip: receive the global advertisements.
	if err := c.roundtrip(); err != nil {
		c.Close()
		return nil, err
	}

	// Bind wl_shm now that we know it exists.
	if c.shmName != 0 {
		c.shm = client.NewShm(display.Context())
		if err := bindGlobal(reg, c.shmName, "wl_shm", min32(c.shmVersion, 1), c.shm); err != nil {
			c.Close()
			return nil, fmt.Errorf("bind wl_shm: %w", err)
		}
	}

	// Second roundtrip: receive per-output geometry/name events.
	if err := c.roundtrip(); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *waylandConn) bindOutput(name, version uint32) {
	out := client.NewOutput(c.display.Context())
	// v4 is needed for the name event; clamp to what the server offers.
	bindVer := min32(version, 4)
	if err := bindGlobal(c.registry, name, "wl_output", bindVer, out); err != nil {
		return
	}
	bo := &boundOutput{output: out}
	out.SetNameHandler(func(e client.OutputNameEvent) {
		bo.name = e.Name
		bo.hasName = true
	})
	out.SetGeometryHandler(func(e client.OutputGeometryEvent) {
		bo.x, bo.y = e.X, e.Y
	})
	c.outputs = append(c.outputs, bo)
}

// outputByName resolves the wl_output matching connector name, falling back to
// logical position match against x,y when the name event is unavailable.
func (c *waylandConn) outputByName(name string, x, y int) (*client.Output, error) {
	for _, o := range c.outputs {
		if o.hasName && o.name == name {
			return o.output, nil
		}
	}
	for _, o := range c.outputs {
		if int(o.x) == x && int(o.y) == y {
			return o.output, nil
		}
	}
	if len(c.outputs) == 1 {
		return c.outputs[0].output, nil
	}
	return nil, fmt.Errorf("no wl_output matched name=%q pos=%d,%d", name, x, y)
}

// roundtrip blocks until the server has processed all prior requests, draining
// events in the meantime.
func (c *waylandConn) roundtrip() error {
	cb, err := c.display.Sync()
	if err != nil {
		return err
	}
	done := false
	cb.SetDoneHandler(func(client.CallbackDoneEvent) { done = true })
	for !done {
		if err := c.display.Context().Dispatch(); err != nil {
			return fmt.Errorf("dispatch: %w", err)
		}
	}
	return nil
}

// dispatchUntil pumps events until pred returns true or an error occurs.
func (c *waylandConn) dispatchUntil(pred func() bool) error {
	for !pred() {
		if err := c.display.Context().Dispatch(); err != nil {
			return fmt.Errorf("dispatch: %w", err)
		}
	}
	return nil
}

func (c *waylandConn) Close() {
	if c.display != nil {
		_ = c.display.Context().Close()
		c.display = nil
	}
}

func min32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// bindGlobal performs wl_registry.bind with the correct (unpadded) interface
// string length. go-wayland's built-in Registry.Bind sends the 4-byte-padded
// length as the string length prefix, which newer libwayland rejects with
// "invalid arguments for wl_registry.bind". This reimplements the request
// correctly. The proxy p must already be registered (the New* constructors do
// this).
func bindGlobal(reg *client.Registry, name uint32, iface string, version uint32, p client.Proxy) error {
	strLen := len(iface) + 1 // include null terminator
	padded := client.PaddedLen(strLen)
	bufLen := 8 + 4 + (4 + padded) + 4 + 4
	buf := make([]byte, bufLen)
	l := 0
	client.PutUint32(buf[l:l+4], reg.ID())
	l += 4
	client.PutUint32(buf[l:l+4], uint32(bufLen<<16)) // size<<16 | opcode(0=bind)
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
