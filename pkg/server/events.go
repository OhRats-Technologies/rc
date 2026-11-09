package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
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

	// Send initial ping
	fmt.Fprintf(w, ": ping\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, _ = w.Write(msg)
			flusher.Flush()
		}
	}
}
