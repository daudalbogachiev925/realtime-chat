package store

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type Message struct {
	ID        uuid.UUID  `json:"id"`
	RoomID    uuid.UUID  `json:"room_id"`
	UserID    uuid.UUID  `json:"user_id"`
	Username  string     `json:"username"`
	ParentID  *uuid.UUID `json:"parent_id,omitempty"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
}

func (s *Store) CreateMessage(ctx context.Context, roomID, userID uuid.UUID, parentID *uuid.UUID, body string) (*Message, error) {
	var m Message
	err := s.pool.QueryRow(ctx,
		`WITH ins AS (
		   INSERT INTO messages (room_id, user_id, parent_id, body)
		   VALUES ($1, $2, $3, $4)
		   RETURNING id, room_id, user_id, parent_id, body, created_at
		 )
		 SELECT ins.id, ins.room_id, ins.user_id, ins.parent_id, ins.body, ins.created_at, u.username
		 FROM ins JOIN users u ON u.id = ins.user_id`,
		roomID, userID, parentID, body,
	).Scan(&m.ID, &m.RoomID, &m.UserID, &m.ParentID, &m.Body, &m.CreatedAt, &m.Username)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListMessages — курсорная пагинация: before = время последнего загруженного
func (s *Store) ListMessages(ctx context.Context, roomID uuid.UUID, before *time.Time, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}

	q := `SELECT m.id, m.room_id, m.user_id, m.parent_id, m.body, m.created_at, u.username
	      FROM messages m JOIN users u ON u.id = m.user_id
	      WHERE m.room_id = $1 AND m.parent_id IS NULL`
	args := []any{roomID}
	if before != nil {
		q += ` AND m.created_at < $2`
		args = append(args, *before)
	}
	q += ` ORDER BY m.created_at DESC LIMIT ` + strconv.Itoa(limit)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.RoomID, &m.UserID, &m.ParentID, &m.Body, &m.CreatedAt, &m.Username); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListReplies — ответы в треде
func (s *Store) ListReplies(ctx context.Context, parentID uuid.UUID) ([]Message, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT m.id, m.room_id, m.user_id, m.parent_id, m.body, m.created_at, u.username
		 FROM messages m JOIN users u ON u.id = m.user_id
		 WHERE m.parent_id = $1 ORDER BY m.created_at ASC`,
		parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.RoomID, &m.UserID, &m.ParentID, &m.Body, &m.CreatedAt, &m.Username); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
