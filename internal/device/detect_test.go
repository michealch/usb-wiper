package device

import (
	"testing"
)

func TestSkipDevice(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"sda", false},
		{"sdb", false},
		{"loop0", true},
		{"loop1", true},
		{"ram0", true},
		{"ram15", true},
		{"dm-0", true},
		{"dm-1", true},
		{"nvme0n1", true},
		{"md0", true},
		{"sr0", false},
	}

	for _, tt := range tests {
		result := skipDevice(tt.name)
		if result != tt.expected {
			t.Errorf("skipDevice(%q) = %v, want %v", tt.name, result, tt.expected)
		}
	}
}

func TestProbeDevice_Nonexistent(t *testing.T) {
	dev, err := probeDevice("nonexistent")
	// probeDevice doesn't explicitly error on missing sysfs; it returns defaults
	if dev != nil {
		// Device returned but should have empty defaults
		if dev.Removable {
			t.Error("nonexistent device should not be removable")
		}
		if dev.IsUSB {
			t.Error("nonexistent device should not be USB")
		}
	}
	_ = err
}

func TestIsUSBDevice(t *testing.T) {
	// Testing a non-existent path should return false
	result := isUSBDevice("/sys/block/nonexistent")
	if result {
		t.Fatal("expected false for nonexistent device")
	}
}

func TestReadSysfsBool_Missing(t *testing.T) {
	result := readSysfsBool("/nonexistent", "removable")
	if result {
		t.Fatal("expected false for missing file")
	}
}

func TestReadSysfsString_Missing(t *testing.T) {
	result := readSysfsString("/nonexistent", "model")
	if result != "" {
		t.Fatalf("expected empty string for missing file, got %q", result)
	}
}

func TestReadSysfsUint64_Missing(t *testing.T) {
	result := readSysfsUint64("/nonexistent", "size")
	if result != 0 {
		t.Fatalf("expected 0 for missing file, got %d", result)
	}
}

func TestGetDevice_EmptyPath(t *testing.T) {
	dev, err := GetDevice("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if dev != nil {
		t.Error("expected nil device for empty path")
	}
}

func TestGetDevice_InvalidPath(t *testing.T) {
	dev, err := GetDevice("/dev")
	if err == nil {
		t.Fatal("expected error for directory path")
	}
	if dev != nil {
		t.Error("expected nil device for invalid path")
	}
}

func TestListUSBDevices_ReturnsEmptyOnNone(t *testing.T) {
	// On non-Linux (macOS/Windows), /sys/block doesn't exist — that's expected.
	devices, err := ListUSBDevices(false)
	if err != nil {
		t.Skipf("sysfs not available on this platform: %v", err)
		return
	}
	if devices == nil {
		t.Error("expected empty slice, got nil")
	}
}

func TestListUSBDevices_UnsafeMode(t *testing.T) {
	devices, err := ListUSBDevices(true)
	if err != nil {
		t.Skipf("sysfs not available on this platform: %v", err)
		return
	}
	if devices == nil {
		t.Error("expected empty slice, got nil")
	}
}
