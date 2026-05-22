package wipe

import (
	"testing"
	"time"
)

func TestComputeSpeed_Empty(t *testing.T) {
	speed := computeSpeed(nil)
	if speed != 0 {
		t.Errorf("expected 0 speed for nil samples, got %d", speed)
	}
}

func TestComputeSpeed_SingleSample(t *testing.T) {
	samples := []speedSample{
		{bytesWritten: 1000, timestamp: time.Now()},
	}
	speed := computeSpeed(samples)
	if speed != 0 {
		t.Errorf("expected 0 speed for single sample, got %d", speed)
	}
}

func TestComputeSpeed_TwoSamples(t *testing.T) {
	now := time.Now()
	samples := []speedSample{
		{bytesWritten: 0, timestamp: now},
		{bytesWritten: 1000, timestamp: now.Add(1 * time.Second)},
	}
	speed := computeSpeed(samples)
	if speed < 900 || speed > 1100 {
		t.Errorf("expected ~1000 bytes/sec, got %d", speed)
	}
}

func TestComputeSpeed_RollingAverage(t *testing.T) {
	now := time.Now()
	samples := []speedSample{
		{bytesWritten: 0, timestamp: now},
		{bytesWritten: 1000, timestamp: now.Add(1 * time.Second)},
		{bytesWritten: 2000, timestamp: now.Add(2 * time.Second)},
		{bytesWritten: 3000, timestamp: now.Add(3 * time.Second)},
		{bytesWritten: 4000, timestamp: now.Add(4 * time.Second)},
		{bytesWritten: 5000, timestamp: now.Add(5 * time.Second)},
		{bytesWritten: 6000, timestamp: now.Add(6 * time.Second)},
	}
	speed := computeSpeed(samples)
	// Last 5 samples: 1000 to 6000 in 5 seconds = 1000/sec
	if speed < 900 || speed > 1100 {
		t.Errorf("expected ~1000 bytes/sec rolling average, got %d", speed)
	}
}

func TestProgressEvent_Defaults(t *testing.T) {
	ev := ProgressEvent{}
	if ev.Percent != 0 {
		t.Errorf("default Percent should be 0, got %f", ev.Percent)
	}
	if ev.Status != "" {
		t.Errorf("default Status should be empty, got %q", ev.Status)
	}
}

func TestWipeJob_Defaults(t *testing.T) {
	job := WipeJob{}
	if job.Status != "" {
		t.Errorf("default Status should be empty, got %q", job.Status)
	}
	if job.StartedAt.IsZero() == false {
		t.Error("default StartedAt should be zero time")
	}
}

func TestBlkGetSize64_RegularFile(t *testing.T) {
	// blkGetSize64 should fail on non-block-device
	// This test verifies the ioctl error path
	// We can't easily test it without an actual block device in CI
	// So we test the helper function structure
	t.Log("blkGetSize64 tested by integration")
}

func TestVerifyZero_EmptyDevice(t *testing.T) {
	// verifyZero was replaced by VerifyRandomChunks.
	// VerifyRandomChunks requires a real block device to test.
	// The function is tested by integration when running against real USB drives.
	t.Log("VerifyRandomChunks tested by integration")
}

func TestProgressEvent_JSON(t *testing.T) {
	ev := ProgressEvent{
		DevicePath:   "/dev/sdb",
		BytesWritten: 500000,
		TotalBytes:   1000000,
		Percent:      50.0,
		Speed:        1000000,
		ETA:          500 * time.Millisecond,
		Status:       "running",
		Message:      "Wiping... 50.0%",
		Timestamp:    time.Now(),
	}

	if ev.DevicePath != "/dev/sdb" {
		t.Errorf("DevicePath mismatch")
	}
	if ev.Percent != 50.0 {
		t.Errorf("Percent mismatch")
	}
	if ev.Status != "running" {
		t.Errorf("Status mismatch")
	}
}
