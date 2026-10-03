package presence

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const ttl = 15 * time.Second

type Presence struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Presence {
	return &Presence{rdb: rdb}
}

func (p *Presence) key(roomID string) string {
	return "presence:room:" + roomID
}

func (p *Presence) Heartbeat(ctx context.Context, roomID, userID string) error {
	pipe := p.rdb.Pipeline()
	pipe.HSet(ctx, p.key(roomID), userID, time.Now().Unix())
	pipe.Expire(ctx, p.key(roomID), ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (p *Presence) Leave(ctx context.Context, roomID, userID string) error {
	return p.rdb.HDel(ctx, p.key(roomID), userID).Err()
}

func (p *Presence) Online(ctx context.Context, roomID string) ([]string, error) {
	return p.rdb.HKeys(ctx, p.key(roomID)).Result()
}
