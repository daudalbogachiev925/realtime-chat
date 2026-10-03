package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Auth struct {
	secret []byte
	ttl    time.Duration
}

func New(secret string) *Auth {
	return &Auth{secret: []byte(secret), ttl: 7 * 24 * time.Hour}
}

func (a *Auth) Issue(userID uuid.UUID, username string) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID.String(),
		"usr": username,
		"exp": time.Now().Add(a.ttl).Unix(),
		"iat": time.Now().Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(a.secret)
}

func (a *Auth) Verify(tokenStr string) (uuid.UUID, string, error) {
	t, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return a.secret, nil
	})
	if err != nil || !t.Valid {
		return uuid.Nil, "", errors.New("invalid token")
	}
	claims, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, "", errors.New("invalid claims")
	}
	sub, _ := claims["sub"].(string)
	usr, _ := claims["usr"].(string)
	id, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, "", err
	}
	return id, usr, nil
}

// FromRequest — WebSocket не может ставить заголовки, поэтому смотрим ещё и ?token=
func (a *Auth) FromRequest(r *http.Request) (uuid.UUID, string, error) {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return a.Verify(strings.TrimPrefix(h, "Bearer "))
	}
	if q := r.URL.Query().Get("token"); q != "" {
		return a.Verify(q)
	}
	return uuid.Nil, "", errors.New("no token")
}
