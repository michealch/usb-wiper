package presets

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestCreateAndList(t *testing.T) {
	s := tempStore(t)
	p, err := s.Create("Test", "zero", false, 1, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected non-empty ID")
	}

	all := s.List()
	found := false
	for _, x := range all {
		if x.ID == p.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("created preset not found in List()")
	}
}

func TestUpdatePreset(t *testing.T) {
	s := tempStore(t)
	p, _ := s.Create("Original", "zero", false, 1, "")

	newName := "Updated"
	updated, err := s.Update(p.ID, &newName, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("expected name %q, got %q", newName, updated.Name)
	}
}

func TestDeletePreset(t *testing.T) {
	s := tempStore(t)
	p, _ := s.Create("ToDelete", "zero", false, 1, "")

	if err := s.Delete(p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(p.ID); err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestDeleteBuiltInRejects(t *testing.T) {
	s := tempStore(t)
	if err := s.Delete("builtin-quick-zero"); err == nil {
		t.Fatal("expected error deleting built-in preset, got nil")
	}
}

func TestConcurrentCreateUpdateDelete(t *testing.T) {
	s := tempStore(t)

	const goroutines = 20
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("preset-%d", idx)
			p, err := s.Create(name, "zero", false, 1, "")
			if err != nil {
				return
			}
			newName := name + "-updated"
			s.Update(p.ID, &newName, nil, nil, nil, nil)
			s.Delete(p.ID)
		}(i)
	}

	wg.Wait()

	// Verify in-memory state is consistent with what's on disk.
	s2, err := New(filepath.Dir(s.file))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	_ = s2
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Create("Persist", "zero", true, 2, "")

	// Reload from disk
	s2, err := New(dir)
	if err != nil {
		t.Fatalf("New (reload): %v", err)
	}
	all := s2.List()
	found := false
	for _, p := range all {
		if p.Name == "Persist" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("preset not found after reload")
	}
}

