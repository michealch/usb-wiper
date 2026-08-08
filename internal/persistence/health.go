package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/usb-wiper/internal/jsonfile"
)

// HealthRecord is a point-in-time SMART/health snapshot for a physical disk.
type HealthRecord struct {
	DeviceID            string                 `json:"deviceId"`
	DevicePath          string                 `json:"devicePath"`
	IdentitySource      string                 `json:"identitySource,omitempty"`
	IdentityConfidence  string                 `json:"identityConfidence,omitempty"`
	Model               string                 `json:"model,omitempty"`
	Serial              string                 `json:"serial,omitempty"`
	Firmware            string                 `json:"firmware,omitempty"`
	WWN                 string                 `json:"wwn,omitempty"`
	SizeBytes           uint64                 `json:"sizeBytes,omitempty"`
	CapturedAt          time.Time              `json:"capturedAt"`
	HealthStatus        string                 `json:"healthStatus"`
	DeviceType          string                 `json:"deviceType,omitempty"`
	PowerOnHours        uint64                 `json:"powerOnHours,omitempty"`
	PowerCycleCount     uint64                 `json:"powerCycleCount,omitempty"`
	TemperatureC        int                    `json:"temperatureC,omitempty"`
	ReadLBAs            uint64                 `json:"readLBAs,omitempty"`
	WriteLBAs           uint64                 `json:"writeLBAs,omitempty"`
	AvailableSparePct   int                    `json:"availableSparePct,omitempty"`
	EnduranceUsedPct    int                    `json:"enduranceUsedPct,omitempty"`
	ReallocatedSectors  uint64                 `json:"reallocatedSectors,omitempty"`
	PendingSectors      uint64                 `json:"pendingSectors,omitempty"`
	UncorrectableErrors uint64                 `json:"uncorrectableErrors,omitempty"`
	Raw                 map[string]interface{} `json:"raw,omitempty"`
}

// HealthStore provides thread-safe SMART/health snapshot persistence.
type HealthStore struct {
	mu      sync.RWMutex
	dataDir string
	records []HealthRecord
}

// NewHealthStore creates or loads a health snapshot store.
func NewHealthStore(dataDir string) (*HealthStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	s := &HealthStore{dataDir: dataDir, records: make([]HealthRecord, 0)}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load health history: %w", err)
	}
	return s, nil
}

func (s *HealthStore) filePath() string {
	return filepath.Join(s.dataDir, "health-history.json")
}

func (s *HealthStore) load() error {
	var records []HealthRecord
	if err := jsonfile.Read(s.filePath(), "parse health history", &records); err != nil {
		return err
	}
	s.mu.Lock()
	s.records = records
	s.mu.Unlock()
	return nil
}

// Append adds a health snapshot and atomically persists it.
func (s *HealthStore) Append(record HealthRecord) error {
	s.mu.Lock()
	s.records = append(s.records, record)
	err := s.save()
	s.mu.Unlock()
	return err
}

// GetByDeviceID returns snapshots for a physical device, newest first.
func (s *HealthStore) GetByDeviceID(deviceID string) []HealthRecord {
	if deviceID == "" {
		return []HealthRecord{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []HealthRecord{}
	for i := len(s.records) - 1; i >= 0; i-- {
		if s.records[i].DeviceID == deviceID {
			result = append(result, s.records[i])
		}
	}
	return result
}

// GetLatestByDeviceID returns the newest snapshot for a physical device.
func (s *HealthStore) GetLatestByDeviceID(deviceID string) *HealthRecord {
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

func (s *HealthStore) save() error {
	return jsonfile.Write(s.dataDir, s.filePath(), "health-history-*.json.tmp", s.records)
}
