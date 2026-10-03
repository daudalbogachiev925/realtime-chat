package chat

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMsgSize = 4096
)

type Incoming struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type SendMessagePayload struct {
	Body     string  `json:"body"`
	ParentID *string `json:"parent_id,omitempty"`
}

type TypingPayload struct {
	Typing bool `json:"typing"`
}

type ReactionPayload struct {
	MessageID string `json:"message_id"`
	Emoji     string `json:"emoji"`
}

type Client struct {
	hub      *Hub
	server   Server
	conn     *websocket.Conn
	userID   uuid.UUID
	username string
	roomID   string
	send     chan []byte
}

// Server — интерфейс, чтобы chat не зависел от конкретных handlers
type Server interface {
	HandleIncoming(ctx context.Context, c *Client, msg Incoming)
	OnDisconnect(ctx context.Context, c *Client)
}

func NewClient(hub *Hub, srv Server, conn *websocket.Conn, userID uuid.UUID, username, roomID string) *Client {
	return &Client{
		hub:      hub,
		server:   srv,
		conn:     conn,
		userID:   userID,
		username: username,
		roomID:   roomID,
		send:     make(chan []byte, 128),
	}
}

func (c *Client) UserID() uuid.UUID { return c.userID }
func (c *Client) RoomID() string    { return c.roomID }
func (c *Client) Username() string  { return c.username }

func (c *Client) Push(data []byte) {
	select {
	case c.send <- data:
	default:
	}
}

func (c *Client) PushJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.Push(b)
}

func (c *Client) Run(ctx context.Context) {
	c.hub.register(c)
	go c.writePump(ctx)
	c.readPump(ctx)
}

func (c *Client) readPump(ctx context.Context) {
	defer func() {
		c.hub.unregister(c)
		c.server.OnDisconnect(ctx, c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMsgSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg Incoming
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		c.server.HandleIncoming(ctx, c, msg)
	}
}

func (c *Client) writePump(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("ws write: %v", err)
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
