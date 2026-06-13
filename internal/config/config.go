package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Config holds in-memory application configuration with persistence support.
// Env vars override file values on boot.
type Config struct {
	AutoFormat        bool   `json:"autoFormat"`
	UnsafeAllowAllUSB bool   `json:"unsafeAllowAllUSB"`
	VerifySizeGB      int    `json:"verifySizeGB"`
	MaxParallelJobs   int    `json:"maxParallelJobs"`
	DefaultSchemeID   string `json:"defaultSchemeId"`
	DefaultPresetID   string `json:"defaultPresetId"`
	AutoWipeEnabled   bool   `json:"autoWipeEnabled"`
	Theme             string `json:"theme"`           // "dark" | "light"
	NotificationsOn   bool   `json:"notificationsOn"` // enable email/webhook notifications
	WebhookURL        string `json:"webhookUrl,omitempty"`
	AuthEnabled       bool   `json:"authEnabled"`
	AuthToken         string `json:"authToken,omitempty"`  // stored hashed in prod
	HistoryRetention  int    `json:"historyRetentionDays"` // days to keep history, 0 = forever
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
		DefaultPresetID:   "",
		AutoWipeEnabled:   false,
		Theme:             "dark",
		NotificationsOn:   false,
		WebhookURL:        "",
		AuthEnabled:       false,
		AuthToken:         "",
		HistoryRetention:  0,
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
	if updates.DefaultPresetID != nil {
		m.cfg.DefaultPresetID = *updates.DefaultPresetID
	}
	if updates.AutoWipeEnabled != nil {
		m.cfg.AutoWipeEnabled = *updates.AutoWipeEnabled
	}
	if updates.Theme != nil {
		m.cfg.Theme = *updates.Theme
	}
	if updates.NotificationsOn != nil {
		m.cfg.NotificationsOn = *updates.NotificationsOn
	}
	if updates.WebhookURL != nil {
		m.cfg.WebhookURL = *updates.WebhookURL
	}
	if updates.AuthEnabled != nil {
		m.cfg.AuthEnabled = *updates.AuthEnabled
	}
	if updates.AuthToken != nil {
		m.cfg.AuthToken = *updates.AuthToken
	}
	if updates.HistoryRetention != nil {
		m.cfg.HistoryRetention = *updates.HistoryRetention
	}
	m.save()
	m.mu.Unlock()
}

// ConfigUpdate is a partial config payload for PATCH/PUT operations.
type ConfigUpdate struct {
	AutoFormat       *bool   `json:"autoFormat,omitempty"`
	VerifySizeGB     *int    `json:"verifySizeGB,omitempty"`
	MaxParallelJobs  *int    `json:"maxParallelJobs,omitempty"`
	DefaultSchemeID  *string `json:"defaultSchemeId,omitempty"`
	DefaultPresetID  *string `json:"defaultPresetId,omitempty"`
	AutoWipeEnabled  *bool   `json:"autoWipeEnabled,omitempty"`
	Theme            *string `json:"theme,omitempty"`
	NotificationsOn  *bool   `json:"notificationsOn,omitempty"`
	WebhookURL       *string `json:"webhookUrl,omitempty"`
	AuthEnabled      *bool   `json:"authEnabled,omitempty"`
	AuthToken        *string `json:"authToken,omitempty"`
	HistoryRetention *int    `json:"historyRetentionDays,omitempty"`
}

// load reads settings from the JSON file.
func (m *Manager) load() error {
	data, err := os.ReadFile(m.file)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse settings: %w", err)
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

	data, err := json.MarshalIndent(m.cfg, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: marshal settings: %v\n", err)
		return
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(m.dataDir, "settings-*.json.tmp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: create temp settings: %v\n", err)
		return
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "WARNING: write temp settings: %v\n", err)
		return
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "WARNING: sync temp settings: %v\n", err)
		return
	}

	tmp.Close()

	if err := os.Rename(tmpName, m.file); err != nil {
		os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "WARNING: rename settings: %v\n", err)
		return
	}

	// Sync directory to ensure rename is durable
	if dirFd, err := os.Open(m.dataDir); err == nil {
		dirFd.Sync()
		dirFd.Close()
	}
}
