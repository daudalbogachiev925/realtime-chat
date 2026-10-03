package room

import (
	"encoding/json"
	"net/http"

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

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.Auth.FromRequest(r); err != nil {
		httpError(w, 401, "unauthorized")
		return
	}
	rooms, err := h.Store.ListRooms(r.Context())
	if err != nil {
		httpError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, rooms)
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	uid, _, err := h.Auth.FromRequest(r)
	if err != nil {
		httpError(w, 401, "unauthorized")
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpError(w, 400, "invalid json")
		return
	}
	if len(body.Name) < 2 {
		httpError(w, 400, "name too short")
		return
	}

	room, err := h.Store.CreateRoom(r.Context(), body.Name, uid)
	if err != nil {
		httpError(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, room)
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

// GetID — маленький хелпер для читаемости
func GetID(r *http.Request, key string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, key))
}
