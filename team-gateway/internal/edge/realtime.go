package edge

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

// SSEHandler serves /api/events/live. Room access is decided at the edge:
//   - "global", "listing:*"  public (no credential needed);
//   - "user:{id}"            the verified principal's id must equal {id};
//   - "chat:*"               any authenticated (non-anonymous) principal;
//   - "ops:*"                the admin scope;
//   - anything else          rejected.
//
// A browser EventSource cannot set an Authorization header, so the `session`
// cookie (the same JWT) is honored as well. CORS is applied by the gateway's
// CORS middleware (CORS_ORIGINS); this handler sets no CORS headers itself.
type SSEHandler struct {
	edge   *Edge
	broker *RealtimeBroker
}

// NewSSEHandler builds the SSE handler over the given broker.
func NewSSEHandler(e *Edge, b *RealtimeBroker) *SSEHandler {
	return &SSEHandler{edge: e, broker: b}
}

// authorizeRoom returns an HTTP status (0 = allowed) and message for the room, plus
// the verified principal when the room required a credential (nil for public rooms).
func (h *SSEHandler) authorizeRoom(r *http.Request, room string) (int, string, *resolvedPrincipal) {
	prefix, rest, _ := strings.Cut(room, ":")
	if room == "global" || (prefix == "listing" && rest != "") {
		return 0, "", nil
	}
	if prefix != "user" && prefix != "chat" && prefix != "ops" || rest == "" {
		return http.StatusForbidden, "unknown room", nil
	}

	header := r.Header
	if c, err := r.Cookie(sessionCookie); err == nil && header.Get("Authorization") == "" {
		header = header.Clone()
		header.Set("Authorization", "Bearer "+strings.TrimSpace(c.Value))
	}
	p, err := h.edge.resolve(header)
	if err != nil {
		return http.StatusUnauthorized, "invalid or expired bearer token", nil
	}
	if p.ptype == "anonymous" {
		return http.StatusUnauthorized, "authentication required", nil
	}
	switch prefix {
	case "user":
		if p.ptype != "user" || p.id != rest {
			return http.StatusForbidden, "forbidden room", nil
		}
	case "ops":
		if !hasScope(p.scopes, adminScope) {
			return http.StatusForbidden, "insufficient_scope: admin required", nil
		}
	}
	return 0, "", &p
}

// ServeHTTP handles incoming Server-Sent Events requests from browsers.
func (h *SSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	room := r.URL.Query().Get("room")
	if room == "" {
		room = "global"
	}
	code, msg, principal := h.authorizeRoom(r, room)
	if code != 0 {
		if code == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", "Bearer")
		}
		http.Error(w, msg, code)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// An authenticated room is watched: the connection ends at the token's exp or
	// when its session is revoked. Public rooms have no principal and no watcher.
	ctx := r.Context()
	var life *streamLifetime
	if principal != nil {
		ctx, life = h.edge.watchStream(ctx, *principal)
		defer life.stop()
	}

	ch := h.broker.Subscribe(room)
	defer h.broker.Unsubscribe(room, ch)

	// Send initial handshake ping
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\",\"room\":%q}\n\n", room)
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if life != nil && life.ended() && r.Context().Err() == nil {
				fmt.Fprintf(w, "event: unauthenticated\ndata: {\"code\":\"unauthenticated\",\"reason\":%q}\n\n", life.reason())
				flusher.Flush()
			}
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
