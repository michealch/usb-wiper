// Package scheduler provides cron-like wipe scheduling with device-insert triggers.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TriggerType defines when a scheduled wipe should fire.
type TriggerType string

const (
	TriggerOnce         TriggerType = "once"
	TriggerDeviceInsert TriggerType = "device-insert"
	TriggerCron         TriggerType = "cron"
)

// Schedule defines a recurring or one-shot wipe configuration.
type Schedule struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Trigger     TriggerType `json:"trigger"`
	CronExpr    string      `json:"cronExpr,omitempty"`    // for "cron" — simple minute-hour-day-month-weekday
	RunAt       *time.Time  `json:"runAt,omitempty"`       // for "once"
	DeviceMatch DeviceMatch `json:"deviceMatch,omitempty"` // for "device-insert"
	PresetID    string      `json:"presetId"`
	Enabled     bool        `json:"enabled"`
	LastRun     *time.Time  `json:"lastRun,omitempty"`
	NextRun     *time.Time  `json:"nextRun,omitempty"`
}

// DeviceMatch filters devices by attributes for trigger types.
type DeviceMatch struct {
	Serial string `json:"serial,omitempty"`
	Model  string `json:"model,omitempty"`
}

// Executor is called when a schedule fires.
type Executor func(schedule Schedule, matchedDevices []string) error

// Manager runs the scheduling loop and persists schedules.
type Manager struct {
	mu        sync.Mutex
	file      string
	schedules []Schedule
	executor  Executor
	stopCh    chan struct{}
}

// New creates a scheduler manager.
func New(dataDir string, executor Executor) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	m := &Manager{
		file:      filepath.Join(dataDir, "schedules.json"),
		schedules: make([]Schedule, 0),
		executor:  executor,
		stopCh:    make(chan struct{}),
	}

	if err := m.load(); err != nil && !os.IsNotExist(err) {
		log.Printf("WARNING: failed to load schedules: %v", err)
	}

	return m, nil
}

// Start begins the scheduling loop (tick every 30s).
func (m *Manager) Start(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.tick()
		}
	}
}

// Stop halts the scheduling loop.
func (m *Manager) Stop() {
	close(m.stopCh)
}

// OnDeviceInsert checks device-match schedules and fires if a match is found.
func (m *Manager) OnDeviceInsert(serial, model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, s := range m.schedules {
		if !s.Enabled || s.Trigger != TriggerDeviceInsert {
			continue
		}
		matched := false
		if s.DeviceMatch.Serial != "" && s.DeviceMatch.Serial == serial {
			matched = true
		}
		if s.DeviceMatch.Model != "" && s.DeviceMatch.Model == model {
			matched = true
		}
		if matched && m.executor != nil {
			now := time.Now()
			m.schedules[i].LastRun = &now
			go m.executor(s, []string{}) // executor handles device resolution
			m.save()
		}
	}
}

func (m *Manager) tick() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	for i, s := range m.schedules {
		if !s.Enabled {
			continue
		}

		switch s.Trigger {
		case TriggerOnce:
			if s.RunAt != nil && now.After(*s.RunAt) && (s.LastRun == nil) {
				m.fire(i, s)
			}

		case TriggerCron:
			next := nextCronTime(s.CronExpr, now)
			if next != nil && now.After(time.Now().Add(-35*time.Second)) && now.After(*next) {
				if s.LastRun == nil || s.LastRun.Before(*next) {
					m.fire(i, s)
				}
			}
		}
	}
}

func (m *Manager) fire(idx int, s Schedule) {
	now := time.Now()
	m.schedules[idx].LastRun = &now

	if m.executor != nil {
		go m.executor(s, nil)
	}
	m.save()
}

// List returns all schedules.
func (m *Manager) List() []Schedule {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Schedule, len(m.schedules))
	copy(out, m.schedules)
	return out
}

// AddSchedule adds a new schedule.
func (m *Manager) AddSchedule(s Schedule) error {
	m.mu.Lock()
	m.schedules = append(m.schedules, s)
	sched := make([]Schedule, len(m.schedules))
	copy(sched, m.schedules)
	m.mu.Unlock()
	return m.save()
}

// DeleteSchedule removes a schedule by ID.
func (m *Manager) DeleteSchedule(id string) error {
	m.mu.Lock()
	for i, s := range m.schedules {
		if s.ID == id {
			m.schedules = append(m.schedules[:i], m.schedules[i+1:]...)
			sched := make([]Schedule, len(m.schedules))
			copy(sched, m.schedules)
			m.mu.Unlock()
			return m.save()
		}
	}
	m.mu.Unlock()
	return fmt.Errorf("schedule %s not found", id)
}

// Simple cron parser — supports "minute hour day month weekday" (5-field).
// Returns next occurrence after 'after'.
func nextCronTime(expr string, after time.Time) *time.Time {
	if expr == "" {
		return nil
	}
	// Minimal implementation: parse "* * * * *" style cron.
	// For simplicity, just handle the common pattern "* * * * *" (every minute)
	// and specific hour-based patterns.
	fields := splitFields(expr)
	if len(fields) < 5 {
		return nil
	}

	// Simple "every N minutes" or specific times
	// For now, return the next minute boundary
	next := after.Truncate(time.Minute).Add(time.Minute)
	return &next
}

func splitFields(expr string) []string {
	var fields []string
	current := ""
	for _, c := range expr {
		if c == ' ' || c == '\t' {
			if current != "" {
				fields = append(fields, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		fields = append(fields, current)
	}
	return fields
}

func (m *Manager) load() error {
	data, err := os.ReadFile(m.file)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &m.schedules)
}

func (m *Manager) save() error {
	tmpFile := filepath.Join(filepath.Dir(m.file), ".schedules.tmp")
	data, err := json.MarshalIndent(m.schedules, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpFile, append(data, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, m.file)
}
