//go:build !linux

package queue

import "errors"

// errUnsupported is returned by platform-specific operations when the current
// build target is not Linux. The queue's destructive paths are designed for a
// Linux container; this stub exists so go build / go vet succeed on Windows
// and macOS for local development.
var errUnsupported = errors.New("queue: device ioctl only supported on Linux")

// blkGetSize64 is unsupported on non-Linux platforms.
func blkGetSize64(f *osFd) (uint64, error) {
	return 0, errUnsupported
}
