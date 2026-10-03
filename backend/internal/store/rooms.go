package store

import (
	"context"

	"github.com/google/uuid"
)

type Room struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

func (s *Store) ListRooms(ctx context.Context) ([]Room, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name FROM rooms ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []Room
	for rows.Next() {
		var r Room
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		rooms = append(rooms, r)
	}
	return rooms, rows.Err()
}

func (s *Store) CreateRoom(ctx context.Context, name string, createdBy uuid.UUID) (*Room, error) {
	var r Room
	err := s.pool.QueryRow(ctx,
		`INSERT INTO rooms (name, created_by) VALUES ($1, $2) RETURNING id, name`,
		name, createdBy,
	).Scan(&r.ID, &r.Name)
	if err != nil {
		return nil, err
	}
	return &r, nil
}
