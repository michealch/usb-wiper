package device

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

// SafetyError indicates a device failed a safety check.
type SafetyError struct {
	Reason string `json:"reason"`
	Device string `json:"device"`
}

func (e *SafetyError) Error() string {
	return fmt.Sprintf("safety check failed for %s: %s", e.Device, e.Reason)
}

// WARNING: DO NOT SIMPLIFY THESE CHECKS. EVERY CHECK IS REQUIRED.
// Removing or weakening any check can result in system disk destruction.

var sdDeviceRegex = regexp.MustCompile(`^/dev/sd[a-z]$`)

// systemMountPoints are mount points that indicate a device is a system disk.
var systemMountPoints = []string{"/", "/boot", "/home", "/var", "/usr", "/etc"}

const maxSafeSizeBytes = 2 * 1024 * 1024 * 1024 * 1024 // 2 TB

// IsSafeToWipe performs ALL safety checks on a device path.
// Returns nil only if the device is safe to wipe.
// The checks are ordered; the first failure is returned.
// When unsafeAllowAllUSB is true, Check 6 (removable flag) is skipped.
func IsSafeToWipe(devicePath string, unsafeAllowAllUSB bool) error {
	// Check 1: Path must match /dev/sd[a-z] (no partitions, no NVMe, no loop, no dm-*)
	if !sdDeviceRegex.MatchString(devicePath) {
		return &SafetyError{
			Reason: fmt.Sprintf("device path %q does not match pattern /dev/sd[a-z]; partitions and non-USB devices are rejected", devicePath),
			Device: devicePath,
		}
	}

	// Check 2: Device must exist
	info, err := os.Stat(devicePath)
	if err != nil {
		return &SafetyError{
			Reason: fmt.Sprintf("device %q does not exist: %v", devicePath, err),
			Device: devicePath,
		}
	}

	// Check 3: Must be a block device
	if info.Mode()&os.ModeDevice == 0 {
		return &SafetyError{
			Reason: fmt.Sprintf("%q is not a block device", devicePath),
			Device: devicePath,
		}
	}

	// Check 4: Must not be NVMe
	if strings.Contains(devicePath, "nvme") {
		return &SafetyError{
			Reason: "NVMe devices are not supported (system disk risk)",
			Device: devicePath,
		}
	}

	// Check 5: Not the root device
	rootDev, err := GetRootDevice()
	if err != nil {
		return &SafetyError{
			Reason: fmt.Sprintf("cannot determine root device: %v", err),
			Device: devicePath,
		}
	}
	if devicePath == rootDev {
		return &SafetyError{
			Reason: fmt.Sprintf("%q is the root device; refusing to wipe", devicePath),
			Device: devicePath,
		}
	}

	// Check 6: Must be removable (skipped when unsafeAllowAllUSB is set)
	name := strings.TrimPrefix(devicePath, "/dev/")
	if !unsafeAllowAllUSB {
		removablePath := filepath.Join("/sys/block", name, "removable")
		removableData, err := os.ReadFile(removablePath)
		if err != nil {
			return &SafetyError{
				Reason: fmt.Sprintf("cannot read removable flag for %s: %v", devicePath, err),
				Device: devicePath,
			}
		}
		if strings.TrimSpace(string(removableData)) != "1" {
			return &SafetyError{
				Reason: fmt.Sprintf("%q is not marked as removable", devicePath),
				Device: devicePath,
			}
		}
	}

	// Check 7: Must be on USB bus
	if !isUSBDevice(filepath.Join("/sys/block", name)) {
		return &SafetyError{
			Reason: fmt.Sprintf("%q is not on a USB bus", devicePath),
			Device: devicePath,
		}
	}

	// Check 8: Not mounted at a system path
	if err := checkSystemMounts(devicePath); err != nil {
		return err
	}

	// Check 9: Size sanity (must be <= 2 TB)
	sizePath := filepath.Join("/sys/block", name, "size")
	sizeData, err := os.ReadFile(sizePath)
	if err == nil {
		var sectors uint64
		fmt.Sscanf(strings.TrimSpace(string(sizeData)), "%d", &sectors)
		sizeBytes := sectors * 512
		if sizeBytes > maxSafeSizeBytes {
			return &SafetyError{
				Reason: fmt.Sprintf("%q is %d bytes (>2TB); refusing to wipe large disks that may be USB-attached system storage", devicePath, sizeBytes),
				Device: devicePath,
			}
		}
	}

	return nil
}

// IsSystemDevice checks if the device path is a system device.
func IsSystemDevice(devicePath string) (bool, error) {
	rootDev, err := GetRootDevice()
	if err != nil {
		return false, err
	}
	return devicePath == rootDev, nil
}

// GetRootDevice finds the base device name of the root filesystem.
// It parses /proc/mounts to find the device mounted at "/",
// then strips any partition suffix.
func GetRootDevice() (string, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return "", fmt.Errorf("read /proc/mounts: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		dev := fields[0]
		mountPoint := fields[1]

		if mountPoint == "/" {
			// Strip partition suffix (e.g., /dev/sda1 → /dev/sda)
			return stripPartitionSuffix(dev), nil
		}
	}

	// Fallback: try stat on /
	var stat syscall.Stat_t
	if err := syscall.Stat("/", &stat); err != nil {
		return "", fmt.Errorf("cannot stat root: %w", err)
	}
	// We can't easily get the device from stat on all systems
	return "", fmt.Errorf("root device not found in /proc/mounts")
}

func stripPartitionSuffix(dev string) string {
	// Remove trailing digits (partition number) from device path
	// /dev/sda1 → /dev/sda, /dev/nvme0n1p2 → /dev/nvme0n1
	re := regexp.MustCompile(`^(/dev/[a-z]+)[0-9]+$`)
	if matches := re.FindStringSubmatch(dev); len(matches) == 2 {
		return matches[1]
	}
	// For NVMe style: /dev/nvme0n1p2 → /dev/nvme0n1
	re = regexp.MustCompile(`^(/dev/nvme[0-9]+n[0-9]+)p[0-9]+$`)
	if matches := re.FindStringSubmatch(dev); len(matches) == 2 {
		return matches[1]
	}
	return dev
}

// checkSystemMounts verifies that neither the device nor any of its
// partitions are mounted at critical system paths.
func checkSystemMounts(devicePath string) error {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return &SafetyError{
			Reason: fmt.Sprintf("cannot read /proc/mounts: %v", err),
			Device: devicePath,
		}
	}

	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		dev := fields[0]
		mountPoint := fields[1]

		// Check if this mount belongs to our device or its partitions
		if dev == devicePath || strings.HasPrefix(dev, devicePath) {
			for _, systemPath := range systemMountPoints {
				if mountPoint == systemPath {
					return &SafetyError{
						Reason: fmt.Sprintf("%q has partition mounted at critical system path %q", devicePath, mountPoint),
						Device: devicePath,
					}
				}
			}
		}
	}

	return nil
}
