package chat

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"
)

// Envelope — универсальный формат сообщения по WS
type Envelope struct {
	Type    string          `json:"type"`
	RoomID  string          `json:"room_id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Hub управляет соединениями только этой ноды.
// Между нодами ходят через Redis Pub/Sub, поэтому глобального состояния тут нет.
type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{rooms: make(map[string]map[*Client]struct{})}
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[c.roomID] == nil {
		h.rooms[c.roomID] = make(map[*Client]struct{})
	}
	h.rooms[c.roomID][c] = struct{}{}
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients, ok := h.rooms[c.roomID]; ok {
		if _, exists := clients[c]; exists {
			delete(clients, c)
			close(c.send)
		}
		if len(clients) == 0 {
			delete(h.rooms, c.roomID)
		}
	}
}

// broadcastLocal — рассылает клиентам этой ноды. Медленных не ждём.
func (h *Hub) broadcastLocal(roomID string, data []byte) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients := h.rooms[roomID]
	n := 0
	for c := range clients {
		select {
		case c.send <- data:
			n++
		default:
			// буфер полон — дропаем, клиент отвалится по ping timeout
		}
	}
	return n
}

// LocalClients — уникальные userID на этой ноде в комнате
func (h *Hub) LocalClients(roomID string) []uuid.UUID {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]uuid.UUID, 0, len(h.rooms[roomID]))
	seen := make(map[uuid.UUID]struct{})
	for c := range h.rooms[roomID] {
		if _, ok := seen[c.userID]; ok {
			continue
		}
		seen[c.userID] = struct{}{}
		out = append(out, c.userID)
	}
	return out
}
