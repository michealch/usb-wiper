package wipe

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/usb-wiper/internal/device"
)

// WipeJob tracks the state of an active or completed wipe operation.
type WipeJob struct {
	DevicePath    string    `json:"devicePath"`
	TotalBytes    uint64    `json:"totalBytes"`
	BytesWritten  uint64    `json:"bytesWritten"`
	Status        string    `json:"status"` // "running", "completed", "failed", "cancelled"
	StartedAt     time.Time `json:"startedAt"`
	FinishedAt    time.Time `json:"finishedAt"`
	Error         string    `json:"error"`
	Verified      string    `json:"verified,omitempty"`   // "passed", "failed", or empty
	BytesVerified uint64    `json:"bytesVerified"`        // how many bytes were verified
}

const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"

	bufferSize  = 4 * 1024 * 1024 // 4 MiB
	chunkSize   = 1 * 1024 * 1024 // 1 MiB per verification chunk
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

	sendProgress(progress, devicePath, bytesWritten, totalBytes, startTime, samples, "completed")
	return nil
}

// VerifyRandomChunks reads random 1 MiB chunks scattered across the device and
// checks that all bytes are zero. The total data read across all chunks
// approximately equals verifySize bytes (converted to MiB boundary).
// Returns the number of bytes actually verified.
func VerifyRandomChunks(devicePath string, totalBytes, verifySize uint64, progress chan<- ProgressEvent) (uint64, error) {
	if verifySize == 0 || totalBytes == 0 {
		return 0, nil
	}

	// Ensure verifySize doesn't exceed total device size
	if verifySize > totalBytes {
		verifySize = totalBytes
	}

	// Calculate number of chunks (each chunk is 1 MiB)
	numChunks := int(verifySize / chunkSize)
	if numChunks < 1 {
		numChunks = 1
	}

	// Cap to reasonable number
	maxChunks := 10000
	if numChunks > maxChunks {
		numChunks = maxChunks
	}

	f, err := os.Open(devicePath)
	if err != nil {
		return 0, fmt.Errorf("open for verification: %w", err)
	}
	defer f.Close()

	buf := make([]byte, chunkSize)
	totalVerified := uint64(0)

	// Generate random offsets
	maxOffset := totalBytes - chunkSize
	// Use a deterministic seed derived from time so we can report which offsets
	// were checked, while still being "random enough" across the device.
	seed := make([]byte, 8)
	if _, err := rand.Read(seed); err != nil {
		// Fallback: if crypto/rand fails, use time-based
		binary.BigEndian.PutUint64(seed, uint64(time.Now().UnixNano()))
	}
	// Simple xoshiro-like state
	s0 := binary.BigEndian.Uint64(seed)
	s1 := s0 ^ 0x9E3779B97F4A7C15

	for i := 0; i < numChunks; i++ {
		// Generate next random offset
		s1 ^= s0
		s0 = ((s0 << 55) | (s0 >> 9)) ^ s1 ^ (s1 << 14)
		s1 = ((s1 << 36) | (s1 >> 28))
		offset := s0 % maxOffset

		n, err := f.ReadAt(buf, int64(offset))
		if err != nil && err.Error() != "EOF" {
			return totalVerified, fmt.Errorf("read at offset %d: %w", offset, err)
		}
		if n == 0 {
			continue
		}

		// Check all bytes are zero
		for j := 0; j < n; j++ {
			if buf[j] != 0 {
				return totalVerified, fmt.Errorf("non-zero byte at offset %d: 0x%02x at chunk %d/%d", int64(offset)+int64(j), buf[j], i+1, numChunks)
			}
		}

		totalVerified += uint64(n)

		// Report verification progress
		if progress != nil {
			vPct := float64(i+1) / float64(numChunks) * 100
			select {
			case progress <- ProgressEvent{
				DevicePath:   devicePath,
				Status:       "verifying",
				Percent:      vPct,
				BytesWritten: totalVerified,
				TotalBytes:   verifySize,
				Message:      fmt.Sprintf("Verifying... %.1f%% (%d/%d chunks)", vPct, i+1, numChunks),
				Timestamp:    time.Now(),
			}:
			default:
			}
		}
	}

	return totalVerified, nil
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
