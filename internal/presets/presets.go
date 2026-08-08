// Package presets provides named, reusable wipe configurations.
// Presets are persisted as JSON in the data directory.
package presets

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/usb-wiper/internal/jsonfile"
	"github.com/usb-wiper/internal/ulid"
)

// Preset is a named, reusable bundle of wipe settings.
type Preset struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	SchemeID      string `json:"schemeId"`
	AutoFormat    bool   `json:"autoFormat"`
	VerifySizeGB  int    `json:"verifySizeGB"`
	LabelTemplate string `json:"labelTemplate,omitempty"` // e.g. "RMA-{date}-{serial}"
}

// Store manages persistent wipe presets.
type Store struct {
	mu      sync.RWMutex
	file    string
	presets []Preset
}

// builtInPresets are seeded on first run.
var builtInPresets = []Preset{
	{
		ID:           "builtin-quick-zero",
		Name:         "Quick Zero",
		SchemeID:     "zero",
		AutoFormat:   false,
		VerifySizeGB: 1,
	},
	{
		ID:           "builtin-standard-sanitize",
		Name:         "Standard Sanitize",
		SchemeID:     "zero",
		AutoFormat:   true,
		VerifySizeGB: 4,
	},
	{
		ID:           "builtin-dod-3pass",
		Name:         "DoD 3-Pass",
		SchemeID:     "dod-3pass",
		AutoFormat:   true,
		VerifySizeGB: 4,
	},
	{
		ID:           "builtin-paranoid",
		Name:         "Paranoid",
		SchemeID:     "dod-3pass",
		AutoFormat:   true,
		VerifySizeGB: 16,
	},
}

// New creates or loads a preset store from the given data directory.
func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	s := &Store{
		file:    filepath.Join(dataDir, "presets.json"),
		presets: make([]Preset, 0),
	}

	if err := s.load(); err != nil {
		if os.IsNotExist(err) {
			// First run: seed built-in presets
			s.presets = make([]Preset, len(builtInPresets))
			copy(s.presets, builtInPresets)
			if err := s.save(); err != nil {
				return nil, fmt.Errorf("seed presets: %w", err)
			}
		} else {
			return nil, fmt.Errorf("load presets: %w", err)
		}
	}

	return s, nil
}

// load reads presets from the JSON file.
func (s *Store) load() error {
	var presets []Preset
	if err := jsonfile.Read(s.file, "parse presets", &presets); err != nil {
		return err
	}

	s.mu.Lock()
	s.presets = presets
	s.mu.Unlock()
	return nil
}

// List returns all presets.
func (s *Store) List() []Preset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Preset, len(s.presets))
	copy(out, s.presets)
	return out
}

// Get returns a preset by ID.
func (s *Store) Get(id string) (*Preset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.presets {
		if s.presets[i].ID == id {
			p := s.presets[i]
			return &p, nil
		}
	}
	return nil, fmt.Errorf("preset %s not found", id)
}

// Create adds a new preset.
func (s *Store) Create(name, schemeID string, autoFormat bool, verifySizeGB int, labelTemplate string) (*Preset, error) {
	p := Preset{
		ID:            ulid.New(),
		Name:          name,
		SchemeID:      schemeID,
		AutoFormat:    autoFormat,
		VerifySizeGB:  verifySizeGB,
		LabelTemplate: labelTemplate,
	}

	s.mu.Lock()
	s.presets = append(s.presets, p)
	err := s.save()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Update modifies an existing preset.
func (s *Store) Update(id string, name, schemeID *string, autoFormat *bool, verifySizeGB *int, labelTemplate *string) (*Preset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.presets {
		if s.presets[i].ID == id {
			if name != nil {
				s.presets[i].Name = *name
			}
			if schemeID != nil {
				s.presets[i].SchemeID = *schemeID
			}
			if autoFormat != nil {
				s.presets[i].AutoFormat = *autoFormat
			}
			if verifySizeGB != nil {
				s.presets[i].VerifySizeGB = *verifySizeGB
			}
			if labelTemplate != nil {
				s.presets[i].LabelTemplate = *labelTemplate
			}
			p := s.presets[i]
			if err := s.save(); err != nil {
				return nil, err
			}
			return &p, nil
		}
	}
	return nil, fmt.Errorf("preset %s not found", id)
}

// Delete removes a preset. Built-in presets cannot be deleted.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Don't delete built-in presets
	for _, bp := range builtInPresets {
		if bp.ID == id {
			return fmt.Errorf("cannot delete built-in preset %q", bp.Name)
		}
	}

	for i := range s.presets {
		if s.presets[i].ID == id {
			s.presets = append(s.presets[:i], s.presets[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("preset %s not found", id)
}

// save atomically writes presets to file. Caller must hold the write lock.
func (s *Store) save() error {
	return jsonfile.Write(filepath.Dir(s.file), s.file, "presets-*.json.tmp", s.presets)
}
