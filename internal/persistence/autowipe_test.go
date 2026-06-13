package persistence

import (
	"testing"
	"time"
)

func TestAutoWipeStorePersistsAndOrdersRecords(t *testing.T) {
	dir := t.TempDir()
	store, err := NewAutoWipeStore(dir)
	if err != nil {
		t.Fatalf("NewAutoWipeStore: %v", err)
	}

	firstSeen := time.Now().Add(-2 * time.Hour).Round(0)
	older := AutoWipeRecord{
		DeviceID:       "disk-old",
		Serial:         "OLD123",
		Model:          "Older Disk",
		FirstSeenAt:    firstSeen,
		LastSeenAt:     firstSeen,
		LastDevicePath: "/dev/sdb",
		LastAction:     "observed_on_enable",
	}
	newer := AutoWipeRecord{
		DeviceID:       "disk-new",
		Serial:         "NEW123",
		Model:          "Newer Disk",
		LastSeenAt:     firstSeen.Add(time.Hour),
		LastDevicePath: "/dev/sdc",
		LastAction:     "queued",
		LastJobID:      "job-1",
	}

	if err := store.Upsert(older); err != nil {
		t.Fatalf("upsert older: %v", err)
	}
	if err := store.Upsert(newer); err != nil {
		t.Fatalf("upsert newer: %v", err)
	}

	reloaded, err := NewAutoWipeStore(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	records := reloaded.List()
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].DeviceID != "disk-new" {
		t.Fatalf("records not newest first: %#v", records)
	}
	if !reloaded.Has("disk-old") {
		t.Fatal("expected store to contain disk-old")
	}
}

func TestAutoWipeStoreUpsertPreservesFirstSeenAndClear(t *testing.T) {
	store, err := NewAutoWipeStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAutoWipeStore: %v", err)
	}

	firstSeen := time.Now().Add(-time.Hour).Round(0)
	if err := store.Upsert(AutoWipeRecord{
		DeviceID:    "disk-1",
		Serial:      "SERIAL",
		FirstSeenAt: firstSeen,
		LastSeenAt:  firstSeen,
	}); err != nil {
		t.Fatalf("initial upsert: %v", err)
	}

	later := firstSeen.Add(30 * time.Minute)
	if err := store.Upsert(AutoWipeRecord{
		DeviceID:   "disk-1",
		LastSeenAt: later,
		LastAction: "queued",
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	records := store.List()
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if !records[0].FirstSeenAt.Equal(firstSeen) {
		t.Fatalf("first seen changed: got %s want %s", records[0].FirstSeenAt, firstSeen)
	}
	if records[0].Serial != "SERIAL" {
		t.Fatalf("serial not preserved: %q", records[0].Serial)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if len(store.List()) != 0 {
		t.Fatal("expected clear to remove records")
	}
}
