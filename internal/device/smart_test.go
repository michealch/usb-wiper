package device

import (
	"testing"
)

func TestParseCapacity(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
	}{
		{"8000000000 bytes", 8000000000},
		{"8,000,000,000 bytes [8.00 GB]", 8000000000},
		{"0", 0},
		{"12345 bytes", 12345},
	}

	for _, tt := range tests {
		result, err := parseCapacity(tt.input)
		if err != nil {
			t.Errorf("parseCapacity(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("parseCapacity(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestGetHealth_NonexistentDevice(t *testing.T) {
	health, err := GetHealth("/dev/nonexistent")
	if err != nil {
		t.Logf("expected behavior: %v", err)
	}
	if health == nil {
		t.Fatal("expected non-nil Health even on error")
	}
	if health.HealthStatus == "" {
		t.Error("HealthStatus should have a value")
	}
}

func TestSafeFloat(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected float64
	}{
		{float64(42.5), 42.5},
		{int(42), 42.0},
		{"99.9", 99.9},
		{nil, 0},
		{true, 0},
	}

	for _, tt := range tests {
		result := safeFloat(tt.input)
		if result != tt.expected {
			t.Errorf("safeFloat(%v) = %f, want %f", tt.input, result, tt.expected)
		}
	}
}

func TestSmartJSONParsing(t *testing.T) {
	raw := []byte(`{
		"model_name": "Example NVMe",
		"serial_number": "ABC123",
		"firmware_version": "1.0",
		"user_capacity": {"blocks": 1953525168, "bytes": 1000204886016},
		"smart_status": {"passed": true},
		"nvme_smart_health_information_log": {
			"temperature": 35,
			"available_spare": 99,
			"percentage_used": 7,
			"data_units_read": 123456,
			"data_units_written": 654321,
			"power_cycles": 42,
			"power_on_hours": 512,
			"media_errors": 3
		}
	}`)

	health := parseSmartJSON(raw)
	if health == nil {
		t.Fatal("expected health to parse")
	}
	if health.HealthStatus != "PASSED" {
		t.Errorf("HealthStatus = %q, want PASSED", health.HealthStatus)
	}
	if health.ModelName != "Example NVMe" || health.SerialNumber != "ABC123" {
		t.Errorf("identity = %q/%q, want Example NVMe/ABC123", health.ModelName, health.SerialNumber)
	}
	if health.CapacityBytes != 1000204886016 {
		t.Errorf("CapacityBytes = %d, want 1000204886016", health.CapacityBytes)
	}
	if health.AvailableSparePct != 99 || health.EnduranceUsedPct != 7 {
		t.Errorf("NVMe spare/endurance = %d/%d, want 99/7", health.AvailableSparePct, health.EnduranceUsedPct)
	}
	if health.ReadLBAs != 123456 || health.WriteLBAs != 654321 {
		t.Errorf("NVMe data units = %d/%d, want 123456/654321", health.ReadLBAs, health.WriteLBAs)
	}
	if health.PowerOnHours != 512 || health.PowerCycleCount != 42 {
		t.Errorf("power counters = %d/%d, want 512/42", health.PowerOnHours, health.PowerCycleCount)
	}
}

func TestSmartHealthScorePrefersNVMeLogOverBridgeIdentity(t *testing.T) {
	bridge := parseSmartJSON([]byte(`{"device":{"model":"USB NVMe bridge"}}`))
	nvme := parseSmartJSON([]byte(`{
		"model_name":"Example NVMe",
		"smart_status":{"passed":true},
		"nvme_smart_health_log":{"temperature":40}
	}`))

	if bridge == nil || nvme == nil {
		t.Fatal("expected both SMART payloads to parse")
	}
	if smartHealthScore(nvme) <= smartHealthScore(bridge) {
		t.Fatalf("expected NVMe health log score %d to beat bridge identity score %d", smartHealthScore(nvme), smartHealthScore(bridge))
	}
}

func TestHealthDefaults(t *testing.T) {
	h := &Health{}
	if h.HealthStatus != "" {
		t.Error("zero value Health should have empty HealthStatus")
	}
	if h.TemperatureC != 0 {
		t.Error("zero value Health should have TemperatureC = 0")
	}
}
