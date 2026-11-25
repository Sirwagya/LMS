package redisadapter

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

func EventChannel(user string) string {
	if user == "" {
		return "pubsub:global"
	}
	return "pubsub:user:" + user
}

type Event struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func Publish(ctx context.Context, client *redis.Client, user string, event Event) error {
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return client.Publish(ctx, EventChannel(user), raw).Err()
}
