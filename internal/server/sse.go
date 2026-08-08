package server

import (
	"log"
	"sync"
	"sync/atomic"

	"github.com/usb-wiper/internal/wipe"
)

const sseRingSize = 100

// sseEntry is a buffered event with a monotonic sequence number.
type sseEntry struct {
	id uint64
	ev wipe.ProgressEvent
}

// SSEHub manages Server-Sent Event clients and a replay ring buffer.
type SSEHub struct {
	clients map[chan wipe.ProgressEvent]bool
	mu      sync.Mutex

	// Monotonic event ID counter (atomic).
	nextID uint64

	// Ring buffer for Last-Event-ID replay.
	ring     [sseRingSize]sseEntry
	ringMu   sync.RWMutex
	ringHead uint64 // index of the oldest entry (mod sseRingSize)
	ringFill uint64 // how many entries have been stored (capped at sseRingSize)
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[chan wipe.ProgressEvent]bool),
	}
}

// Subscribe registers a new client and returns its event channel.
// The caller is responsible for calling Unsubscribe when done.
func (h *SSEHub) Subscribe() chan wipe.ProgressEvent {
	ch := make(chan wipe.ProgressEvent, 64)
	h.mu.Lock()
	h.clients[ch] = true
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a client and closes its channel.
func (h *SSEHub) Unsubscribe(ch chan wipe.ProgressEvent) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

// NextID returns the next event ID without consuming it (for initial sync).
func (h *SSEHub) NextID() uint64 {
	return atomic.LoadUint64(&h.nextID)
}

// Broadcast sends a progress event to all connected clients and stores it in
// the ring buffer for Last-Event-ID replay. Implements the queue.SSEHub interface.
func (h *SSEHub) Broadcast(ev wipe.ProgressEvent) {
	id := atomic.AddUint64(&h.nextID, 1)

	// Store in ring buffer.
	h.ringMu.Lock()
	idx := (h.ringHead + h.ringFill) % sseRingSize
	if h.ringFill == sseRingSize {
		// Ring full — advance head (oldest entry evicted).
		h.ringHead = (h.ringHead + 1) % sseRingSize
	} else {
		h.ringFill++
	}
	h.ring[idx] = sseEntry{id: id, ev: ev}
	h.ringMu.Unlock()

	// Broadcast to live clients.
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- ev:
		default:
			log.Printf("SSE client buffer full, skipping event %d", id)
		}
	}
}

// EventsSince returns all ring-buffered events with ID > afterID, in order.
// Used to replay missed events on reconnect (Last-Event-ID).
func (h *SSEHub) EventsSince(afterID uint64) []sseEntry {
	h.ringMu.RLock()
	defer h.ringMu.RUnlock()

	var out []sseEntry
	for i := uint64(0); i < h.ringFill; i++ {
		idx := (h.ringHead + i) % sseRingSize
		if h.ring[idx].id > afterID {
			out = append(out, h.ring[idx])
		}
	}
	return out
}
