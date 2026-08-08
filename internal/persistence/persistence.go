// Package persistence provides atomic JSON file storage for wipe history.
// Uses a simple append-to-file journal approach: records are appended as
// individual JSON lines, and the full history is read back on startup.
// Writes use rename-based atomicity to prevent corruption.
package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/usb-wiper/internal/jsonfile"
)

// WipeRecord represents a single wipe operation in history.
type WipeRecord struct {
	DevicePath         string    `json:"devicePath"`
	DeviceID           string    `json:"deviceId,omitempty"`
	IdentitySource     string    `json:"identitySource,omitempty"`
	IdentityConfidence string    `json:"identityConfidence,omitempty"`
	DeviceModel        string    `json:"deviceModel"`
	DeviceSerial       string    `json:"deviceSerial"`
	DeviceFirmware     string    `json:"deviceFirmware,omitempty"`
	DeviceWWN          string    `json:"deviceWwn,omitempty"`
	SizeBytes          uint64    `json:"sizeBytes"`
	Status             string    `json:"status"`                 // "completed", "failed", "cancelled"
	Verification       string    `json:"verification,omitempty"` // "passed", "failed", or empty if not done/cancelled
	Error              string    `json:"error,omitempty"`
	BytesVerified      uint64    `json:"bytesVerified"` // how many bytes were checked
	StartedAt          time.Time `json:"startedAt"`
	FinishedAt         time.Time `json:"finishedAt"`
	Duration           string    `json:"duration"`
}

// Store provides thread-safe access to wipe history persisted as JSON.
type Store struct {
	mu      sync.RWMutex
	dataDir string
	records []WipeRecord
}

// New creates or loads a persistence store from the given data directory.
func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	s := &Store{dataDir: dataDir, records: make([]WipeRecord, 0)}

	// Load existing records on startup
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load history: %w", err)
	}

	return s, nil
}

// filePath returns the history file path.
func (s *Store) filePath() string {
	return filepath.Join(s.dataDir, "history.json")
}

// load reads all records from the history file.
func (s *Store) load() error {
	var records []WipeRecord
	if err := jsonfile.Read(s.filePath(), "parse history", &records); err != nil {
		return err
	}

	s.mu.Lock()
	s.records = records
	s.mu.Unlock()

	return nil
}

// Append adds a new wipe record and atomically persists to disk.
func (s *Store) Append(record WipeRecord) error {
	s.mu.Lock()
	s.records = append(s.records, record)
	// Save while holding the lock to prevent concurrent writes from
	// racing on the temp-file + rename.
	err := s.save()
	s.mu.Unlock()
	return err
}

// GetAll returns a copy of all wipe records (newest first).
func (s *Store) GetAll() []WipeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]WipeRecord, len(s.records))
	// Reverse: newest first
	for i, j := 0, len(s.records)-1; i < len(s.records); i, j = i+1, j-1 {
		result[i] = s.records[j]
	}
	return result
}

// GetLatestByDeviceID returns the most recent record for a trusted physical
// device identity. Empty or low-confidence IDs should not be passed here.
func (s *Store) GetLatestByDeviceID(deviceID string) *WipeRecord {
	if deviceID == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.records) - 1; i >= 0; i-- {
		if s.records[i].DeviceID == deviceID {
			r := s.records[i]
			return &r
		}
	}
	return nil
}

// GetByDeviceID returns records for a trusted physical device identity,
// newest first.
func (s *Store) GetByDeviceID(deviceID string) []WipeRecord {
	if deviceID == "" {
		return []WipeRecord{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []WipeRecord{}
	for i := len(s.records) - 1; i >= 0; i-- {
		if s.records[i].DeviceID == deviceID {
			result = append(result, s.records[i])
		}
	}
	return result
}

// save atomically writes records to the history file using a temp file + rename.
// Caller must hold s.mu.
func (s *Store) save() error {
	return jsonfile.Write(s.dataDir, s.filePath(), "history-*.json.tmp", s.records)
}
