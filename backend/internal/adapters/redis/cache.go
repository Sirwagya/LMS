package redisadapter

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const CatalogKey = "cache:catalog:v1"

func BootstrapKey(user string) string { return "cache:bootstrap:" + user + ":v1" }

type Cache struct {
	client *redis.Client
	group  singleflight.Group
}

func NewCache(client *redis.Client) *Cache { return &Cache{client: client} }

// A generation changes whenever the worker invalidates a projection. A reader
// already building an old generation can only populate its old, expiring key.
func (c *Cache) Load(ctx context.Context, key string, generations []string, ttl time.Duration, load func(context.Context) (json.RawMessage, error)) (json.RawMessage, error) {
	keys := make([]string, len(generations))
	for i, g := range generations {
		keys[i] = g + ":generation"
	}
	versions, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return load(ctx)
	}
	parts := []string{key}
	for _, v := range versions {
		version, _ := v.(string)
		parts = append(parts, version)
	}
	versioned := strings.Join(parts, ":")
	if raw, err := c.client.Get(ctx, versioned).Bytes(); err == nil {
		return raw, nil
	}
	ch := c.group.DoChan(versioned, func() (any, error) {
		work, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if raw, err := c.client.Get(work, versioned).Bytes(); err == nil {
			return json.RawMessage(raw), nil
		}
		raw, err := load(work)
		if err == nil {
			_ = c.client.Set(work, versioned, []byte(raw), ttl).Err()
		}
		return raw, err
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-ch:
		if result.Err != nil {
			return nil, result.Err
		}
		return result.Val.(json.RawMessage), nil
	}
}
func (c *Cache) Invalidate(ctx context.Context, user string) error {
	key := CatalogKey
	if user != "" {
		key = BootstrapKey(user)
	}
	return c.client.Set(ctx, key+":generation", uuid.NewString(), 24*time.Hour).Err()
}

func (c *Cache) ReadIdempotency(ctx context.Context, user, key string) json.RawMessage {
	raw, err := c.client.Get(ctx, "idem:"+user+":"+key).Bytes()
	if err != nil {
		return nil
	}
	return raw
}
func (c *Cache) SaveIdempotency(ctx context.Context, user, key string, raw json.RawMessage) {
	_ = c.client.Set(ctx, "idem:"+user+":"+key, []byte(raw), 24*time.Hour).Err()
}
