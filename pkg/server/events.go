package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type EventHub struct {
	mu      sync.Mutex
	clients map[chan []byte]bool
}

func NewEventHub() *EventHub {
	return &EventHub{
		clients: make(map[chan []byte]bool),
	}
}

func (h *EventHub) Publish(eventType string, data any) {
	h.mu.Lock()
	defer h.mu.Unlock()

	payload, ok := data.(map[string]any)
	if !ok {
		payload = map[string]any{"value": data}
	}
	payload["kind"] = eventType
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	msg := []byte("data: " + string(raw) + "\n\n")

	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (h *EventHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[ch] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
		close(ch)
	}()

	// Clear the server WriteTimeout for long-lived SSE streams.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})

	// Send initial ping
	if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
		return
	}
	flusher.Flush()

	ping := time.NewTicker(protocol.KeepaliveInterval)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if _, err := w.Write(msg); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
