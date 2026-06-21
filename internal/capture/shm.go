package capture

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// shmBuffer is an anonymous, memory-mapped buffer shared with the compositor via
// a file descriptor. The compositor copies captured pixels into Data.
type shmBuffer struct {
	fd   int
	Data []byte
	Size int
}

// newShmBuffer creates a memfd of the given size and maps it.
func newShmBuffer(size int) (*shmBuffer, error) {
	if size <= 0 {
		return nil, fmt.Errorf("shm: invalid size %d", size)
	}
	fd, err := unix.MemfdCreate("wfo-capture", unix.MFD_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("memfd_create: %w", err)
	}
	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("ftruncate: %w", err)
	}
	data, err := unix.Mmap(fd, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("mmap: %w", err)
	}
	return &shmBuffer{fd: fd, Data: data, Size: size}, nil
}

// Close unmaps and closes the buffer.
func (b *shmBuffer) Close() {
	if b.Data != nil {
		_ = unix.Munmap(b.Data)
		b.Data = nil
	}
	if b.fd >= 0 {
		_ = unix.Close(b.fd)
		b.fd = -1
	}
}
