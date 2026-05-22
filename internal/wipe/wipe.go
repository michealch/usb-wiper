package wipe

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/usb-wiper/internal/device"
)

const (
	_ = iota
	KB = 1 << (10 * iota)
	MB
	GB
)

func formatBytes(n uint64) string {
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1f GiB", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(MB))
	case n >= KB:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(KB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// FormatBytes formats a byte count into a human-readable string.
func FormatBytes(n uint64) string {
	return formatBytes(n)
}

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
	totalBytes, err := BlkGetSize64(f)
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

		// Report verification progress (with message only on milestones
		// to keep the log clean; progress bar still updates every chunk)
		if progress != nil {
			vPct := float64(i+1) / float64(numChunks) * 100
			vMsg := ""
			if i == 0 || i == numChunks-1 || (i+1)%(numChunks/5+1) == 0 {
				vMsg = fmt.Sprintf("Verifying %s: chunk %d/%d (%.0f%% done)", devicePath, i+1, numChunks, vPct)
			}
			select {
			case progress <- ProgressEvent{
				DevicePath:   devicePath,
				Status:       "verifying",
				Percent:      vPct,
				BytesWritten: totalVerified,
				TotalBytes:   verifySize,
				Message:      vMsg,
				Timestamp:    time.Now(),
			}:
			default:
			}
		}
	}

	return totalVerified, nil
}

// openDeviceWrite opens a device file for direct writing (O_SYNC).
func openDeviceWrite(devicePath string) (*os.File, error) {
	return os.OpenFile(devicePath, os.O_WRONLY|syscall.O_SYNC, 0)
}

// seekToStart seeks the file descriptor to byte 0.
func seekToStart(f *os.File) error {
	_, err := f.Seek(0, 0)
	return err
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
	speedStr := ""
	etaStr := ""
	if speed > 0 {
		speedStr = fmt.Sprintf(" @ %s/s", formatBytes(speed))
		if eta > 0 {
			etaStr = fmt.Sprintf(" ETA %s", eta.Round(time.Second))
		}
	}

	switch status {
	case "completed":
		// Final event handled by the handlers layer with richer context
		msg = fmt.Sprintf("Wiping %s: %s / %s (%.1f%%)%s%s", devicePath, formatBytes(written), formatBytes(total), pct, speedStr, etaStr)
	case "cancelled":
		msg = fmt.Sprintf("Wipe %s cancelled after %s (%s written)", devicePath, elapsed.Round(time.Second), formatBytes(written))
	case "failed":
		msg = fmt.Sprintf("Wipe %s failed after %s (%s written)", devicePath, elapsed.Round(time.Second), formatBytes(written))
	default:
		// Omit message during normal progress — the per-row progress bar
		// already shows percentage, speed, and ETA. Only emit message on
		// 0% (start), 100% (final), and every 10% milestones to keep the
		// log concise.
		if written == 0 || written >= total {
			msg = fmt.Sprintf("Wiping %s: %s / %s (%.1f%%)%s%s", devicePath, formatBytes(written), formatBytes(total), pct, speedStr, etaStr)
		}
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
