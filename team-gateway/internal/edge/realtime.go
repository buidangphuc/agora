package edge

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// RealtimeBroker multiplexes live events to connected browser SSE clients by room.
// Rooms:
// - "listing:{id}": Flash sale stock battle, price drops
// - "chat:{thread}": Instant live messages
// - "user:{id}": Notification bell
// - "ops:orders": Live Order Ticker for Admin Cockpit
type RealtimeBroker struct {
	mu      sync.RWMutex
	clients map[string]map[chan []byte]struct{}
}

var GlobalBroker = NewRealtimeBroker()

func NewRealtimeBroker() *RealtimeBroker {
	return &RealtimeBroker{
		clients: make(map[string]map[chan []byte]struct{}),
	}
}

func (b *RealtimeBroker) Subscribe(room string) chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.clients[room] == nil {
		b.clients[room] = make(map[chan []byte]struct{})
	}
	ch := make(chan []byte, 16)
	b.clients[room][ch] = struct{}{}
	return ch
}

func (b *RealtimeBroker) Unsubscribe(room string, ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subs, ok := b.clients[room]; ok {
		delete(subs, ch)
		close(ch)
		if len(subs) == 0 {
			delete(b.clients, room)
		}
	}
}

func (b *RealtimeBroker) Broadcast(room string, eventName string, data interface{}) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	payload, err := json.Marshal(map[string]interface{}{
		"event":     eventName,
		"room":      room,
		"data":      data,
		"timestamp": time.Now().Format(time.RFC3339Nano),
	})
	if err != nil {
		return
	}

	if subs, ok := b.clients[room]; ok {
		for ch := range subs {
			select {
			case ch <- payload:
			default:
				// Dropped if client buffer full
			}
		}
	}
}

// HandleSSE handles incoming Server-Sent Events requests from browsers.
func HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	room := r.URL.Query().Get("room")
	if room == "" {
		room = "global"
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := GlobalBroker.Subscribe(room)
	defer GlobalBroker.Unsubscribe(room, ch)

	// Send initial handshake ping
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\",\"room\":\"%s\"}\n\n", room)
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", string(msg))
			flusher.Flush()
		}
	}
}
