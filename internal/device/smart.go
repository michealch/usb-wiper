package device

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Health contains SMART/health information for a device.
type Health struct {
	HealthStatus        string                 `json:"healthStatus"`
	DeviceType          string                 `json:"deviceType"`
	PowerOnHours        uint64                 `json:"powerOnHours"`
	PowerCycleCount     uint64                 `json:"powerCycleCount"`
	TemperatureC        int                    `json:"temperatureC"`
	ReadLBAs            uint64                 `json:"readLBAs"`
	WriteLBAs           uint64                 `json:"writeLBAs"`
	AvailableSparePct   int                    `json:"availableSparePct"`
	EnduranceUsedPct    int                    `json:"enduranceUsedPct"`
	ReallocatedSectors  uint64                 `json:"reallocatedSectors"`
	PendingSectors      uint64                 `json:"pendingSectors"`
	UncorrectableErrors uint64                 `json:"uncorrectableErrors"`
	ModelName           string                 `json:"modelName"`
	SerialNumber        string                 `json:"serialNumber"`
	FirmwareVersion     string                 `json:"firmwareVersion"`
	CapacityBytes       uint64                 `json:"capacityBytes"`
	Raw                 map[string]interface{} `json:"raw"`
}

// SmartIdentity is the subset of smartctl identity data used to identify the
// physical disk behind a USB bridge.
type SmartIdentity struct {
	Model         string
	Serial        string
	Firmware      string
	WWN           string
	CapacityBytes uint64
}

// smartDeviceTypes is the ordered list of -d flags to try with smartctl.
// These cover common USB bridge chips for NVMe and SATA devices.
var smartDeviceTypes = []string{
	"",           // auto-detect (try first)
	"sat,12",     // SAT passthrough (ASMedia, common USB-SATA/USB-NVMe bridges)
	"sat,16",     // SAT passthrough 16-byte CDB
	"sntasmedia", // ASMedia NVMe bridges
	"sntjmicron", // JMicron NVMe bridges
	"sntrealtek", // Realtek NVMe bridges
}

// GetSmartIdentity tries to read physical disk identity via smartctl.
// Returns zero values on failure (caller should fall back to sysfs).
// Uses a short timeout to avoid blocking device enumeration.
func GetSmartIdentity(devicePath string) SmartIdentity {
	bestScore := -1
	best := SmartIdentity{}

	for _, dt := range smartDeviceTypes {
		output, _, err := runSmartctlJSON("-i", devicePath, dt, 3*time.Second)
		if err != nil && len(output) == 0 {
			continue
		}

		var raw map[string]interface{}
		if json.Unmarshal(output, &raw) != nil {
			continue
		}

		attempt := parseSmartIdentity(raw)

		score := smartIdentityScore(raw, attempt)
		if score > bestScore {
			bestScore = score
			best = attempt
		}

		// Full disk identity is enough. Bridge-only device metadata is not:
		// a later -d snt* probe may expose the actual NVMe behind the bridge.
		if score >= 30 {
			return attempt
		}
	}

	if bestScore > 0 {
		return best
	}
	return SmartIdentity{}
}

// GetHealth retrieves SMART health information for a device.
// It tries multiple smartctl -d flags to handle USB bridges that
// don't support auto-detection (common with NVMe enclosures).
func GetHealth(devicePath string) (*Health, error) {
	var lastErr error
	var lastStderr string
	var best *Health
	bestScore := -1

	for _, dt := range smartDeviceTypes {
		output, stderr, err := runSmartctlJSON("-a", devicePath, dt, 5*time.Second)
		lastErr = err
		lastStderr = strings.TrimSpace(string(stderr))

		if health := parseSmartJSON(output); health != nil {
			health.DeviceType = smartDeviceTypeLabel(dt)
			if health.Raw != nil {
				health.Raw["usb_wiper_smartctl_type"] = health.DeviceType
			}
			score := smartHealthScore(health)
			if score > bestScore {
				best = health
				bestScore = score
			}
			if score >= 80 {
				return health, nil
			}
		}
	}

	if best != nil {
		return best, nil
	}

	errText := fmt.Sprintf("smartctl error: %v", lastErr)
	if lastStderr != "" {
		errText += ": " + lastStderr
	}
	return &Health{
		HealthStatus: "UNKNOWN",
		Raw:          map[string]interface{}{"error": errText},
	}, nil
}

func runSmartctlJSON(mode, devicePath, deviceType string, timeout time.Duration) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{mode, "-j"}
	if deviceType != "" {
		args = append(args, "-d", deviceType)
	}
	args = append(args, devicePath)

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "smartctl", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		log.Printf("smartctl %s %s timed out for %s", mode, smartDeviceTypeLabel(deviceType), devicePath)
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

