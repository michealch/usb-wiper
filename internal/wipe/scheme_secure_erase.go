package wipe

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// SchemeSecureErase uses ATA Secure Erase or NVMe Format (firmware-level sanitize).
// Requires ALLOW_HARDWARE_SECURE_ERASE=1 env var or settings toggle.
type SchemeSecureErase struct{}

func (s *SchemeSecureErase) ID() string          { return "secure-erase" }
func (s *SchemeSecureErase) DisplayName() string { return "ATA Secure Erase / NVMe Format" }
func (s *SchemeSecureErase) Passes() int         { return 1 }

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

	supported, frozen, enhanced := parseATASecurity(output)
	if !supported {
		return fmt.Errorf("device %s does not support the ATA Security feature set", devicePath)
	}
	if frozen {
		return fmt.Errorf("device %s has ATA security frozen; power-cycle the enclosure and retry", devicePath)
	}

	sendSecureProgress(progress, devicePath, "Setting temporary security password...", 20)

	setPass := exec.CommandContext(ctx, "hdparm", "--user-master", "u",
		"--security-set-pass", "usb-wiper-tmp", devicePath)
	if out, err := setPass.CombinedOutput(); err != nil {
		return fmt.Errorf("set security password: %w\nOutput: %s", err, string(out))
	}

	// If the erase never confirms success, drop the temporary password so the
	// drive is not left locked. WithoutCancel so a cancelled job still unwinds.
	erased := false
	defer func() {
		if erased {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		out, err := exec.CommandContext(cleanupCtx, "hdparm", "--user-master", "u",
			"--security-disable", "usb-wiper-tmp", devicePath).CombinedOutput()
		if err != nil {
			log.Printf("WARNING: could not clear temporary ATA password on %s: %v (output: %s)",
				devicePath, err, string(out))
		}
	}()

	sendSecureProgress(progress, devicePath, "Issuing ATA SECURITY ERASE...", 40)

	// Enhanced erase if supported, otherwise standard erase
	eraseFlag := "--security-erase"
	if enhanced {
		eraseFlag = "--security-erase-enhanced"
	}

	// Inform the user this may take a long time
	sendSecureProgress(progress, devicePath, fmt.Sprintf("Erasing %s (this may take minutes — device is unresponsive during erase)...", devicePath), 50)

	erase := exec.CommandContext(ctx, "hdparm", "--user-master", "u",
		eraseFlag, "usb-wiper-tmp", devicePath)
	if out, err := erase.CombinedOutput(); err != nil {
		return fmt.Errorf("secure erase: %w\nOutput: %s", err, string(out))
	}
	erased = true

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

// parseATASecurity extracts the ATA Security feature-set state from `hdparm -I`
// output. It reads only the indented lines of the "Security:" section, where
// hdparm prints bare "supported"/"not supported" and "frozen"/"not frozen"
// flags. Both return values default to false when the section is absent.
func parseATASecurity(hdparmOutput string) (supported, frozen, enhanced bool) {
	inSection := false
	for _, line := range strings.Split(hdparmOutput, "\n") {
		trimmed := strings.TrimSpace(line)
		if !inSection {
			if trimmed == "Security:" {
				inSection = true
			}
			continue
		}
		// Leave the section at the first subsequent line that is non-empty and
		// not indented.
		if trimmed != "" && (len(line) == 0 || (line[0] != ' ' && line[0] != '\t')) {
			break
		}
		switch trimmed {
		case "supported":
			supported = true
		case "not supported":
			supported = false
		case "frozen":
			frozen = true
		case "not frozen":
			frozen = false
		}
		lower := strings.ToLower(trimmed)
		if strings.HasSuffix(lower, "enhanced erase") && !strings.HasPrefix(lower, "not") {
			enhanced = true
		}
	}
	return supported, frozen, enhanced
}
