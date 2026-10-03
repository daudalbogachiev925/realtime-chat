package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/daudalobogachiev925/realtime-chat/backend/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type Handlers struct {
	Store *store.Store
	Auth  *Auth
}

func NewHandlers(s *store.Store, a *Auth) *Handlers {
	return &Handlers{Store: s, Auth: a}
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		httpError(w, 400, "invalid json")
		return
	}
	c.Username = strings.TrimSpace(c.Username)
	if len(c.Username) < 3 || len(c.Password) < 6 {
		httpError(w, 400, "username >= 3, password >= 6")
		return
	}

	existing, err := h.Store.FindUserByUsername(r.Context(), c.Username)
	if err != nil {
		httpError(w, 500, err.Error())
		return
	}
	if existing != nil {
		httpError(w, 409, "username taken")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(c.Password), bcrypt.DefaultCost)
	if err != nil {
		httpError(w, 500, err.Error())
		return
	}

	u, err := h.Store.CreateUser(r.Context(), c.Username, string(hash))
	if err != nil {
		httpError(w, 500, err.Error())
		return
	}

	token, err := h.Auth.Issue(u.ID, u.Username)
	if err != nil {
		httpError(w, 500, err.Error())
		return
	}

	writeJSON(w, 201, map[string]any{
		"token": token,
		"user":  map[string]any{"id": u.ID, "username": u.Username},
	})
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		httpError(w, 400, "invalid json")
		return
	}

	u, err := h.Store.FindUserByUsername(r.Context(), c.Username)
	if err != nil {
		httpError(w, 500, err.Error())
		return
	}
	if u == nil {
		httpError(w, 401, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(c.Password)); err != nil {
		httpError(w, 401, "invalid credentials")
		return
	}

	token, err := h.Auth.Issue(u.ID, u.Username)
	if err != nil {
		httpError(w, 500, err.Error())
		return
	}

	writeJSON(w, 200, map[string]any{
		"token": token,
		"user":  map[string]any{"id": u.ID, "username": u.Username},
	})
}

func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	uid, username, err := h.Auth.FromRequest(r)
	if err != nil {
		httpError(w, 401, "unauthorized")
		return
	}
	writeJSON(w, 200, map[string]any{
		"id":       uid,
		"username": username,
	})
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
