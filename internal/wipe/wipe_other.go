//go:build !linux

package wipe

import (
	"errors"
	"os"
)

// ErrUnsupported is returned by platform-specific operations when the current
// build target is not Linux. The wiper is designed to run inside a Linux
// container; these stubs exist so go build / go vet succeed on Windows and
// macOS for local development.
var ErrUnsupported = errors.New("usb-wiper: device ioctl only supported on Linux")

// BlkGetSize64 is unsupported on non-Linux platforms.
func BlkGetSize64(f *os.File) (uint64, error) {
	return 0, ErrUnsupported
}
