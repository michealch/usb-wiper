package device

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// Health contains SMART/health information for a device.
type Health struct {
	HealthStatus        string                 `json:"healthStatus"`
	PowerOnHours        uint64                 `json:"powerOnHours"`
	PowerCycleCount     uint64                 `json:"powerCycleCount"`
	TemperatureC        int                    `json:"temperatureC"`
	ReadLBAs            uint64                 `json:"readLBAs"`
	WriteLBAs           uint64                 `json:"writeLBAs"`
	ReallocatedSectors  uint64                 `json:"reallocatedSectors"`
	PendingSectors      uint64                 `json:"pendingSectors"`
	UncorrectableErrors uint64                 `json:"uncorrectableErrors"`
	ModelName           string                 `json:"modelName"`
	SerialNumber        string                 `json:"serialNumber"`
	FirmwareVersion     string                 `json:"firmwareVersion"`
	CapacityBytes       uint64                 `json:"capacityBytes"`
	Raw                 map[string]interface{} `json:"raw"`
}

// deviceTypes to try with smartctl -d when auto-detect fails.
// These cover common USB bridge chips for NVMe and SATA devices.
var deviceTypes = []string{
	"",           // auto-detect (try first)
	"sat,12",     // SAT passthrough (ASMedia, common USB-SATA/USB-NVMe bridges)
	"sat,16",     // SAT passthrough 16-byte CDB
	"sntasmedia", // ASMedia NVMe bridges
	"sntjmicron", // JMicron NVMe bridges
	"sntrealtek", // Realtek NVMe bridges
}

// GetHealth retrieves SMART health information for a device.
// It tries multiple smartctl -d flags to handle USB bridges that
// don't support auto-detection (common with NVMe enclosures).
func GetHealth(devicePath string) (*Health, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var lastOutput []byte
	var lastErr error

	for _, dt := range deviceTypes {
		args := []string{"-a", "-j"}
		if dt != "" {
			args = append(args, "-d", dt)
		}
		args = append(args, devicePath)

		cmd := exec.CommandContext(ctx, "smartctl", args...)
		output, err := cmd.Output()
		if err != nil {
			// smartctl exits non-zero for many reasons; save output and try next type
			lastOutput = output
			lastErr = err
			if len(output) > 0 {
				// If we got JSON output despite the error, try to parse it
				if parseResult := parseSmartJSON(output); parseResult != nil {
					return parseResult, nil
				}
			}
			continue
		}

		if len(output) == 0 {
			continue
		}

		if health := parseSmartJSON(output); health != nil {
			return health, nil
		}
	}

	// All attempts failed
	if lastOutput != nil && len(lastOutput) > 0 {
		return &Health{
			HealthStatus: "UNKNOWN",
			Raw:          map[string]interface{}{"error": fmt.Sprintf("all device types failed: %v", lastErr)},
		}, nil
	}

	return &Health{
		HealthStatus: "UNKNOWN",
		Raw:          map[string]interface{}{"error": fmt.Sprintf("smartctl error: %v", lastErr)},
	}, nil
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

	// ---- Capacity ----
	if userCap, ok := raw["user_capacity"].(string); ok {
		if capBytes, err := parseCapacity(userCap); err == nil {
			h.CapacityBytes = capBytes
		}
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
		if current, ok := temp["current"].(float64); ok {
			h.TemperatureC = int(current)
		}
	}

	// ---- Power on hours (top-level, set by smartctl for all device types) ----
	if poh, ok := raw["power_on_time"].(map[string]interface{}); ok {
		if hours, ok := poh["hours"].(float64); ok {
			h.PowerOnHours = uint64(hours)
		}
	}

	// ---- Power cycle count (top-level, set by smartctl for all device types) ----
	// NVMe: smartctl copies nvme*.power_cycles => top-level power_cycle_count
	if pcc, ok := raw["power_cycle_count"].(float64); ok {
		h.PowerCycleCount = uint64(pcc)
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
	// Data units read (each unit = 512 * 1000 bytes for NVMe 1.0)
	// Mapped to ReadLBAs for reuse in the Health struct
	if unitsRead, ok := nvme["data_units_read"].(float64); ok {
		h.ReadLBAs = uint64(unitsRead)
	}

	// Data units written
	if unitsWritten, ok := nvme["data_units_written"].(float64); ok {
		h.WriteLBAs = uint64(unitsWritten)
	}

	// Media errors (analogous to ATA uncorrectable errors)
	if mediaErrs, ok := nvme["media_errors"].(float64); ok {
		h.UncorrectableErrors = uint64(mediaErrs)
	}
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
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

func parseCapacity(s string) (uint64, error) {
	// Handle "8,000,000,000 bytes [8.00 GB]" format
	var bytes uint64
	var unit string
	_, err := fmt.Sscanf(s, "%d %s", &bytes, &unit)
	if err != nil {
		// Try basic number parsing
		clean := ""
		for _, c := range s {
			if c >= '0' && c <= '9' {
				clean += string(c)
			}
		}
		return strconv.ParseUint(clean, 10, 64)
	}
	return bytes, nil
}
