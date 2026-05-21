package format

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/usb-wiper/internal/device"
)

// FormatFAT32 partitions and formats a device as FAT32.
// CRITICAL: Re-runs safety check before proceeding.
func FormatFAT32(devicePath string, unsafeAllowAllUSB bool) error {
	// Re-validate safety
	if err := device.IsSafeToWipe(devicePath, unsafeAllowAllUSB); err != nil {
		return fmt.Errorf("safety check before format: %w", err)
	}

	// Step 1: Create MSDOS partition label
	if err := runCmd(30*time.Second, "parted", "-s", devicePath, "mklabel", "msdos"); err != nil {
		return fmt.Errorf("mklabel: %w", err)
	}

	// Step 2: Create primary FAT32 partition (1MiB to 100%)
	if err := runCmd(30*time.Second, "parted", "-s", devicePath, "mkpart", "primary", "fat32", "1MiB", "100%"); err != nil {
		return fmt.Errorf("mkpart: %w", err)
	}

	// Step 3: Wait for kernel to register partition
	time.Sleep(1 * time.Second)

	// Step 4: Format partition as FAT32
	partitionPath := devicePath + "1"
	if err := runCmd(30*time.Second, "mkfs.vfat", "-F", "32", partitionPath); err != nil {
		return fmt.Errorf("mkfs.vfat: %w", err)
	}

	return nil
}

func runCmd(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w (output: %s)", name, args, err, string(output))
	}
	return nil
}
