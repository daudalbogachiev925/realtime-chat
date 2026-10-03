package chat

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/daudalobogachiev925/realtime-chat/backend/internal/metrics"
	"github.com/daudalobogachiev925/realtime-chat/backend/internal/presence"
	"github.com/daudalobogachiev925/realtime-chat/backend/internal/pubsub"
	"github.com/daudalobogachiev925/realtime-chat/backend/internal/store"
	"github.com/google/uuid"
)

type Service struct {
	Hub      *Hub
	Store    *store.Store
	Bus      *pubsub.Bus
	Presence *presence.Presence
}

func NewService(h *Hub, s *store.Store, b *pubsub.Bus, p *presence.Presence) *Service {
	return &Service{Hub: h, Store: s, Bus: b, Presence: p}
}

func roomChannel(roomID string) string {
	return "chat:room:" + roomID
}

func (s *Service) HandleIncoming(ctx context.Context, c *Client, in Incoming) {
	switch in.Type {
	case "message":
		s.handleMessage(ctx, c, in.Payload)
	case "typing":
		s.handleTyping(ctx, c, in.Payload)
	case "reaction":
		s.handleReaction(ctx, c, in.Payload)
	case "presence_sync":
		s.handlePresenceSync(ctx, c)
	}
}

func (s *Service) handleMessage(ctx context.Context, c *Client, raw json.RawMessage) {
	var p SendMessagePayload
	if err := json.Unmarshal(raw, &p); err != nil || p.Body == "" {
		return
	}

	var parentID *uuid.UUID
	if p.ParentID != nil && *p.ParentID != "" {
		if id, err := uuid.Parse(*p.ParentID); err == nil {
			parentID = &id
		}
	}

	roomID, err := uuid.Parse(c.roomID)
	if err != nil {
		return
	}

	msg, err := s.Store.CreateMessage(ctx, roomID, c.userID, parentID, p.Body)
	if err != nil {
		log.Printf("create message: %v", err)
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"type":    "message",
		"room_id": c.roomID,
		"payload": msg,
	})

	if err := s.Bus.Publish(ctx, roomChannel(c.roomID), payload); err != nil {
		log.Printf("publish: %v", err)
	}
	metrics.MessagesSent.Inc()
}

func (s *Service) handleTyping(ctx context.Context, c *Client, raw json.RawMessage) {
	var p TypingPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"type":    "typing",
		"room_id": c.roomID,
		"payload": map[string]any{
			"user_id":  c.userID.String(),
			"username": c.username,
			"typing":   p.Typing,
		},
	})
	_ = s.Bus.Publish(ctx, roomChannel(c.roomID), payload)
}

func (s *Service) handleReaction(ctx context.Context, c *Client, raw json.RawMessage) {
	var p ReactionPayload
	if err := json.Unmarshal(raw, &p); err != nil || p.MessageID == "" || p.Emoji == "" {
		return
	}
	msgID, err := uuid.Parse(p.MessageID)
	if err != nil {
		return
	}

	_, err = s.Store.Pool().Exec(ctx,
		`INSERT INTO reactions (message_id, user_id, emoji) VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING`,
		msgID, c.userID, p.Emoji,
	)
	if err != nil {
		log.Printf("reaction: %v", err)
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"type":    "reaction",
		"room_id": c.roomID,
		"payload": map[string]any{
			"message_id": p.MessageID,
			"user_id":    c.userID.String(),
			"username":   c.username,
			"emoji":      p.Emoji,
		},
	})
	_ = s.Bus.Publish(ctx, roomChannel(c.roomID), payload)
}

func (s *Service) handlePresenceSync(ctx context.Context, c *Client) {
	users, err := s.Presence.Online(ctx, c.roomID)
	if err != nil {
		return
	}
	c.PushJSON(map[string]any{
		"type":    "presence_sync",
		"room_id": c.roomID,
		"payload": map[string]any{"online": users},
	})
}

func (s *Service) OnDisconnect(ctx context.Context, c *Client) {
	for _, uid := range s.Hub.LocalClients(c.roomID) {
		if uid == c.userID {
			return
		}
	}
	_ = s.Presence.Leave(ctx, c.roomID, c.userID.String())

	payload, _ := json.Marshal(map[string]any{
		"type":    "presence",
		"room_id": c.roomID,
		"payload": map[string]any{
			"user_id":  c.userID.String(),
			"username": c.username,
			"online":   false,
		},
	})
	_ = s.Bus.Publish(ctx, roomChannel(c.roomID), payload)
	metrics.WSConnections.Dec()
}

// RunSubscriber — подписка ноды на все активные комнаты, один раз при старте
func (s *Service) RunSubscriber(ctx context.Context) {
	sub := s.Bus.Subscribe(ctx, "chat:room:*")
	go func() {
		for data := range sub {
			var env struct {
				RoomID string `json:"room_id"`
			}
			if err := json.Unmarshal(data, &env); err != nil || env.RoomID == "" {
				continue
			}
			n := s.Hub.broadcastLocal(env.RoomID, data)
			metrics.PubSubMessages.Inc()
			if n > 0 {
				metrics.MessagesDelivered.Add(float64(n))
			}
		}
	}()
}

// RunPresenceHeartbeat — обновляем presence для всех клиентов ноды
func (s *Service) RunPresenceHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Hub.mu.RLock()
			for roomID, clients := range s.Hub.rooms {
				seen := map[uuid.UUID]struct{}{}
				for c := range clients {
					if _, ok := seen[c.userID]; ok {
						continue
					}
					seen[c.userID] = struct{}{}
					_ = s.Presence.Heartbeat(ctx, roomID, c.userID.String())
				}
			}
			s.Hub.mu.RUnlock()
		}
	}
}

// BroadcastPresence — вход/выход юзера в комнату
func (s *Service) BroadcastPresence(ctx context.Context, c *Client, online bool) {
	payload, _ := json.Marshal(map[string]any{
		"type":    "presence",
		"room_id": c.roomID,
		"payload": map[string]any{
			"user_id":  c.userID.String(),
			"username": c.username,
			"online":   online,
		},
	})
	_ = s.Bus.Publish(ctx, roomChannel(c.roomID), payload)
}
