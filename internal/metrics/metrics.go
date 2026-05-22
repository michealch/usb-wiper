// Package metrics provides Prometheus-compatible metrics exposition.
// Minimal implementation using stdlib only — serves /metrics endpoint.
package metrics

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// Registry holds counters and gauges for Prometheus scraping.
type Registry struct {
	mu       sync.RWMutex
	counters map[string]*int64
	gauges   map[string]*int64
	labels   map[string]map[string]string
}

// New creates a new metrics registry.
func New() *Registry {
	return &Registry{
		counters: make(map[string]*int64),
		gauges:   make(map[string]*int64),
		labels:   make(map[string]map[string]string),
	}
}

// NewCounter registers and returns a pointer to a counter.
func (r *Registry) NewCounter(name, help string) *int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var v int64
	r.counters[name] = &v
	r.labels[name] = map[string]string{"help": help}
	return &v
}

// NewGauge registers and returns a pointer to a gauge.
func (r *Registry) NewGauge(name, help string) *int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var v int64
	r.gauges[name] = &v
	r.labels[name] = map[string]string{"help": help}
	return &v
}

// SetGauge sets a gauge value.
func (r *Registry) SetGauge(name string, val int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.gauges[name]; ok {
		atomic.StoreInt64(g, val)
	}
}

// IncCounter increments a counter.
func (r *Registry) IncCounter(name string) {
	r.mu.RLock()
	c, ok := r.counters[name]
	r.mu.RUnlock()
	if ok {
		atomic.AddInt64(c, 1)
	}
}

// AddCounter adds to a counter.
func (r *Registry) AddCounter(name string, delta int64) {
	r.mu.RLock()
	c, ok := r.counters[name]
	r.mu.RUnlock()
	if ok {
		atomic.AddInt64(c, delta)
	}
}

// Handler returns an http.Handler for the /metrics endpoint.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		var b strings.Builder

		r.mu.RLock()
		defer r.mu.RUnlock()

		for name, ptr := range r.counters {
			help := r.labels[name]["help"]
			b.WriteString(fmt.Sprintf("# HELP %s %s\n", name, help))
			b.WriteString(fmt.Sprintf("# TYPE %s counter\n", name))
			b.WriteString(fmt.Sprintf("%s %d\n", name, atomic.LoadInt64(ptr)))
		}

		for name, ptr := range r.gauges {
			help := r.labels[name]["help"]
			b.WriteString(fmt.Sprintf("# HELP %s %s\n", name, help))
			b.WriteString(fmt.Sprintf("# TYPE %s gauge\n", name))
			b.WriteString(fmt.Sprintf("%s %d\n", name, atomic.LoadInt64(ptr)))
		}

		w.Write([]byte(b.String()))
	})
}
