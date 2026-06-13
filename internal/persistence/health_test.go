package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHealthStoreAppendAndGetByDeviceID(t *testing.T) {
	dir := t.TempDir()
	store, err := NewHealthStore(dir)
	if err != nil {
		t.Fatalf("NewHealthStore failed: %v", err)
	}

	now := time.Now()
	if err := store.Append(HealthRecord{
		DeviceID:     "dev_a",
		DevicePath:   "/dev/sdb",
		CapturedAt:   now.Add(-1 * time.Hour),
		HealthStatus: "PASSED",
		TemperatureC: 31,
		PowerOnHours: 100,
	}); err != nil {
		t.Fatalf("Append first failed: %v", err)
	}
	if err := store.Append(HealthRecord{
		DeviceID:     "dev_b",
		DevicePath:   "/dev/sdb",
		CapturedAt:   now.Add(-30 * time.Minute),
		HealthStatus: "FAILED",
		TemperatureC: 70,
		PowerOnHours: 200,
	}); err != nil {
		t.Fatalf("Append second failed: %v", err)
	}
	if err := store.Append(HealthRecord{
		DeviceID:     "dev_a",
		DevicePath:   "/dev/sdc",
		CapturedAt:   now,
		HealthStatus: "PASSED",
		TemperatureC: 33,
		PowerOnHours: 101,
	}); err != nil {
		t.Fatalf("Append third failed: %v", err)
	}

	records := store.GetByDeviceID("dev_a")
	if len(records) != 2 {
		t.Fatalf("expected 2 records for dev_a, got %d", len(records))
	}
	if records[0].DevicePath != "/dev/sdc" || records[1].DevicePath != "/dev/sdb" {
		t.Fatalf("records not returned newest first: %#v", records)
	}

	latest := store.GetLatestByDeviceID("dev_b")
	if latest == nil || latest.HealthStatus != "FAILED" {
		t.Fatalf("expected latest failed record for dev_b, got %#v", latest)
	}

	if _, err := os.Stat(filepath.Join(dir, "health-history.json")); err != nil {
		t.Fatalf("health-history.json not found: %v", err)
	}
}

func TestHealthStorePersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	first, err := NewHealthStore(dir)
	if err != nil {
		t.Fatalf("NewHealthStore first failed: %v", err)
	}
	if err := first.Append(HealthRecord{DeviceID: "dev_a", HealthStatus: "PASSED", CapturedAt: time.Now()}); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	second, err := NewHealthStore(dir)
	if err != nil {
		t.Fatalf("NewHealthStore second failed: %v", err)
	}
	records := second.GetByDeviceID("dev_a")
	if len(records) != 1 {
		t.Fatalf("expected 1 persisted record, got %d", len(records))
	}
}
