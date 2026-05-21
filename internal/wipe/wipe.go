package wipe

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/usb-wiper/internal/device"
)

// WipeJob tracks the state of an active or completed wipe operation.
type WipeJob struct {
	DevicePath   string    `json:"devicePath"`
	TotalBytes   uint64    `json:"totalBytes"`
	BytesWritten uint64    `json:"bytesWritten"`
	Status       string    `json:"status"` // "running", "completed", "failed", "cancelled"
	StartedAt    time.Time `json:"startedAt"`
	FinishedAt   time.Time `json:"finishedAt"`
	Error        string    `json:"error"`
	autoFormat   bool
	mu           sync.Mutex
	cancel       context.CancelFunc
}

const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"

	bufferSize = 4 * 1024 * 1024 // 4 MiB
)

// Wipe performs a single-pass zero-write on the specified device.
// Progress is reported via the progress channel.
func Wipe(ctx context.Context, devicePath string, progress chan<- ProgressEvent, unsafeAllowAllUSB bool) error {
	// CRITICAL: Re-validate safety before any destructive operation
	if err := device.IsSafeToWipe(devicePath, unsafeAllowAllUSB); err != nil {
		return fmt.Errorf("safety check: %w", err)
	}

	// Open device for writing
	f, err := os.OpenFile(devicePath, os.O_WRONLY|syscall.O_SYNC, 0)
	if err != nil {
		return fmt.Errorf("open device %s: %w", devicePath, err)
	}
	defer f.Close()

	// Get device size via ioctl BLKGETSIZE64
	totalBytes, err := blkGetSize64(f)
	if err != nil {
		return fmt.Errorf("get device size: %w", err)
	}

	// Allocate zero buffer
	buf := make([]byte, bufferSize)

	startTime := time.Now()
	var bytesWritten uint64
	var lastReport time.Time
	var samples []speedSample

	for bytesWritten < totalBytes {
		select {
		case <-ctx.Done():
			sendProgress(progress, devicePath, bytesWritten, totalBytes, startTime, samples, "cancelled")
			return context.Canceled
		default:
		}

		// Calculate remaining bytes
		remaining := totalBytes - bytesWritten
		writeSize := uint64(len(buf))
		if remaining < writeSize {
			writeSize = remaining
		}

		n, err := f.Write(buf[:writeSize])
		if err != nil {
			return fmt.Errorf("write at offset %d: %w", bytesWritten, err)
		}
		bytesWritten += uint64(n)

		// Report progress every 250ms
		now := time.Now()
		if now.Sub(lastReport) >= 250*time.Millisecond || bytesWritten >= totalBytes {
			sendProgress(progress, devicePath, bytesWritten, totalBytes, startTime, samples, "running")
			lastReport = now

			samples = append(samples, speedSample{
				bytesWritten: bytesWritten,
				timestamp:    now,
			})
			// Keep last 5 samples
			if len(samples) > 5 {
				samples = samples[len(samples)-5:]
			}
		}
	}

	// Sync and close
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	// Verify: read first and last 1 MiB, must all be zero
	if err := verifyZero(devicePath, totalBytes); err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	sendProgress(progress, devicePath, bytesWritten, totalBytes, startTime, samples, "completed")
	return nil
}

// blkGetSize64 retrieves device size in bytes using ioctl BLKGETSIZE64.
func blkGetSize64(f *os.File) (uint64, error) {
	var size uint64
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), 0x80081272, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return 0, fmt.Errorf("ioctl BLKGETSIZE64: %v", errno)
	}
	return size, nil
}

// verifyZero confirms that the first and last 1 MiB of the device are all zeros.
func verifyZero(devicePath string, totalBytes uint64) error {
	f, err := os.Open(devicePath)
	if err != nil {
		return fmt.Errorf("open for verification: %w", err)
	}
	defer f.Close()

	checkSize := uint64(1024 * 1024) // 1 MiB

	// Check first 1 MiB
	buf := make([]byte, checkSize)
	if _, err := f.ReadAt(buf, 0); err != nil {
		return fmt.Errorf("read beginning for verification: %w", err)
	}
	for i, b := range buf {
		if b != 0 {
			return fmt.Errorf("non-zero byte at offset %d: 0x%02x", i, b)
		}
	}

	// Check last 1 MiB
	if totalBytes > checkSize {
		offset := int64(totalBytes - checkSize)
		if _, err := f.ReadAt(buf, offset); err != nil {
			return fmt.Errorf("read end for verification: %w", err)
		}
		for i, b := range buf {
			if b != 0 {
				return fmt.Errorf("non-zero byte at offset %d: 0x%02x", int64(offset)+int64(i), b)
			}
		}
	}

	return nil
}

func sendProgress(ch chan<- ProgressEvent, devicePath string, written, total uint64, start time.Time, samples []speedSample, status string) {
	elapsed := time.Since(start)
	var pct float64
	if total > 0 {
		pct = float64(written) / float64(total) * 100
	}

	speed := computeSpeed(samples)
	var eta time.Duration
	if speed > 0 && total > written {
		eta = time.Duration(float64(total-written)/float64(speed)) * time.Second
	}

	msg := ""
	switch status {
	case "completed":
		msg = fmt.Sprintf("Wipe completed successfully in %s", elapsed.Round(time.Second))
	case "cancelled":
		msg = "Wipe cancelled"
	case "failed":
		msg = "Wipe failed"
	default:
		msg = fmt.Sprintf("Wiping... %.1f%%", pct)
	}

	event := ProgressEvent{
		DevicePath:   devicePath,
		BytesWritten: written,
		TotalBytes:   total,
		Percent:      pct,
		Speed:        speed,
		ETA:          eta,
		Status:       status,
		Message:      msg,
		Timestamp:    time.Now(),
	}

	// Non-blocking send
	select {
	case ch <- event:
	default:
	}
}
