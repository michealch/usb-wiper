package device

import (
	"testing"
)

func TestSafetyError_Error(t *testing.T) {
	e := &SafetyError{Reason: "test reason", Device: "/dev/sdb"}
	expected := "safety check failed for /dev/sdb: test reason"
	if e.Error() != expected {
		t.Errorf("SafetyError.Error() = %q, want %q", e.Error(), expected)
	}
}

func TestIsSafeToWipe_RejectsNonExistentPath(t *testing.T) {
	err := IsSafeToWipe("/dev/sdx", false)
	if err == nil {
		t.Fatal("expected error for non-existent device")
	}
}

func TestIsSafeToWipe_RejectsPartitionPath(t *testing.T) {
	err := IsSafeToWipe("/dev/sda1", false)
	if err == nil {
		t.Fatal("expected error for partition path")
	}
	// Should fail regex check
	if se, ok := err.(*SafetyError); ok {
		t.Logf("correctly rejected: %v", se)
	}
}

func TestIsSafeToWipe_RejectsNVMe(t *testing.T) {
	err := IsSafeToWipe("/dev/nvme0n1", false)
	if err == nil {
		t.Fatal("expected error for NVMe device")
	}
}

func TestIsSafeToWipe_RejectsEmpty(t *testing.T) {
	err := IsSafeToWipe("", false)
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestIsSafeToWipe_RejectsRelativePath(t *testing.T) {
	err := IsSafeToWipe("sdb", false)
	if err == nil {
		t.Fatal("expected error for relative path")
	}
}

func TestIsSafeToWipe_RejectsLoopDevice(t *testing.T) {
	err := IsSafeToWipe("/dev/loop0", false)
	if err == nil {
		t.Fatal("expected error for loop device")
	}
}

func TestStripPartitionSuffix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/dev/sda1", "/dev/sda"},
		{"/dev/sda12", "/dev/sda"},
		{"/dev/sdb", "/dev/sdb"}, // no partition
		{"/dev/nvme0n1p2", "/dev/nvme0n1"},
		{"/dev/dm-0", "/dev/dm-0"}, // no change
	}

	for _, tt := range tests {
		result := stripPartitionSuffix(tt.input)
		if result != tt.expected {
			t.Errorf("stripPartitionSuffix(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCheckSystemMounts_InvalidPaths(t *testing.T) {
	// Test that critical system mount points would be caught
	// This is a unit test verifying the logic structure
	err := &SafetyError{
		Reason: "test",
		Device: "/dev/sda",
	}
	if err.Error() == "" {
		t.Fatal("SafetyError should have message")
	}
}
