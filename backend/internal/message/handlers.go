package message

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/daudalobogachiev925/realtime-chat/backend/internal/auth"
	"github.com/daudalobogachiev925/realtime-chat/backend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handlers struct {
	Store *store.Store
	Auth  *auth.Auth
}

func NewHandlers(s *store.Store, a *auth.Auth) *Handlers {
	return &Handlers{Store: s, Auth: a}
}

// List — GET /api/rooms/{roomID}/messages?before=RFC3339&limit=50
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.Auth.FromRequest(r); err != nil {
		httpError(w, 401, "unauthorized")
		return
	}

	roomID, err := uuid.Parse(chi.URLParam(r, "roomID"))
	if err != nil {
		httpError(w, 400, "invalid room id")
		return
	}

	var before *time.Time
	if b := r.URL.Query().Get("before"); b != "" {
		if t, err := time.Parse(time.RFC3339Nano, b); err == nil {
			before = &t
		}
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 50
	}

	msgs, err := h.Store.ListMessages(r.Context(), roomID, before, limit)
	if err != nil {
		httpError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, msgs)
}

// Replies — GET /api/messages/{messageID}/replies
func (h *Handlers) Replies(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.Auth.FromRequest(r); err != nil {
		httpError(w, 401, "unauthorized")
		return
	}

	msgID, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpError(w, 400, "invalid message id")
		return
	}

	msgs, err := h.Store.ListReplies(r.Context(), msgID)
	if err != nil {
		httpError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, msgs)
}

// Reactions — GET /api/messages/{messageID}/reactions
func (h *Handlers) Reactions(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.Auth.FromRequest(r); err != nil {
		httpError(w, 401, "unauthorized")
		return
	}

	msgID, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpError(w, 400, "invalid message id")
		return
	}

	rows, err := h.Store.Pool().Query(r.Context(),
		`SELECT r.emoji, u.username, r.user_id
		 FROM reactions r JOIN users u ON u.id = r.user_id
		 WHERE r.message_id = $1`, msgID)
	if err != nil {
		httpError(w, 500, err.Error())
		return
	}
	defer rows.Close()

	type reaction struct {
		Emoji    string `json:"emoji"`
		Username string `json:"username"`
		UserID   string `json:"user_id"`
	}
	var out []reaction
	for rows.Next() {
		var rec reaction
		var uid uuid.UUID
		if err := rows.Scan(&rec.Emoji, &rec.Username, &uid); err != nil {
			continue
		}
		rec.UserID = uid.String()
		out = append(out, rec)
	}
	writeJSON(w, 200, out)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
