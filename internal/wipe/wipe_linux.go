//go:build linux

package wipe

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// BlkGetSize64 retrieves device size in bytes using Linux ioctl BLKGETSIZE64.
func BlkGetSize64(f *os.File) (uint64, error) {
	var size uint64
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), 0x80081272, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return 0, fmt.Errorf("ioctl BLKGETSIZE64: %v", errno)
	}
	return size, nil
}
