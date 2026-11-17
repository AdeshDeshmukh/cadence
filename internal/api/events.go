package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Event represents a broadcast event delivered over Server-Sent Events.
type Event struct {
	ID        string    `json:"id,omitempty"`
	Type      string    `json:"type"`
	Data      any       `json:"data"`
	Timestamp time.Time `json:"timestamp"`
}

// Broadcaster manages active SSE client streams and event fan-out.
type Broadcaster struct {
	mu      sync.RWMutex
	clients map[chan Event]struct{}
}

// NewBroadcaster initializes an empty SSE Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients: make(map[chan Event]struct{}),
	}
}

// Subscribe registers a new subscriber channel.
func (b *Broadcaster) Subscribe() chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan Event, 64)
	b.clients[ch] = struct{}{}
	return ch
}

// Unsubscribe removes and closes a subscriber channel.
func (b *Broadcaster) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.clients[ch]; ok {
		delete(b.clients, ch)
		close(ch)
	}
}

// Broadcast distributes an event to all connected SSE clients without blocking.
func (b *Broadcaster) Broadcast(eventType string, data any) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	ev := Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}

	for ch := range b.clients {
		select {
		case ch <- ev:
		default:
			// Non-blocking drop if client is saturated
		}
	}
}

// ClientCount returns the number of active SSE subscribers.
func (b *Broadcaster) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}

// ServeHTTP handles incoming HTTP requests for the /v1/events SSE stream.
func (b *Broadcaster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	// Send initial connected event
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
	flusher.Flush()

	notify := r.Context().Done()
	ticker := time.NewTicker(15 * time.Second) // keep-alive ping
	defer ticker.Stop()

	for {
		select {
		case <-notify:
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": keep-alive\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev.Data)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, string(payload))
			flusher.Flush()
		}
	}
}
