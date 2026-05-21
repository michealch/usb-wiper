package config

import "sync"

// Config holds in-memory application configuration.
// Resets on container restart (no persistence).
type Config struct {
	AutoFormat    bool `json:"autoFormat"`
	UnsafeAllowAllUSB bool `json:"unsafeAllowAllUSB"`
}

type Manager struct {
	mu  sync.RWMutex
	cfg Config
}

// New creates a new config manager with defaults.
func New(unsafeAllowAllUSB bool) *Manager {
	return &Manager{
		cfg: Config{
			AutoFormat:        false,
			UnsafeAllowAllUSB: unsafeAllowAllUSB,
		},
	}
}

// Get returns a copy of the current config.
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// SetAutoFormat updates the auto-format toggle.
func (m *Manager) SetAutoFormat(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg.AutoFormat = v
}
