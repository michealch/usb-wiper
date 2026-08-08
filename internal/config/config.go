package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/usb-wiper/internal/jsonfile"
)

// Config holds in-memory application configuration with persistence support.
// Env vars override file values on boot.
type Config struct {
	AutoFormat        bool   `json:"autoFormat"`
	UnsafeAllowAllUSB bool   `json:"unsafeAllowAllUSB"`
	VerifySizeGB      int    `json:"verifySizeGB"`
	MaxParallelJobs   int    `json:"maxParallelJobs"`
	DefaultSchemeID   string `json:"defaultSchemeId"`
	AutoWipeEnabled   bool   `json:"autoWipeEnabled"`
}

type Manager struct {
	mu      sync.RWMutex
	cfg     Config
	dataDir string
	file    string
}

// New creates a new config manager with defaults, loading from file if present.
// Env-var values override any persisted or default values.
func New(unsafeAllowAllUSB bool, dataDir string) *Manager {
	cfg := Config{
		AutoFormat:        false,
		UnsafeAllowAllUSB: unsafeAllowAllUSB,
		VerifySizeGB:      1,
		MaxParallelJobs:   2,
		DefaultSchemeID:   "zero",
		AutoWipeEnabled:   false,
	}

	m := &Manager{
		cfg:     cfg,
		dataDir: dataDir,
		file:    filepath.Join(dataDir, "settings.json"),
	}

	// Load from persisted file (env vars will override below)
	if err := m.load(); err != nil && !os.IsNotExist(err) {
		// Log warning but continue with defaults
		fmt.Fprintf(os.Stderr, "WARNING: failed to load settings: %v\n", err)
	}

	return m
}

// Get returns a copy of the current config.
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// SetAutoFormat updates the auto-format toggle and persists.
func (m *Manager) SetAutoFormat(v bool) {
	m.mu.Lock()
	m.cfg.AutoFormat = v
	m.save()
	m.mu.Unlock()
}

// SetVerifySizeGB updates the verification size and persists.
func (m *Manager) SetVerifySizeGB(v int) {
	m.mu.Lock()
	m.cfg.VerifySizeGB = v
	m.save()
	m.mu.Unlock()
}

// Update applies multiple settings at once and persists.
func (m *Manager) Update(updates ConfigUpdate) {
	m.mu.Lock()
	if updates.AutoFormat != nil {
		m.cfg.AutoFormat = *updates.AutoFormat
	}
	if updates.VerifySizeGB != nil {
		m.cfg.VerifySizeGB = *updates.VerifySizeGB
	}
	if updates.MaxParallelJobs != nil {
		m.cfg.MaxParallelJobs = *updates.MaxParallelJobs
	}
	if updates.DefaultSchemeID != nil {
		m.cfg.DefaultSchemeID = *updates.DefaultSchemeID
	}
	if updates.AutoWipeEnabled != nil {
		m.cfg.AutoWipeEnabled = *updates.AutoWipeEnabled
	}
	m.save()
	m.mu.Unlock()
}

// ConfigUpdate is a partial config payload for PATCH/PUT operations.
type ConfigUpdate struct {
	AutoFormat      *bool   `json:"autoFormat,omitempty"`
	VerifySizeGB    *int    `json:"verifySizeGB,omitempty"`
	MaxParallelJobs *int    `json:"maxParallelJobs,omitempty"`
	DefaultSchemeID *string `json:"defaultSchemeId,omitempty"`
	AutoWipeEnabled *bool   `json:"autoWipeEnabled,omitempty"`
}

// load reads settings from the JSON file.
func (m *Manager) load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var cfg Config
	if err := jsonfile.Read(m.file, "parse settings", &cfg); err != nil {
		return err
	}

	// Merge: preserve env-var overrides
	cfg.UnsafeAllowAllUSB = m.cfg.UnsafeAllowAllUSB
	m.cfg = cfg
	return nil
}

// save atomically writes settings to file. Caller must hold m.mu.RLock or Lock.
func (m *Manager) save() {
	if m.dataDir == "" {
		return
	}
	if err := jsonfile.Write(m.dataDir, m.file, "settings-*.json.tmp", m.cfg); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: save settings: %v\n", err)
	}
}
