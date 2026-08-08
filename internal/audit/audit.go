// Package audit provides an append-only event log for security-relevant operations.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event represents a single auditable action.
type Event struct {
	Timestamp time.Time              `json:"ts"`
	Actor     string                 `json:"actor"`
	Event     string                 `json:"event"`
	Target    string                 `json:"target,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	RequestID string                 `json:"requestId,omitempty"`
}

// Logger provides thread-safe append-only audit logging to a file.
type Logger struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	maxSize int64 // rotate at this many bytes (default 50 MiB)
}

// New creates or opens an audit log file.
func New(dataDir string) (*Logger, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create audit dir: %w", err)
	}

	path := filepath.Join(dataDir, "audit.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return nil, fmt.Errorf("open audit log: %w", err)
	}

	return &Logger{
		file:    f,
		path:    path,
		maxSize: 50 * 1024 * 1024, // 50 MB
	}, nil
}

// Log appends an event to the audit log.
func (l *Logger) Log(ev Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}

	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}

	data = append(data, '\n')

	if l.file == nil {
		return fmt.Errorf("audit log unavailable (previous rotation failed)")
	}

	if _, err := l.file.Write(data); err != nil {
		return fmt.Errorf("write audit: %w", err)
	}

	// Sync after each write for durability
	if err := l.file.Sync(); err != nil {
		return fmt.Errorf("sync audit: %w", err)
	}

	// Rotate if needed
	info, err := l.file.Stat()
	if err == nil && info.Size() >= l.maxSize {
		if rotErr := l.rotate(); rotErr != nil {
			return fmt.Errorf("audit rotate: %w", rotErr)
		}
	}

	return nil
}

// rotate closes the current log, renames it with a timestamp suffix, and
// reopens a fresh log file. Returns an error if any step fails; on failure
// l.file is set to nil so subsequent Log() calls return a clear error
// rather than writing to a closed/nil fd.
func (l *Logger) rotate() error {
	l.file.Close()
	l.file = nil

	ts := time.Now().Format("20060102-150405")
	newPath := filepath.Join(filepath.Dir(l.path), fmt.Sprintf("audit-%s.log", ts))
	if err := os.Rename(l.path, newPath); err != nil {
		return fmt.Errorf("rotate rename: %w", err)
	}

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return fmt.Errorf("rotate reopen: %w", err)
	}
	l.file = f
	return nil
}

// Read returns recent audit events (newest first, up to limit).
func (l *Logger) Read(limit int) ([]Event, error) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, err
	}

	var events []Event
	for _, line := range bytesToLines(data) {
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}

	// Reverse: newest first
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}

	return events, nil
}

// Close flushes and closes the audit log.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

func bytesToLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