// parseSmartJSON parses smartctl -j output into a Health struct.
// Returns nil if the JSON is unparseable or empty.
func parseSmartJSON(output []byte) *Health {
	var raw map[string]interface{}
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil
	}

	// Require at minimum some device model info to treat as valid
	if !hasAnyField(raw, "model_name", "model_family", "device") {
		return nil
	}

	h := &Health{
		HealthStatus: "UNKNOWN",
		Raw:          raw,
	}

	// ---- Health status (common to ATA, NVMe, SCSI) ----
	if hs, ok := raw["smart_status"].(map[string]interface{}); ok {
		if passed, ok := hs["passed"].(bool); ok {
			if passed {
				h.HealthStatus = "PASSED"
			} else {
				h.HealthStatus = "FAILED"
			}
		}
	}

	// ---- Model, serial, firmware (common keys) ----
	if modelName, ok := raw["model_name"].(string); ok {
		h.ModelName = modelName
	}
	if serial, ok := raw["serial_number"].(string); ok {
		h.SerialNumber = serial
	}
	if fw, ok := raw["firmware_version"].(string); ok {
		h.FirmwareVersion = fw
	}
	if h.ModelName == "" {
		if dev, ok := raw["device"].(map[string]interface{}); ok {
			if m, ok := dev["model"].(string); ok {
				h.ModelName = strings.TrimSpace(m)
			}
		}
	}

	// ---- Capacity ----
	if capBytes, ok := parseCapacityValue(raw["user_capacity"]); ok {
		h.CapacityBytes = capBytes
	}

	// ---- NVMe SMART/Health Information Log ----
	// smartctl 7.x+ uses "nvme_smart_health_information_log" (old name)
	// smartctl 7.5+ also uses "nvme_smart_health_log" (newer shorthand)
	// Both may be present; the log data is identical.
	nvmeLog := extractNVMeLog(raw)
	if nvmeLog != nil {
		parseNVMeHealthLog(h, nvmeLog)
	}

	// ---- Temperature (top-level, set by smartctl for all device types) ----
	// NVMe: smartctl copies temperature => current into top-level "temperature"
	if temp, ok := raw["temperature"].(map[string]interface{}); ok {
		if current := safeUint(temp["current"]); current > 0 {
			h.TemperatureC = int(current)
		}
	}

	// ---- Power on hours (top-level, set by smartctl for all device types) ----
	if poh, ok := raw["power_on_time"].(map[string]interface{}); ok {
		if hours := safeUint(poh["hours"]); hours > 0 {
			h.PowerOnHours = hours
		}
	}

	// ---- Power cycle count (top-level, set by smartctl for all device types) ----
	// NVMe: smartctl copies nvme*.power_cycles => top-level power_cycle_count
	if pcc := safeUint(raw["power_cycle_count"]); pcc > 0 {
		h.PowerCycleCount = pcc
	}

	// ---- ATA/SATA attributes ----
	if ata, ok := raw["ata_smart_attributes"].(map[string]interface{}); ok {
		if table, ok := ata["table"].([]interface{}); ok {
			for _, entry := range table {
				e, ok := entry.(map[string]interface{})
				if !ok {
					continue
				}
				id := int(safeFloat(e["id"]))
				var rawValue uint64
				if rawSub, ok := e["raw"].(map[string]interface{}); ok {
					rawValue = uint64(safeFloat(rawSub["value"]))
				}
				switch id {
				case 1: // Read error rate
					h.ReadLBAs = rawValue
				case 5: // Reallocated sectors
					h.ReallocatedSectors = rawValue
				case 187: // Reported uncorrectable
					h.UncorrectableErrors = rawValue
				case 197: // Current pending sector
					h.PendingSectors = rawValue
				}
			}
		}
	}

	return h
}

// extractNVMeLog locates the NVMe SMART/Health log in the raw JSON.
// smartctl versions differ on the key name.
func extractNVMeLog(raw map[string]interface{}) map[string]interface{} {
	// smartctl 7.5+ consolidated to "nvme_smart_health_log"
	if log, ok := raw["nvme_smart_health_log"].(map[string]interface{}); ok {
		return log
	}
	// Older smartctl 7.x used "nvme_smart_health_information_log"
	if log, ok := raw["nvme_smart_health_information_log"].(map[string]interface{}); ok {
		return log
	}
	return nil
}

// parseNVMeHealthLog extracts NVMe-specific health fields from the NVMe
// SMART/Health Information Log (0x02).
func parseNVMeHealthLog(h *Health, nvme map[string]interface{}) {
	if h.DeviceType == "" {
		h.DeviceType = "nvme"
	}

	// Data units read (each unit = 512 * 1000 bytes for NVMe 1.0)
	// Mapped to ReadLBAs for reuse in the Health struct
	if unitsRead := safeUint(nvme["data_units_read"]); unitsRead > 0 {
		h.ReadLBAs = unitsRead
	}

	// Data units written
	if unitsWritten := safeUint(nvme["data_units_written"]); unitsWritten > 0 {
		h.WriteLBAs = unitsWritten
	}

	// Media errors (analogous to ATA uncorrectable errors)
	h.UncorrectableErrors = safeUint(nvme["media_errors"])

	if spare := safeUint(nvme["available_spare"]); spare > 0 {
		h.AvailableSparePct = int(spare)
	}
	if used := safeUint(nvme["percentage_used"]); used > 0 {
		h.EnduranceUsedPct = int(used)
	}
	if temp := safeUint(nvme["temperature"]); temp > 0 && h.TemperatureC == 0 {
		h.TemperatureC = int(temp)
	}
	if cycles := safeUint(nvme["power_cycles"]); cycles > 0 && h.PowerCycleCount == 0 {
		h.PowerCycleCount = cycles
	}
	if hours := safeUint(nvme["power_on_hours"]); hours > 0 && h.PowerOnHours == 0 {
		h.PowerOnHours = hours
	}
}

