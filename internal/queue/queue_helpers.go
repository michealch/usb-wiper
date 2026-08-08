package queue

import (
	"context"
	"os"

	"github.com/usb-wiper/internal/format"
	"github.com/usb-wiper/internal/persistence"
)

// osFd wraps *os.File for direct ioctl access.
type osFd struct{ f *os.File }

// Fd returns the file descriptor.
func (o *osFd) Fd() uintptr { return o.f.Fd() }

// Close closes the underlying file.
func (o *osFd) Close() error { return o.f.Close() }

// openOSFile opens a device file for reading.
func openOSFile(devicePath string) (*os.File, error) {
	return os.OpenFile(devicePath, os.O_RDONLY, 0)
}

// blkGetSize64 is implemented per-platform (queue_helpers_linux.go on Linux,
// queue_helpers_other.go elsewhere). It retrieves device size in bytes.

// formatDevice formats the device as FAT32.
func formatDevice(ctx context.Context, devicePath string, unsafeAllow bool) error {
	return format.FormatFAT32(ctx, devicePath, unsafeAllow)
}

// writeHistory persists a wipe job result to the history store.
func writeHistory(store *persistence.Store, job *Job) {
	if store == nil {
		return
	}
	rec := persistence.WipeRecord{
		DevicePath:         job.DevicePath,
		DeviceID:           job.DeviceID,
		IdentitySource:     job.IdentitySource,
		IdentityConfidence: job.IdentityConfidence,
		DeviceModel:        job.DeviceModel,
		DeviceSerial:       job.DeviceSerial,
		DeviceFirmware:     job.DeviceFirmware,
		DeviceWWN:          job.DeviceWWN,
		SizeBytes:          job.DeviceSizeBytes,
		Status:             string(job.Status),
		Verification:       job.Verified,
		Error:              job.ErrorMessage,
		BytesVerified:      job.BytesVerified,
	}
	if job.CreatedAt.IsZero() == false {
		rec.StartedAt = job.CreatedAt
	}
	if job.StartedAt != nil {
		rec.StartedAt = *job.StartedAt
	}
	if job.CompletedAt != nil {
		rec.FinishedAt = *job.CompletedAt
		rec.Duration = job.CompletedAt.Sub(rec.StartedAt).Round(0).String()
	}

	if err := store.Append(rec); err != nil {
		// Log but don't fail — history is best-effort
		// (import "log" at top would create circular dependency concern,
		// but we can use println for this fallback)
		println("WARNING: failed to persist wipe job", job.ID, ":", err.Error())
	}
}
