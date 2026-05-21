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

// GetHealth retrieves SMART health information for a device.
func GetHealth(devicePath string) (*Health, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "smartctl", "-a", "-j", devicePath)
	output, err := cmd.Output()
	if err != nil {
		// smartctl exits non-zero for some USB sticks; try to parse anyway
		if output == nil || len(output) == 0 {
			return &Health{
				HealthStatus: "UNKNOWN",
				Raw:          map[string]interface{}{"error": err.Error()},
			}, nil
		}
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(output, &raw); err != nil {
		return &Health{
			HealthStatus: "UNKNOWN",
			Raw:          map[string]interface{}{"error": fmt.Sprintf("json parse: %v", err)},
		}, nil
	}

	h := &Health{
		HealthStatus: "UNKNOWN",
		Raw:          raw,
	}

	// Extract health status
	if hs, ok := raw["smart_status"].(map[string]interface{}); ok {
		if passed, ok := hs["passed"].(bool); ok {
			if passed {
				h.HealthStatus = "PASSED"
			} else {
				h.HealthStatus = "FAILED"
			}
		}
	}

	// Extract power on hours
	if poh, ok := raw["power_on_time"].(map[string]interface{}); ok {
		if hours, ok := poh["hours"].(float64); ok {
			h.PowerOnHours = uint64(hours)
		}
	}

	// Extract power cycle count
	if pcc, ok := raw["power_cycle_count"].(float64); ok {
		h.PowerCycleCount = uint64(pcc)
	}

	// Extract temperature
	if temp, ok := raw["temperature"].(map[string]interface{}); ok {
		if current, ok := temp["current"].(float64); ok {
			h.TemperatureC = int(current)
		}
	}

	// Extract model info
	if modelName, ok := raw["model_name"].(string); ok {
		h.ModelName = modelName
	}
	if serial, ok := raw["serial_number"].(string); ok {
		h.SerialNumber = serial
	}
	if fw, ok := raw["firmware_version"].(string); ok {
		h.FirmwareVersion = fw
	}

	// Extract capacity
	if userCap, ok := raw["user_capacity"].(string); ok {
		// Parse e.g. "8,000,000,000 bytes"
		if capBytes, err := parseCapacity(userCap); err == nil {
			h.CapacityBytes = capBytes
		}
	}

	// Extract ATA/SATA attributes if present
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

	return h, nil
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