func parseSmartIdentity(raw map[string]interface{}) SmartIdentity {
	id := SmartIdentity{}
	if m, ok := raw["model_name"].(string); ok {
		id.Model = strings.TrimSpace(m)
	}
	if id.Model == "" {
		if dev, ok := raw["device"].(map[string]interface{}); ok {
			if m, ok := dev["model"].(string); ok {
				id.Model = strings.TrimSpace(m)
			}
		}
	}
	if s, ok := raw["serial_number"].(string); ok {
		id.Serial = strings.TrimSpace(s)
	}
	if fw, ok := raw["firmware_version"].(string); ok {
		id.Firmware = strings.TrimSpace(fw)
	}
	if capBytes, ok := parseCapacityValue(raw["user_capacity"]); ok {
		id.CapacityBytes = capBytes
	}
	id.WWN = extractWWN(raw)
	return id
}

func extractWWN(raw map[string]interface{}) string {
	if value, ok := raw["wwn"].(map[string]interface{}); ok {
		parts := []string{}
		for _, key := range []string{"naa", "oui", "id"} {
			if s, ok := value[key].(string); ok && strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
				continue
			}
			if n := safeUint(value[key]); n > 0 {
				parts = append(parts, strconv.FormatUint(n, 16))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "-")
		}
	}
	for _, key := range []string{"world_wide_name", "wwn"} {
		if s, ok := raw[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func smartIdentityScore(raw map[string]interface{}, id SmartIdentity) int {
	score := 0
	if id.Model != "" {
		score += 10
	}
	if id.Serial != "" {
		score += 10
	}
	if id.WWN != "" {
		score += 30
	}
	if id.CapacityBytes > 0 {
		score += 5
	}
	if hasAnyField(raw, "model_name", "serial_number", "firmware_version") {
		score += 10
	}
	if extractNVMeLog(raw) != nil {
		score += 20
	}
	return score
}

func smartHealthScore(h *Health) int {
	if h == nil || h.Raw == nil {
		return 0
	}
	score := 0
	if h.ModelName != "" || h.SerialNumber != "" {
		score += 5
	}
	if _, ok := h.Raw["smart_status"].(map[string]interface{}); ok {
		score += 30
	}
	if extractNVMeLog(h.Raw) != nil {
		score += 80
	}
	if ata, ok := h.Raw["ata_smart_attributes"].(map[string]interface{}); ok {
		if table, ok := ata["table"].([]interface{}); ok && len(table) > 0 {
			score += 80
		}
	}
	if h.TemperatureC > 0 {
		score += 10
	}
	if h.PowerOnHours > 0 || h.PowerCycleCount > 0 {
		score += 10
	}
	return score
}

func smartDeviceTypeLabel(deviceType string) string {
	if deviceType == "" {
		return "auto"
	}
	return deviceType
}

// hasAnyField returns true if the map contains at least one of the given keys.
func hasAnyField(m map[string]interface{}, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func safeFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(strings.ReplaceAll(val, ",", ""), 64)
		return f
	default:
		return 0
	}
}

func safeUint(v interface{}) uint64 {
	f := safeFloat(v)
	if f <= 0 {
		return 0
	}
	return uint64(f)
}

func parseCapacityValue(v interface{}) (uint64, bool) {
	switch val := v.(type) {
	case string:
		capBytes, err := parseCapacity(val)
		return capBytes, err == nil
	case map[string]interface{}:
		if capBytes := safeUint(val["bytes"]); capBytes > 0 {
			return capBytes, true
		}
		if blocks := safeUint(val["blocks"]); blocks > 0 {
			return blocks * 512, true
		}
	}
	return 0, false
}

func parseCapacity(s string) (uint64, error) {
	// Handle "8,000,000,000 bytes [8.00 GB]" and "8000000000 bytes".
	lower := strings.ToLower(s)
	if idx := strings.Index(lower, "bytes"); idx >= 0 {
		s = s[:idx]
	}
	clean := ""
	for _, c := range s {
		if c >= '0' && c <= '9' {
			clean += string(c)
		}
	}
	if clean == "" {
		return 0, fmt.Errorf("no byte count in %q", s)
	}
	return strconv.ParseUint(clean, 10, 64)
}
