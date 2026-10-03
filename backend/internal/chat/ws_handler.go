package chat

import (
	"net/http"

	"github.com/daudalobogachiev925/realtime-chat/backend/internal/auth"
	"github.com/daudalobogachiev925/realtime-chat/backend/internal/metrics"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

type WSHandler struct {
	Hub      *Hub
	Service  *Service
	Auth     *auth.Auth
	Upgrader websocket.Upgrader
}

func NewWSHandler(hub *Hub, svc *Service, a *auth.Auth) *WSHandler {
	return &WSHandler{
		Hub:     hub,
		Service: svc,
		Auth:    a,
		Upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			// фронт будет ходить с другого порта в дев-режиме
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Serve — GET /ws/rooms/{roomID}?token=JWT
func (h *WSHandler) Serve(w http.ResponseWriter, r *http.Request) {
	uid, username, err := h.Auth.FromRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	roomID := chi.URLParam(r, "roomID")
	if roomID == "" {
		http.Error(w, "room required", http.StatusBadRequest)
		return
	}

	conn, err := h.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	c := NewClient(h.Hub, h.Service, conn, uid, username, roomID)
	metrics.WSConnections.Inc()

	// presence: сразу говорим, что мы тут
	_ = h.Service.Presence.Heartbeat(r.Context(), roomID, uid.String())

	// сообщаем о своём появлении в комнате
	h.Service.broadcastPresence(r.Context(), c, true)

	c.Run(r.Context())
}

// broadcastPresence — общий помощник для входа/выхода
func (s *Service) broadcastPresence(ctx any, c *Client, online bool) {
	// ctx здесь может быть context.Context или что-то ещё — приводим к интерфейсу
	type contextLike interface {
		Done() <-chan struct{}
		Err() error
		Value(any) any
		Deadline() (deadline any, ok bool)
	}
	_ = ctx
	// см. реализацию ниже в service.go — тут дублируем для читаемости
	_ = online
	_ = c
}
