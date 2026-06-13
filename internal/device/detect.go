package device

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Device represents a USB block device detected on the system.
type Device struct {
	Path               string              `json:"path"`
	Name               string              `json:"name"`
	DeviceID           string              `json:"deviceId"`
	IdentitySource     string              `json:"identitySource"`
	IdentityConfidence string              `json:"identityConfidence"`
	Model              string              `json:"model"`
	Serial             string              `json:"serial"`
	Firmware           string              `json:"firmware,omitempty"`
	WWN                string              `json:"wwn,omitempty"`
	SizeBytes          uint64              `json:"sizeBytes"`
	Removable          bool                `json:"removable"`
	IsUSB              bool                `json:"isUSB"`
	WipeBlocked        bool                `json:"wipeBlocked"`
	BlockReason        string              `json:"blockReason"`
	Wiping             bool                `json:"wiping"`
	WipeStatus         string              `json:"wipeStatus"`
	WipePercent        float64             `json:"wipePercent"`
	Mounted            bool                `json:"mounted"`
	MountPoints        []string            `json:"mountPoints"`
	WipeHistory        *WipeHistorySummary `json:"wipeHistory,omitempty"`
	HealthLatest       *HealthSummary      `json:"healthLatest,omitempty"`
}

// WipeHistorySummary carries the most recent wipe outcome for this device.
type WipeHistorySummary struct {
	Status       string    `json:"status"`       // "completed", "failed", "cancelled"
	Verification string    `json:"verification"` // "passed", "failed", or empty
	FinishedAt   time.Time `json:"finishedAt"`
}

// HealthSummary carries the most recent persisted SMART/health snapshot.
type HealthSummary struct {
	HealthStatus        string    `json:"healthStatus"`
	TemperatureC        int       `json:"temperatureC,omitempty"`
	PowerOnHours        uint64    `json:"powerOnHours,omitempty"`
	EnduranceUsedPct    int       `json:"enduranceUsedPct,omitempty"`
	UncorrectableErrors uint64    `json:"uncorrectableErrors,omitempty"`
	CapturedAt          time.Time `json:"capturedAt"`
}

// ListUSBDevices enumerates all USB block devices on the system.
func ListUSBDevices(unsafeAllowAllUSB bool) ([]Device, error) {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, fmt.Errorf("read /sys/block: %w", err)
	}

	devices := make([]Device, 0)
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

	// Read size (in 512-byte sectors)
	size := readSysfsUint64(base, "size")
	sizeBytes := size * 512

	// Read identity: prefer the disk behind the USB bridge, not the bridge.
	identity := readDeviceIdentity(name, sizeBytes)

	// Check mounts
	mountPoints := findMountPoints(name)

	dev := &Device{
		Path:               "/dev/" + name,
		Name:               name,
		DeviceID:           identity.DeviceID,
		IdentitySource:     identity.Source,
		IdentityConfidence: identity.Confidence,
		Model:              identity.Model,
		Serial:             identity.Serial,
		Firmware:           identity.Firmware,
		WWN:                identity.WWN,
		SizeBytes:          sizeBytes,
		Removable:          removable,
		IsUSB:              isUSB,
		Mounted:            len(mountPoints) > 0,
		MountPoints:        mountPoints,
	}

	return dev, nil
}

type Identity struct {
	DeviceID   string
	Source     string
	Confidence string
	Model      string
	Serial     string
	Firmware   string
	WWN        string
	SizeBytes  uint64
}

// readDeviceIdentity obtains identity for the physical disk behind a USB
// bridge. Low-confidence fallback IDs intentionally include kernel diskseq so
// swapped drives do not inherit history from a reused bridge or /dev path.
func readDeviceIdentity(devName string, sizeBytes uint64) Identity {
	devicePath := "/dev/" + devName
	smart := GetSmartIdentity(devicePath)

	// Sysfs fallback values
	base := filepath.Join("/sys/block", devName)
	sysfsModel := readSysfsString(filepath.Join(base, "device"), "model")
	sysfsSerial := readSysfsString(filepath.Join(base, "device"), "serial")
	diskSeq := readSysfsString(base, "diskseq")

	identity := Identity{
		Model:     firstNonEmpty(smart.Model, sysfsModel),
		Serial:    firstNonEmpty(smart.Serial, sysfsSerial),
		Firmware:  smart.Firmware,
		WWN:       smart.WWN,
		SizeBytes: firstNonZero(smart.CapacityBytes, sizeBytes),
	}

	switch {
	case identity.WWN != "":
		identity.Source = "smart-wwn"
		identity.Confidence = "high"
		identity.DeviceID = stableDeviceID("wwn", identity.WWN)
	case smart.Serial != "" && identity.Model != "":
		identity.Source = "smart-model-serial-capacity"
		identity.Confidence = "high"
		identity.DeviceID = stableDeviceID("smart", identity.Model, smart.Serial, strconv.FormatUint(identity.SizeBytes, 10))
	case identity.Serial != "" && identity.Model != "":
		identity.Source = "sysfs-model-serial-capacity"
		identity.Confidence = "medium"
		identity.DeviceID = stableDeviceID("sysfs", identity.Model, identity.Serial, strconv.FormatUint(identity.SizeBytes, 10))
	default:
		identity.Source = "attachment-diskseq"
		identity.Confidence = "low"
		identity.DeviceID = stableDeviceID("unknown", devicePath, diskSeq, resolvedSysfsDevicePath(base), identity.Model, strconv.FormatUint(identity.SizeBytes, 10))
	}

	return identity
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

func stableDeviceID(parts ...string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized = append(normalized, strings.ToLower(strings.TrimSpace(part)))
	}
	sum := sha256.Sum256([]byte(strings.Join(normalized, "\x00")))
	return "dev_" + hex.EncodeToString(sum[:])[:32]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonZero(values ...uint64) uint64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func resolvedSysfsDevicePath(sysBlockPath string) string {
	resolved, err := filepath.EvalSymlinks(filepath.Join(sysBlockPath, "device"))
	if err != nil {
		return ""
	}
	return resolved
}
