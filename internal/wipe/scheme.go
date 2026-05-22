package wipe

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
)

// Scheme represents a wipe strategy (single-pass zero, DoD multi-pass, etc.).
type Scheme interface {
	ID() string         // "zero", "random", "dod-3pass", "nist-clear", "secure-erase"
	DisplayName() string // human-readable name for UI
	Passes() int        // number of write passes
	Execute(ctx context.Context, devicePath string, size uint64,
		progress chan<- ProgressEvent) error
}

// SchemeMeta is lightweight scheme info for API/UI listing.
type SchemeMeta struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Passes      int    `json:"passes"`
	Description string `json:"description"`
}

// SchemeRegistry holds all available wipe schemes.
type SchemeRegistry struct {
	mu      sync.RWMutex
	schemes map[string]Scheme
}

// NewSchemeRegistry creates a registry with all built-in schemes.
func NewSchemeRegistry() *SchemeRegistry {
	r := &SchemeRegistry{schemes: make(map[string]Scheme)}
	r.Register(&SchemeZero{})
	r.Register(&SchemeRandom{})
	r.Register(&SchemeDoD{})
	r.Register(&SchemeNISTClear{})
	r.Register(&SchemeSecureErase{})
	return r
}

// Register adds a scheme. Panics on duplicate ID.
func (r *SchemeRegistry) Register(s Scheme) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.schemes[s.ID()]; ok {
		panic("duplicate scheme id: " + s.ID())
	}
	r.schemes[s.ID()] = s
}

// Get returns a scheme by ID, or an error if not found.
func (r *SchemeRegistry) Get(id string) (Scheme, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.schemes[id]
	if !ok {
		return nil, fmt.Errorf("unknown scheme: %q", id)
	}
	return s, nil
}

// List returns metadata for all registered schemes.
func (r *SchemeRegistry) List() []SchemeMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]SchemeMeta, 0, len(r.schemes))
	for _, s := range r.schemes {
		out = append(out, SchemeMeta{
			ID:          s.ID(),
			DisplayName: s.DisplayName(),
			Passes:      s.Passes(),
			Description: schemeDescription(s.ID()),
		})
	}
	return out
}

func schemeDescription(id string) string {
	switch id {
	case "zero":
		return "Single pass of zeros — fast and effective for most use cases."
	case "random":
		return "Single pass of crypto-random data."
	case "dod-3pass":
		return "DoD 5220.22-M 3-pass: zeros, ones (0xFF), random. Historically used for classified data."
	case "nist-clear":
		return "NIST 800-88 Clear: single zero pass with NIST-compliant metadata."
	case "secure-erase":
		return "ATA Secure Erase / NVMe Format — uses firmware-level sanitize command."
	default:
		return ""
	}
}

// fillRandom fills a byte slice with crypto/rand data.
// Falls back to a simple PRNG if crypto/rand fails (shouldn't happen on modern systems).
func fillRandom(buf []byte) {
	if _, err := rand.Read(buf); err != nil {
		// Extremely unlikely — fall back to a deterministic pattern
		for i := range buf {
			buf[i] = byte(i ^ (i >> 8))
		}
	}
}
