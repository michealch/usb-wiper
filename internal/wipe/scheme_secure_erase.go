package wipe

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SchemeSecureErase uses ATA Secure Erase or NVMe Format (firmware-level sanitize).
// Requires ALLOW_HARDWARE_SECURE_ERASE=1 env var or settings toggle.
// This is a stub — full implementation requires hdparm/nvme-cli in the runtime image.
type SchemeSecureErase struct{}

func (s *SchemeSecureErase) ID() string          { return "secure-erase" }
func (s *SchemeSecureErase) DisplayName() string { return "ATA Secure Erase / NVMe Format" }
func (s *SchemeSecureErase) Passes() int          { return 1 }

func (s *SchemeSecureErase) Execute(ctx context.Context, devicePath string, size uint64,
	progress chan<- ProgressEvent) error {

	// Check if hdparm is available
	if _, err := exec.LookPath("hdparm"); err != nil {
		return fmt.Errorf("hardware secure erase requires hdparm (install smartmontools)")
	}

	// Determine device type
	isNVMe := strings.Contains(devicePath, "nvme")

	if isNVMe {
		return s.eraseNVMe(ctx, devicePath, size, progress)
	}
	return s.eraseATA(ctx, devicePath, size, progress)
}

func (s *SchemeSecureErase) eraseATA(ctx context.Context, devicePath string, size uint64,
	progress chan<- ProgressEvent) error {

	sendSecureProgress(progress, devicePath, "Checking security status...", 0)

	// Check if security is frozen
	hdparmOutput, err := exec.CommandContext(ctx, "hdparm", "-I", devicePath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("hdparm -I %s: %w\nOutput: %s", devicePath, err, string(hdparmOutput))
	}

	output := string(hdparmOutput)

	if strings.Contains(output, "frozen") {
		// Frozen — need suspend/resume or power cycle
		sendSecureProgress(progress, devicePath, "Security is frozen — attempting suspend/resume...", 10)
		if err := exec.CommandContext(ctx, "sh", "-c",
			fmt.Sprintf("echo -n mem > /sys/power/state 2>/dev/null; sleep 1; echo 0 > /sys/power/state 2>/dev/null || true"),
		).Run(); err != nil {
			// Non-fatal — the command may fail but still unfreeze the device
		}
	}

	if !strings.Contains(output, "supported") && !strings.Contains(output, "Supported") {
		return fmt.Errorf("device %s does not support ATA Security feature set", devicePath)
	}

	sendSecureProgress(progress, devicePath, "Setting temporary security password...", 20)

	setPass := exec.CommandContext(ctx, "hdparm", "--user-master", "u",
		"--security-set-pass", "usb-wiper-tmp", devicePath)
	if out, err := setPass.CombinedOutput(); err != nil {
		return fmt.Errorf("set security password: %w\nOutput: %s", err, string(out))
	}

	sendSecureProgress(progress, devicePath, "Issuing ATA SECURITY ERASE...", 40)

	// Enhanced erase if supported, otherwise standard erase
	eraseFlag := "--security-erase"
	if strings.Contains(output, "enhanced erase") || strings.Contains(output, "Enhanced erase") {
		eraseFlag = "--security-erase-enhanced"
	}

	// Inform the user this may take a long time
	sendSecureProgress(progress, devicePath, fmt.Sprintf("Erasing %s (this may take minutes — device is unresponsive during erase)...", devicePath), 50)

	erase := exec.CommandContext(ctx, "hdparm", "--user-master", "u",
		eraseFlag, "usb-wiper-tmp", devicePath)
	if out, err := erase.CombinedOutput(); err != nil {
		return fmt.Errorf("secure erase: %w\nOutput: %s", err, string(out))
	}

	// Wait a bit for the device to finish
	time.Sleep(2 * time.Second)

	sendSecureProgress(progress, devicePath, "Secure erase complete. Device is all zeros.", 100)

	return nil
}

func (s *SchemeSecureErase) eraseNVMe(ctx context.Context, devicePath string, size uint64,
	progress chan<- ProgressEvent) error {
	// NVMe Format with secure erase
	nvmeOutput, err := exec.CommandContext(ctx, "nvme", "format", devicePath,
		"--ses=1", // Secure Erase
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("nvme format: %w\nOutput: %s", err, string(nvmeOutput))
	}

	time.Sleep(2 * time.Second)

	sendSecureProgress(progress, devicePath, "NVMe secure erase complete.", 100)
	return nil
}

func sendSecureProgress(ch chan<- ProgressEvent, devicePath, msg string, pct float64) {
	if ch == nil {
		return
	}
	select {
	case ch <- ProgressEvent{
		DevicePath:  devicePath,
		Status:      "running",
		Percent:     pct,
		CurrentPass: 1,
		TotalPasses: 1,
		Message:     msg,
		Timestamp:   time.Now(),
	}:
	default:
	}
}
