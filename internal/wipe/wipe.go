package wipe

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"
)

const (
	_  = iota
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

const (
	bufferSize = 4 * 1024 * 1024 // 4 MiB
	chunkSize  = 1 * 1024 * 1024 // 1 MiB per verification chunk
)

// VerifyRandomChunks reads random 1 MiB chunks scattered across the device and
// checks that all bytes are zero. The total data read across all chunks
// approximately equals verifySize bytes (converted to MiB boundary).
// Returns the number of bytes actually verified.
func VerifyRandomChunks(ctx context.Context, devicePath string, totalBytes, verifySize uint64, progress chan<- ProgressEvent) (uint64, error) {
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
	readLen := uint64(chunkSize)
	if totalBytes < readLen {
		readLen = totalBytes
	}
	span := totalBytes - readLen // cannot underflow: readLen <= totalBytes
	totalVerified := uint64(0)

	// Generate random offsets
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
		offset := uint64(0)
		if span > 0 {
			offset = s0 % span
		}

		select {
		case <-ctx.Done():
			return totalVerified, ctx.Err()
		default:
		}

		n, err := f.ReadAt(buf[:readLen], int64(offset))
		if err != nil && !errors.Is(err, io.EOF) {
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

	if totalVerified == 0 {
		return 0, fmt.Errorf("verification read no data from %s; refusing to report a pass", devicePath)
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
