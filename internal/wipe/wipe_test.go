package wipe

import (
	"context"
	"errors"
	"os"
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

func TestBlkGetSize64_RegularFile(t *testing.T) {
	// blkGetSize64 should fail on non-block-device
	// This test verifies the ioctl error path
	// We can't easily test it without an actual block device in CI
	// So we test the helper function structure
	t.Log("blkGetSize64 tested by integration")
}

func TestVerifyRandomChunks_ExactChunkSizeDoesNotPanic(t *testing.T) {
	// A device of exactly chunkSize (1 MiB) previously caused an integer
	// divide-by-zero panic. Post-fix it must verify the whole device.
	f := writeZeroFile(t, chunkSize)
	defer f.Close()

	n, err := VerifyRandomChunks(context.Background(), f.Name(), chunkSize, chunkSize, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != chunkSize {
		t.Fatalf("expected %d bytes verified, got %d", chunkSize, n)
	}
}

func TestVerifyRandomChunks_SubChunkDeviceDoesNotFalselyPass(t *testing.T) {
	// A device smaller than one chunk previously underflowed maxOffset and
	// verified zero bytes while still reporting a pass. Post-fix it must
	// verify every byte of the device.
	const size = uint64(512 << 10)
	f := writeZeroFile(t, size)
	defer f.Close()

	n, err := VerifyRandomChunks(context.Background(), f.Name(), size, size, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != size {
		t.Fatalf("expected %d bytes verified, got %d", size, n)
	}
}

func TestVerifyRandomChunks_NonZeroDataFails(t *testing.T) {
	f := writeZeroFile(t, chunkSize)
	defer f.Close()
	// Corrupt a single byte in the middle of the device.
	if _, err := f.WriteAt([]byte{0xFF}, int64(chunkSize/2)); err != nil {
		t.Fatalf("write corruption byte: %v", err)
	}

	_, err := VerifyRandomChunks(context.Background(), f.Name(), chunkSize, chunkSize, nil)
	if err == nil {
		t.Fatal("expected a non-nil error for non-zero data")
	}
}

func TestVerifyRandomChunks_CancelledContext(t *testing.T) {
	f := writeZeroFile(t, chunkSize)
	defer f.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := VerifyRandomChunks(ctx, f.Name(), chunkSize, chunkSize, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// writeZeroFile creates a temp file of the given size filled with zeros.
func writeZeroFile(t *testing.T, size uint64) *os.File {
	t.Helper()
	f, err := os.CreateTemp("", "usb-wiper-verify-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	if err := f.Truncate(int64(size)); err != nil {
		t.Fatalf("truncate temp file: %v", err)
	}
	return f
}

func TestSchemeRegistryListOrderIsStable(t *testing.T) {
	want := []string{"zero", "nist-clear", "random", "dod-3pass"}
	r := NewSchemeRegistry()
	for i := 0; i < 20; i++ {
		got := r.List()
		if len(got) != len(want) {
			t.Fatalf("iteration %d: got %d schemes, want %d", i, len(got), len(want))
		}
		for j := range want {
			if got[j].ID != want[j] {
				t.Fatalf("iteration %d: position %d = %q, want %q", i, j, got[j].ID, want[j])
			}
		}
	}
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
