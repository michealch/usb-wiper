package server

import (
	"log"
	"sync"

	"github.com/usb-wiper/internal/wipe"
)

// SSEHub manages Server-Sent Event clients.
type SSEHub struct {
	clients map[chan wipe.ProgressEvent]bool
	mu      sync.Mutex
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

// Broadcast sends a progress event to all connected clients.
// If a client's buffer is full, the event is skipped for that client
// (avoids blocking the sender).
func (h *SSEHub) Broadcast(ev wipe.ProgressEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for ch := range h.clients {
		select {
		case ch <- ev:
		default:
			// Client buffer full, skip event
			log.Printf("SSE client buffer full, skipping event")
		}
	}
}
