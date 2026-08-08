// Package jsonfile provides the atomic JSON file persistence shared by the settings,
// presets, wipe-history, health-history, and auto-wipe stores.
package jsonfile

import (
	"encoding/json"
	"fmt"
	"os"
)

// Write atomically writes v to dest as indented JSON with a trailing newline.
// It creates the temp file in dir via os.CreateTemp (mode 0600), fsyncs it, renames it
// over dest, then best-effort fsyncs dir. The caller must hold the store's write lock.
func Write(dir, dest, tmpPattern string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, tmpPattern)
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
	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	if dirFd, err := os.Open(dir); err == nil {
		dirFd.Sync()
		dirFd.Close()
	}
	return nil
}

// Read loads JSON from file into out. An empty file is not an error and leaves out
// untouched. The raw os.ReadFile error is returned unwrapped so callers can keep their
// own os.IsNotExist policy. parseErr prefixes unmarshal failures.
func Read(file, parseErr string, out any) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: %w", parseErr, err)
	}
	return nil
}
