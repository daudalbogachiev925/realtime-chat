package pubsub

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type Bus struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Bus {
	return &Bus{rdb: rdb}
}

func (b *Bus) Publish(ctx context.Context, channel string, payload []byte) error {
	return b.rdb.Publish(ctx, channel, payload).Err()
}

// Subscribe — pattern subscribe, чтобы ловить chat:room:* без знания id заранее
func (b *Bus) Subscribe(ctx context.Context, pattern string) <-chan []byte {
	out := make(chan []byte, 128)
	sub := b.rdb.PSubscribe(ctx, pattern)
	go func() {
		defer close(out)
		defer sub.Close()
		for msg := range sub.Channel() {
			select {
			case out <- []byte(msg.Payload):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
