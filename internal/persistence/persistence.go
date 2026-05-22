// Package persistence provides atomic JSON file storage for wipe history.
// Uses a simple append-to-file journal approach: records are appended as
// individual JSON lines, and the full history is read back on startup.
// Writes use rename-based atomicity to prevent corruption.
package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WipeRecord represents a single wipe operation in history.
type WipeRecord struct {
	DevicePath    string    `json:"devicePath"`
	DeviceModel   string    `json:"deviceModel"`
	DeviceSerial  string    `json:"deviceSerial"`
	SizeBytes     uint64    `json:"sizeBytes"`
	Status        string    `json:"status"` // "completed", "failed", "cancelled"
	Verification  string    `json:"verification,omitempty"` // "passed", "failed", or empty if not done/cancelled
	Error         string    `json:"error,omitempty"`
	BytesVerified uint64    `json:"bytesVerified"` // how many bytes were checked
	StartedAt     time.Time `json:"startedAt"`
	FinishedAt    time.Time `json:"finishedAt"`
	Duration      string    `json:"duration"`
}

// Store provides thread-safe access to wipe history persisted as JSON.
type Store struct {
	mu       sync.RWMutex
	dataDir  string
	records  []WipeRecord
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
	data, err := os.ReadFile(s.filePath())
	if err != nil {
		return err
	}

	// File may be empty
	if len(data) == 0 {
		return nil
	}

	// Parse JSON array
	var records []WipeRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("parse history: %w", err)
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

// UpdateByDevice finds a record by device path (matching the latest) and
// updates its fields. Returns the updated record.
func (s *Store) UpdateByDevice(devicePath string, fn func(*WipeRecord)) error {
	s.mu.Lock()
	found := false
	for i := len(s.records) - 1; i >= 0; i-- {
		if s.records[i].DevicePath == devicePath {
			fn(&s.records[i])
			found = true
			break
		}
	}

	if !found {
		s.mu.Unlock()
		return fmt.Errorf("no record found for %s", devicePath)
	}

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

// GetLatest returns the most recent record for a device, or nil.
func (s *Store) GetLatest(devicePath string) *WipeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.records) - 1; i >= 0; i-- {
		if s.records[i].DevicePath == devicePath {
			r := s.records[i]
			return &r
		}
	}
	return nil
}

// save atomically writes records to the history file using a temp file + rename.
// Caller must hold s.mu.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')

	// Use a unique temp file name to prevent concurrent writers from
	// corrupting each other (s.mu serializes writes, but CreateTemp is
	// the safe pattern).
	dir := s.dataDir
	tmp, err := os.CreateTemp(dir, "history-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp: %w", err)
	}

	// Sync the temp file before rename for crash safety
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("sync temp: %w", err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp: %w", err)
	}

	// Atomic rename
	dest := s.filePath()
	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}

	// Sync the directory to ensure the rename is durable
	if dirFd, err := os.Open(dir); err == nil {
		dirFd.Sync()
		dirFd.Close()
	}

	return nil
}
