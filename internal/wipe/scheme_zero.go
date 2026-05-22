package wipe

import (
	"context"
	"fmt"
	"time"
)

// SchemeZero performs a single pass of zeros.
type SchemeZero struct{}

func (s *SchemeZero) ID() string          { return "zero" }
func (s *SchemeZero) DisplayName() string { return "Zero Fill" }
func (s *SchemeZero) Passes() int          { return 1 }

func (s *SchemeZero) Execute(ctx context.Context, devicePath string, size uint64,
	progress chan<- ProgressEvent) error {
	return multiPassWipe(ctx, devicePath, size, progress, []writePass{
		{fillByte: 0x00, label: "Pass 1/1: Zero fill"},
	})
}

// SchemeNISTClear is identical to Zero but tagged for NIST compliance metadata.
type SchemeNISTClear struct{ SchemeZero }

func (s *SchemeNISTClear) ID() string          { return "nist-clear" }
func (s *SchemeNISTClear) DisplayName() string { return "NIST 800-88 Clear" }
func (s *SchemeNISTClear) Passes() int          { return 1 }

// SchemeRandom performs a single pass of crypto-random bytes.
type SchemeRandom struct{}

func (s *SchemeRandom) ID() string          { return "random" }
func (s *SchemeRandom) DisplayName() string { return "Random Fill" }
func (s *SchemeRandom) Passes() int          { return 1 }

func (s *SchemeRandom) Execute(ctx context.Context, devicePath string, size uint64,
	progress chan<- ProgressEvent) error {
	return multiPassWipe(ctx, devicePath, size, progress, []writePass{
		{fillRandom: true, label: "Pass 1/1: Random fill"},
	})
}

// ---- DoD 5220.22-M 3-pass ----

// SchemeDoD performs a 3-pass wipe: zeros, 0xFF, random.
type SchemeDoD struct{}

func (s *SchemeDoD) ID() string          { return "dod-3pass" }
func (s *SchemeDoD) DisplayName() string { return "DoD 5220.22-M 3-Pass" }
func (s *SchemeDoD) Passes() int          { return 3 }

func (s *SchemeDoD) Execute(ctx context.Context, devicePath string, size uint64,
	progress chan<- ProgressEvent) error {
	return multiPassWipe(ctx, devicePath, size, progress, []writePass{
		{fillByte: 0x00, label: "Pass 1/3: Zero fill"},
		{fillByte: 0xFF, label: "Pass 2/3: Ones fill"},
		{fillRandom: true, label: "Pass 3/3: Random fill"},
	})
}

// ---- writePass defines a single pass in a multi-pass wipe ----

type writePass struct {
	fillByte   byte
	fillRandom bool
	label      string
}

// multiPassWipe is the shared engine that drives one or more write passes.
// Each pass writes the full device using 4 MiB buffers, reports progress,
// and syncs after each pass.
func multiPassWipe(ctx context.Context, devicePath string, size uint64,
	progress chan<- ProgressEvent, passes []writePass) error {

	f, err := openDeviceWrite(devicePath)
	if err != nil {
		return fmt.Errorf("open device %s: %w", devicePath, err)
	}
	defer f.Close()
	// Ensure data reaches the device even on cancellation.
	// Close() alone does not guarantee a flush.
	defer f.Sync()

	buf := make([]byte, bufferSize)
	startTime := time.Now()
	var totalWritten uint64 // cumulative across all passes
	var samples []speedSample
	var lastReport time.Time

	totalPasses := len(passes)

	for passIdx, p := range passes {
		// Reset position to start for each pass
		if err := seekToStart(f); err != nil {
			return fmt.Errorf("seek start pass %d: %w", passIdx+1, err)
		}

		// Prepare buffer: fill once per pass for byte-based fills.
		// Random fills are done per-chunk.
		if !p.fillRandom {
			for i := range buf {
				buf[i] = p.fillByte
			}
		}

		var passWritten uint64

		for passWritten < size {
			select {
			case <-ctx.Done():
				sendProgress(progress, devicePath, totalWritten, uint64(totalPasses)*size,
					startTime, samples, "cancelled")
				return context.Canceled
			default:
			}

			remaining := size - passWritten
			writeSize := uint64(len(buf))
			if remaining < writeSize {
				writeSize = remaining
			}

			// For random pass, re-fill buffer each chunk
			if p.fillRandom {
				fillRandom(buf[:writeSize])
			}

			n, err := f.Write(buf[:writeSize])
			if err != nil {
				return fmt.Errorf("write pass %d at offset %d: %w", passIdx+1, passWritten, err)
			}
			passWritten += uint64(n)
			totalWritten += uint64(n)

			now := time.Now()
			if now.Sub(lastReport) >= 250*time.Millisecond || passWritten >= size {
				reportMultiPassProgress(progress, devicePath, totalWritten, uint64(totalPasses)*size,
					startTime, samples, p.label, passIdx+1, totalPasses)
				lastReport = now

				samples = append(samples, speedSample{
					bytesWritten: totalWritten,
					timestamp:    now,
				})
				if len(samples) > 5 {
					samples = samples[len(samples)-5:]
				}
			}
		}

		// Sync after each pass
		if err := f.Sync(); err != nil {
			return fmt.Errorf("sync pass %d: %w", passIdx+1, err)
		}
	}

	// Final sync and close
	if err := f.Sync(); err != nil {
		return fmt.Errorf("final sync: %w", err)
	}

	reportMultiPassProgress(progress, devicePath, totalWritten, uint64(totalPasses)*size,
		startTime, samples, "pass "+fmt.Sprint(totalPasses)+"/"+fmt.Sprint(totalPasses), totalPasses, totalPasses)
	return nil
}

func reportMultiPassProgress(ch chan<- ProgressEvent, devicePath string,
	written, total uint64, start time.Time, samples []speedSample,
	label string, currentPass, totalPasses int) {

	var pct float64
	if total > 0 {
		pct = float64(written) / float64(total) * 100
	}

	speed := computeSpeed(samples)
	var eta time.Duration
	if speed > 0 && total > written {
		eta = time.Duration(float64(total-written)/float64(speed)) * time.Second
	}

	msg := fmt.Sprintf("Wiping %s: %s — %s / %s (%.1f%%)", devicePath,
		label, formatBytes(written), formatBytes(total), pct)
	if speed > 0 {
		msg += fmt.Sprintf(" @ %s/s", formatBytes(speed))
	}
	if eta > 0 {
		msg += fmt.Sprintf(" ETA %s", eta.Round(time.Second))
	}

	event := ProgressEvent{
		DevicePath:   devicePath,
		BytesWritten: written,
		TotalBytes:   total,
		Percent:      pct,
		Speed:        speed,
		ETA:          eta,
		Status:       "running",
		CurrentPass:  currentPass,
		TotalPasses:  totalPasses,
		Message:      msg,
		Timestamp:    time.Now(),
	}

	select {
	case ch <- event:
	default:
	}
}
