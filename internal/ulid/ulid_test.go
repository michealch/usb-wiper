package ulid

import (
	"sort"
	"testing"
)

func TestULIDFormat(t *testing.T) {
	id := New()
	if len(id) != 26 {
		t.Fatalf("expected 26 chars, got %d: %q", len(id), id)
	}
	for _, c := range id {
		if !isValidCrockford(byte(c)) {
			t.Fatalf("invalid Crockford char %q in ULID %q", c, id)
		}
	}
}

func TestULIDMonotonicity(t *testing.T) {
	const n = 10000
	ids := make([]string, n)
	for i := range ids {
		ids[i] = New()
	}

	for i := 1; i < n; i++ {
		if ids[i] <= ids[i-1] {
			t.Fatalf("ULID not strictly increasing at index %d: %q <= %q", i, ids[i], ids[i-1])
		}
	}
}

func TestULIDUniqueness(t *testing.T) {
	const n = 10000
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		id := New()
		if seen[id] {
			t.Fatalf("duplicate ULID: %q", id)
		}
		seen[id] = true
	}
}

func TestULIDLexicographicSort(t *testing.T) {
	const n = 1000
	ids := make([]string, n)
	for i := range ids {
		ids[i] = New()
	}
	sorted := make([]string, n)
	copy(sorted, ids)
	sort.Strings(sorted)
	for i := range ids {
		if ids[i] != sorted[i] {
			t.Fatalf("sort order differs at index %d: original=%q sorted=%q", i, ids[i], sorted[i])
		}
	}
}

func isValidCrockford(c byte) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'A' && c <= 'H') ||
		(c >= 'J' && c <= 'N') ||
		(c >= 'P' && c <= 'T') ||
		(c >= 'V' && c <= 'Z')
}
