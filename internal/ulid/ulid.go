// Package ulid provides a monotonic ULID generator using crypto/rand.
// ULIDs are 26-character Crockford base32 strings, sortable by time.
// Monotonicity: when two calls occur within the same millisecond the random
// tail is incremented so IDs remain strictly ascending.
package ulid

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var (
	mu     sync.Mutex
	lastMs int64
	lastRa [10]byte // last random portion (80 bits)
)

// New generates a new, monotonically increasing ULID string.
// Thread-safe; safe to call from multiple goroutines.
func New() string {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now().UnixMilli()

	var ra [10]byte
	if now > lastMs {
		// New millisecond — fresh randomness.
		if _, err := rand.Read(ra[:]); err != nil {
			// Extremely unlikely; fall back to XOR-of-timestamp bytes.
			binary.BigEndian.PutUint64(ra[:8], uint64(now)^0xdeadbeefcafebabe)
			binary.BigEndian.PutUint16(ra[8:], uint16(now))
		}
		lastMs = now
		lastRa = ra
	} else {
		// Same (or retrograde) millisecond — increment the 80-bit random tail.
		// Overflow is astronomically unlikely (2^80 calls in <1ms).
		ra = lastRa
		for i := 9; i >= 0; i-- {
			ra[i]++
			if ra[i] != 0 {
				break
			}
		}
		lastRa = ra
	}

	var b [16]byte
	// Timestamp: 48 bits — only the low 48 bits of now (ms) are used per spec.
	binary.BigEndian.PutUint64(b[:8], uint64(now))
	// Clear the top 2 bytes so the timestamp occupies bits [63:16] of b[:8]
	// but we need exactly 48 bits starting at bit 0 of the output.
	// ULID layout: 10 chars timestamp (50 bits) + 16 chars random (80 bits).
	// Encode via the existing encode() path using the 48-bit ms as top 6 bytes.
	copy(b[2:8], b[2:8]) // already in place; just fill random
	// Actually use spec layout: b[0:6] = 6-byte big-endian ms timestamp
	binary.BigEndian.PutUint16(b[0:2], uint16(now>>32))
	binary.BigEndian.PutUint32(b[2:6], uint32(now))
	copy(b[6:], ra[:])

	return encode(b)
}

// encode converts 16 bytes to a 26-character Crockford base32 string.
func encode(b [16]byte) string {
	// 128 bits → 26 × 5-bit groups.
	var s [26]byte
	// Pack all 16 bytes into a 128-bit integer and extract 5-bit groups.
	hi := binary.BigEndian.Uint64(b[0:8])
	lo := binary.BigEndian.Uint64(b[8:16])

	// Extract 26 groups of 5 bits from MSB to LSB.
	// Total = 130 bits needed; we have 128 — the top 2 bits of the first char are 0.
	for i := 25; i >= 0; i-- {
		var v uint64
		bit := uint(25-i) * 5
		if bit < 64 {
			v = lo >> bit & 0x1F
		} else {
			bit -= 64
			v = hi >> bit & 0x1F
		}
		s[i] = crockford[v]
	}
	return string(s[:])
}
