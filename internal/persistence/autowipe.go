package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AutoWipeRecord tracks a physical device identity already seen by auto-wipe.
type AutoWipeRecord struct {
	DeviceID           string    `json:"deviceId"`
	Serial             string    `json:"serial,omitempty"`
	Model              string    `json:"model,omitempty"`
	IdentitySource     string    `json:"identitySource,omitempty"`
	IdentityConfidence string    `json:"identityConfidence,omitempty"`
	FirstSeenAt        time.Time `json:"firstSeenAt"`
	LastSeenAt         time.Time `json:"lastSeenAt"`
	LastDevicePath     string    `json:"lastDevicePath,omitempty"`
	LastAction         string    `json:"lastAction,omitempty"`
	LastJobID          string    `json:"lastJobId,omitempty"`
	LastMessage        string    `json:"lastMessage,omitempty"`
}

// AutoWipeStore persists auto-wipe seen-device state.
type AutoWipeStore struct {
	mu      sync.RWMutex
	dataDir string
	records map[string]AutoWipeRecord
}

// NewAutoWipeStore creates or loads the auto-wipe state store.
func NewAutoWipeStore(dataDir string) (*AutoWipeStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	s := &AutoWipeStore{dataDir: dataDir, records: make(map[string]AutoWipeRecord)}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load auto wipe state: %w", err)
	}
	return s, nil
}

func (s *AutoWipeStore) filePath() string {
	return filepath.Join(s.dataDir, "auto-wipe-seen.json")
}

func (s *AutoWipeStore) load() error {
	data, err := os.ReadFile(s.filePath())
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var records []AutoWipeRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("parse auto wipe state: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range records {
		if rec.DeviceID != "" {
			s.records[rec.DeviceID] = rec
		}
	}
	return nil
}

// List returns records newest-last-seen first.
func (s *AutoWipeStore) List() []AutoWipeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AutoWipeRecord, 0, len(s.records))
	for _, rec := range s.records {
		out = append(out, rec)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].LastSeenAt.After(out[i].LastSeenAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// Has reports whether deviceID has already been tracked.
func (s *AutoWipeStore) Has(deviceID string) bool {
	if deviceID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.records[deviceID]
	return ok
}

// Upsert records or updates a seen physical device.
func (s *AutoWipeStore) Upsert(record AutoWipeRecord) error {
	if record.DeviceID == "" {
		return fmt.Errorf("device id required")
	}
	s.mu.Lock()
	now := time.Now()
	existing, ok := s.records[record.DeviceID]
	if ok {
		record.FirstSeenAt = existing.FirstSeenAt
		if record.Serial == "" {
			record.Serial = existing.Serial
		}
		if record.Model == "" {
			record.Model = existing.Model
		}
	} else if record.FirstSeenAt.IsZero() {
		record.FirstSeenAt = now
	}
	if record.LastSeenAt.IsZero() {
		record.LastSeenAt = now
	}
	s.records[record.DeviceID] = record
	err := s.saveLocked()
	s.mu.Unlock()
	return err
}

// Clear removes all seen-device records.
func (s *AutoWipeStore) Clear() error {
	s.mu.Lock()
	s.records = make(map[string]AutoWipeRecord)
	err := s.saveLocked()
	s.mu.Unlock()
	return err
}

func (s *AutoWipeStore) saveLocked() error {
	records := make([]AutoWipeRecord, 0, len(s.records))
	for _, rec := range s.records {
		records = append(records, rec)
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(s.dataDir, "auto-wipe-seen-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, s.filePath()); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	if dirFd, err := os.Open(s.dataDir); err == nil {
		dirFd.Sync()
		dirFd.Close()
	}
	return nil
}
