//go:build !linux

// Non-Linux build stub. The production target is a Linux Docker container;
// the real safety logic lives in safety.go (Linux build tag). These stubs
// exist solely so `go build ./...` and `go vet ./...` succeed on Windows and
// macOS for local development. They fail closed on every call.

package device

import "fmt"

// SafetyError indicates a device failed a safety check.
type SafetyError struct {
	Reason string `json:"reason"`
	Device string `json:"device"`
}

func (e *SafetyError) Error() string {
	return fmt.Sprintf("unsafe device %s: %s", e.Device, e.Reason)
}

// IsSafeToWipe always returns an error on non-Linux platforms.
func IsSafeToWipe(devicePath string, unsafeAllowAllUSB bool) error {
	return &SafetyError{Device: devicePath, Reason: "usb-wiper safety checks are only implemented on Linux"}
}

// IsSystemDevice always reports true (fail closed) on non-Linux.
func IsSystemDevice(devicePath string) (bool, error) {
	return true, nil
}

// GetRootDevice is not implemented on non-Linux platforms.
func GetRootDevice() (string, error) {
	return "", fmt.Errorf("GetRootDevice not implemented on non-Linux")
}
