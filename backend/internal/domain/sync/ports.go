package sync

import (
	"context"
	"encoding/json"
	"time"
)

type OutboxEvent struct {
	ID          string
	AggregateID string
	Topic       string
	Payload     json.RawMessage
	CreatedAt   time.Time
}

type ChangelogEntry struct {
	Seq        int64           `json:"seq"`
	Scope      string          `json:"scope"`
	EntityType string          `json:"entityType"`
	EntityID   string          `json:"entityId"`
	Op         string          `json:"op"`
	Payload    json.RawMessage `json:"payload"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}

type OutboxWriter interface {
	EnqueueEvent(ctx context.Context, event OutboxEvent) error
	AppendChangelog(ctx context.Context, scope string, targetUserID *string, entityType, entityID, op string, payload json.RawMessage) (int64, error)
}

type OutboxRelayRepository interface {
	ClaimUnpublished(ctx context.Context, limit int) ([]OutboxEvent, error)
	MarkPublished(ctx context.Context, ids []string) error
}

type SyncLogReader interface {
	ReadSince(ctx context.Context, userID string, cursor int64, limit int) (entries []ChangelogEntry, latestSeq int64, err error)
}

type IdempotencyStore interface {
	LookupMutation(ctx context.Context, key string) (response json.RawMessage, found bool, err error)
	SaveMutation(ctx context.Context, key, userID string, response json.RawMessage) error
}
