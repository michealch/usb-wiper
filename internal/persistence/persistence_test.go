package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewEmptyStore(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	records := store.GetAll()
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestAppendAndGetAll(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	now := time.Now()
	r1 := WipeRecord{
		DevicePath:   "/dev/sdb",
		DeviceModel:  "Test USB",
		DeviceSerial: "ABC123",
		SizeBytes:    8_000_000_000,
		Status:       "completed",
		Verification: "passed",
		BytesVerified: 1_073_741_824,
		StartedAt:    now.Add(-1 * time.Hour),
		FinishedAt:   now,
		Duration:     "1h0m0s",
	}

	r2 := WipeRecord{
		DevicePath:  "/dev/sdc",
		Status:      "failed",
		Error:       "device busy",
		StartedAt:   now.Add(-30 * time.Minute),
		FinishedAt:  now.Add(-25 * time.Minute),
	}

	if err := store.Append(r1); err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if err := store.Append(r2); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	records := store.GetAll()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	// Newest first
	if records[0].DevicePath != "/dev/sdc" {
		t.Errorf("expected /dev/sdc first, got %s", records[0].DevicePath)
	}
	if records[1].DevicePath != "/dev/sdb" {
		t.Errorf("expected /dev/sdb second, got %s", records[1].DevicePath)
	}

	// Verify file exists
	if _, err := os.Stat(filepath.Join(dir, "history.json")); err != nil {
		t.Errorf("history.json not found: %v", err)
	}
}

func TestUpdateByDevice(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	store.Append(WipeRecord{
		DevicePath: "/dev/sdb",
		Status:     "running",
		StartedAt:  time.Now(),
	})

	err = store.UpdateByDevice("/dev/sdb", func(r *WipeRecord) {
		r.Status = "completed"
		r.Verification = "passed"
		r.BytesVerified = 1024
	})
	if err != nil {
		t.Fatalf("UpdateByDevice failed: %v", err)
	}

	records := store.GetAll()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", records[0].Status)
	}
	if records[0].Verification != "passed" {
		t.Errorf("expected verification 'passed', got '%s'", records[0].Verification)
	}
}

func TestGetLatest(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	store.Append(WipeRecord{DevicePath: "/dev/sdb", Status: "completed"})
	store.Append(WipeRecord{DevicePath: "/dev/sdb", Status: "failed"})

	latest := store.GetLatest("/dev/sdb")
	if latest == nil {
		t.Fatal("GetLatest returned nil")
	}
	if latest.Status != "failed" {
		t.Errorf("expected status 'failed', got '%s'", latest.Status)
	}

	// Unknown device
	if store.GetLatest("/dev/sdz") != nil {
		t.Error("expected nil for unknown device")
	}
}

func TestPersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()

	// First instance
	s1, _ := New(dir)
	s1.Append(WipeRecord{DevicePath: "/dev/sdb", Status: "completed"})

	// Second instance should load the same data
	s2, err := New(dir)
	if err != nil {
		t.Fatalf("second New failed: %v", err)
	}

	records := s2.GetAll()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].DevicePath != "/dev/sdb" {
		t.Errorf("expected /dev/sdb, got %s", records[0].DevicePath)
	}
}
