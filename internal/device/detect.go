package device

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Device represents a USB block device detected on the system.
type Device struct {
	Path        string   `json:"path"`
	Name        string   `json:"name"`
	Model       string   `json:"model"`
	Serial      string   `json:"serial"`
	SizeBytes   uint64   `json:"sizeBytes"`
	Removable   bool     `json:"removable"`
	IsUSB       bool     `json:"isUSB"`
	WipeBlocked bool     `json:"wipeBlocked"`
	BlockReason string   `json:"blockReason"`
	Mounted     bool     `json:"mounted"`
	MountPoints []string `json:"mountPoints"`
}

// ListUSBDevices enumerates all USB block devices on the system.
func ListUSBDevices(unsafeAllowAllUSB bool) ([]Device, error) {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, fmt.Errorf("read /sys/block: %w", err)
	}

	var devices []Device
	for _, entry := range entries {
		name := entry.Name()

		// Skip non-disk devices
		if skipDevice(name) {
			continue
		}

		dev, err := probeDevice(name)
		if err != nil {
			continue // skip devices we can't fully probe
		}

		// Show USB-connected devices even if they don't report as removable
		// (common with USB SSDs using UASP). Safety checks at wipe time
		// will still enforce the removable flag.
		if dev.IsUSB {
			devices = append(devices, *dev)
		}
	}

	// Annotate devices with wipe-block status for UI display
	for i := range devices {
		if err := IsSafeToWipe(devices[i].Path, unsafeAllowAllUSB); err != nil {
			devices[i].WipeBlocked = true
			devices[i].BlockReason = err.Error()
		}
	}

	return devices, nil
}

// GetDevice returns a single device by its path (e.g., "/dev/sdb").
func GetDevice(devicePath string) (*Device, error) {
	name := strings.TrimPrefix(devicePath, "/dev/")
	if name == devicePath || name == "" {
		return nil, fmt.Errorf("invalid device path: %s", devicePath)
	}

	return probeDevice(name)
}

func skipDevice(name string) bool {
	prefixes := []string{"loop", "ram", "dm-", "nvme", "md"}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func probeDevice(name string) (*Device, error) {
	base := filepath.Join("/sys/block", name)

	// Check removable
	removable := readSysfsBool(base, "removable")

	// Check if USB
	isUSB := isUSBDevice(base)

	// Read model
	model := readSysfsString(filepath.Join(base, "device"), "model")

	// Read serial
	serial := readSysfsString(filepath.Join(base, "device"), "serial")

	// Read size (in 512-byte sectors)
	size := readSysfsUint64(base, "size")

	// Check mounts
	mountPoints := findMountPoints(name)

	dev := &Device{
		Path:        "/dev/" + name,
		Name:        name,
		Model:       model,
		Serial:      serial,
		SizeBytes:   size * 512,
		Removable:   removable,
		IsUSB:       isUSB,
		Mounted:     len(mountPoints) > 0,
		MountPoints: mountPoints,
	}

	return dev, nil
}

func isUSBDevice(sysBlockPath string) bool {
	// /sys/block/sda is a symlink, and /sys/block/sda/device is another
	// relative symlink (e.g., "../../../0:0:0:0"). The raw link text
	// doesn't contain "/usb", but the resolved absolute path does.
	// We use EvalSymlinks to get the real path.
	devicePath := filepath.Join(sysBlockPath, "device")
	resolved, err := filepath.EvalSymlinks(devicePath)
	if err != nil {
		return false
	}
	return strings.Contains(resolved, "/usb")
}

func findMountPoints(devName string) []string {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil
	}

	var points []string
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		devPath := fields[0]
		if strings.HasPrefix(devPath, "/dev/"+devName) {
			points = append(points, fields[1])
		}
	}
	return points
}

func readSysfsBool(base, file string) bool {
	data, err := os.ReadFile(filepath.Join(base, file))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "1"
}

func readSysfsString(base, file string) string {
	data, err := os.ReadFile(filepath.Join(base, file))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readSysfsUint64(base, file string) uint64 {
	data, err := os.ReadFile(filepath.Join(base, file))
	if err != nil {
		return 0
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
