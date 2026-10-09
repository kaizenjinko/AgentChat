package hub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Client is a single SSE subscriber.
type Client struct {
	UserID int64
	Room   string
	Ch     chan []byte
}

// Hub tracks SSE clients and fans out events.
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

// New creates an empty hub.
func New() *Hub {
	return &Hub{clients: make(map[*Client]struct{})}
}

// Add registers a client and returns it. The caller must Remove it on disconnect.
func (h *Hub) Add(userID int64, room string) *Client {
	c := &Client{UserID: userID, Room: room, Ch: make(chan []byte, 64)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

// Remove unregisters a client.
func (h *Hub) Remove(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.Ch)
	}
	h.mu.Unlock()
}

// Count returns the number of registered clients.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Broadcast sends a "message" event for the given room to matching clients.
func (h *Hub) Broadcast(userID int64, room string, msg any) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.send(userID, room, sseFrame("message", payload))
}

// NotifyRoomDeleted sends a "room-deleted" event for the given room.
func (h *Hub) NotifyRoomDeleted(userID int64, room string) {
	h.send(userID, room, sseFrame("room-deleted", []byte(`{"room":`+strconvJSON(room)+`}`)))
}

// sseFrame builds a single "event: <name>\ndata: <payload>\n\n" frame in one
// allocation, avoiding the intermediate fmt.Sprintf + []byte conversion.
func sseFrame(name string, payload []byte) []byte {
	n := len("event: \ndata: \n\n") + len(name) + len(payload)
	b := make([]byte, 0, n)
	b = append(b, "event: "...)
	b = append(b, name...)
	b = append(b, '\n', 'd', 'a', 't', 'a', ':', ' ')
	b = append(b, payload...)
	b = append(b, '\n', '\n')
	return b
}

// strconvJSON quotes s as a JSON string body (without the surrounding quotes).
func strconvJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// send fans out a preformatted frame to clients in room (or "*").
func (h *Hub) send(userID int64, room string, frame []byte) {
	h.mu.RLock()
	targets := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		if (userID == 0 || c.UserID == userID) && (c.Room == room || c.Room == "*") {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range targets {
		select {
		case c.Ch <- frame:
		default:
			// Slow consumer: drop the client to avoid blocking broadcast.
			h.Remove(c)
		}
	}
}

// Stream runs the SSE loop for a client until the request context is done.
// It relies on the ResponseWriter supporting http.Flusher.
func (h *Hub) Stream(w http.ResponseWriter, r *http.Request, c *Client) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	hello, _ := json.Marshal(map[string]string{"room": c.Room})
	fmt.Fprintf(w, "event: hello\ndata: %s\n\n", hello)
	flusher.Flush()

	ping := make(chan struct{})
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				select {
				case c.Ch <- []byte(": ping\n\n"):
				default:
				}
			case <-ping:
				return
			}
		}
	}()

	defer func() {
		close(ping)
		h.Remove(c)
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-c.Ch:
			if !ok {
				return
			}
			if _, err := w.Write(frame); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
