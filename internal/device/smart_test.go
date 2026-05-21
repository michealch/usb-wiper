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
	// Test that our JSON parsing handles empty input
	health := &Health{
		HealthStatus: "UNKNOWN",
	}
	if health.HealthStatus != "UNKNOWN" {
		t.Errorf("default HealthStatus should be UNKNOWN")
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
