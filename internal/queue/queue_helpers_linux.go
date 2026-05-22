//go:build linux

package queue

import (
	"syscall"
	"unsafe"
)

// blkGetSize64 retrieves device size in bytes via Linux ioctl BLKGETSIZE64.
func blkGetSize64(f *osFd) (uint64, error) {
	var size uint64
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), 0x80081272, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return 0, errno
	}
	return size, nil
}
