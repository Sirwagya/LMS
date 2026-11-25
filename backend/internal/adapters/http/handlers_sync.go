package httpadapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"vedam/lms/internal/domain/errs"
	syncdomain "vedam/lms/internal/domain/sync"
)

func (a *api) invalidate(ctx context.Context, user string) {
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	if err := a.Cache.Invalidate(cacheCtx, user); err != nil {
		a.Log.WarnContext(ctx, "cache invalidation deferred to outbox", "err", err)
	}
}
func (a *api) mutate(ctx context.Context, user string, fn func(context.Context) error) error {
	err := a.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := a.Sync.LockMutations(ctx); err != nil {
			return err
		}
		return fn(ctx)
	})
	if err == nil {
		a.invalidate(ctx, user)
	}
	return err
}
func (a *api) changed(ctx context.Context, user, entityType, id, eventType string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	scope := "global"
	var target *string
	if user != "" {
		scope = "user:" + user
		target = &user
	}
	seq, err := a.Sync.AppendChangelog(ctx, scope, target, entityType, id, "upsert", raw)
	if err != nil {
		return err
	}
	event, err := json.Marshal(map[string]any{"userId": user, "cursor": seq, "entityType": entityType, "entityId": id})
	if err != nil {
		return err
	}
	return a.Sync.EnqueueEvent(ctx, syncdomain.OutboxEvent{ID: uuid.NewString(), AggregateID: id, Topic: eventType, Payload: event, CreatedAt: time.Now()})
}

type recordedMutation struct {
	Fingerprint string          `json:"fingerprint"`
	Data        json.RawMessage `json:"data"`
}

func hashKey(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
func (a *api) once(ctx context.Context, user, namespace, key string, input any, fn func(context.Context) (any, error)) (json.RawMessage, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	fingerprint := hashKey(string(canonical))
	cacheKey := namespace + ":" + hashKey(key)
	storageKey := hashKey(user + ":" + cacheKey)
	decode := func(raw json.RawMessage) (json.RawMessage, error) {
		var record recordedMutation
		if err := json.Unmarshal(raw, &record); err != nil {
			return nil, err
		}
		if record.Fingerprint != fingerprint {
			return nil, problem(409, "Idempotency key already used for a different request.")
		}
		return record.Data, nil
	}
	if key != "" {
		if raw := a.Cache.ReadIdempotency(ctx, user, cacheKey); raw != nil {
			return decode(raw)
		}
	}
	var data, record json.RawMessage
	err = a.mutate(ctx, user, func(ctx context.Context) error {
		if key != "" {
			raw, found, err := a.Sync.LookupMutation(ctx, storageKey)
			if err != nil {
				return err
			}
			if found {
				record = raw
				data, err = decode(raw)
				return err
			}
		}
		value, err := fn(ctx)
		if err != nil {
			return err
		}
		data, err = json.Marshal(value)
		if err != nil || key == "" {
			return err
		}
		record, err = json.Marshal(recordedMutation{fingerprint, data})
		if err != nil {
			return err
		}
		return a.Sync.SaveMutation(ctx, storageKey, user, record)
	})
	if err == nil && key != "" {
		a.Cache.SaveIdempotency(ctx, user, cacheKey, record)
	}
	return data, err
}

func queryNumber(r *http.Request, key string, min, max, fallback int64) (int64, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < min || n > max {
		return 0, invalid("Invalid " + key + ".")
	}
	return n, nil
}
func (a *api) pull(w http.ResponseWriter, r *http.Request) error {
	cursor, err := queryNumber(r, "cursor", 0, 1<<63-1, 0)
	if err != nil {
		return err
	}
	limit, err := queryNumber(r, "limit", 1, 200, 200)
	if err != nil {
		return err
	}
	list, latest, err := a.Sync.ReadSince(r.Context(), actor(r).UserID, cursor, int(limit)+1)
	if err != nil {
		return err
	}
	more := len(list) > int(limit)
	if more {
		list = list[:limit]
		latest = list[len(list)-1].Seq
	}
	writeJSON(w, 200, map[string]any{"cursor": latest, "hasMore": more, "entries": items(list)})
	return nil
}

type mutation struct {
	Key     string          `json:"idempotencyKey"`
	Type    string          `json:"entityType"`
	ID      string          `json:"entityId"`
	Base    int64           `json:"baseVersion"`
	Payload json.RawMessage `json:"payload"`
}
type outcome struct {
	Key           string `json:"idempotencyKey"`
	Type          string `json:"entityType"`
	ID            string `json:"entityId"`
	Status        string `json:"status"`
	Snapshot      any    `json:"serverPayload"`
	ServerVersion *int64 `json:"serverVersion"`
	Error         string `json:"reason,omitempty"`
}

func (a *api) push(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	if f.err != nil {
		return f.err
	}
	var batch []json.RawMessage
	if json.Unmarshal(f.values["mutations"], &batch) != nil || len(batch) == 0 || len(batch) > 200 {
		return invalid("Provide between 1 and 200 mutations.")
	}
	user := actor(r).UserID
	header, _ := r.Context().Value(mutationKey).(string)
	// Bind a batch key to the exact request before applying its first item. Item
	// transactions remain individually resumable if the connection drops.
	if header != "" {
		if _, err := a.once(r.Context(), user, "sync-batch", header, batch, func(context.Context) (any, error) { return true, nil }); err != nil {
			return err
		}
	}
	results := make([]outcome, 0, len(batch))
	for _, item := range batch {
		var m mutation
		if err := json.Unmarshal(item, &m); err != nil {
			results = append(results, outcome{Key: m.Key, Type: m.Type, ID: m.ID, Status: "rejected", Error: "Invalid mutation."})
			continue
		}
		result := outcome{Key: m.Key, Type: m.Type, ID: m.ID, Status: "rejected"}
		if strings.TrimSpace(m.Key) == "" || len(m.Key) > 200 || m.Base < 0 {
			result.Error = "Invalid mutation key or base version."
		} else {
			raw, err := a.once(r.Context(), user, "sync-item", m.Key, m, func(ctx context.Context) (any, error) {
				err := a.applyMutation(ctx, user, m, &result)
				return result, err
			})
			if err != nil {
				var failure httpError
				if !errors.As(err, &failure) || failure.status >= 500 {
					return err
				}
				result.Status = "rejected"
				result.Error = err.Error()
			} else if err = json.Unmarshal(raw, &result); err != nil {
				return err
			}
		}
		results = append(results, result)
	}
	writeJSON(w, 200, map[string]any{"results": results})
	return nil
}
func (a *api) applyMutation(ctx context.Context, user string, m mutation, result *outcome) error {
	switch m.Type {
	case "profile":
		if m.ID != "" && m.ID != user {
			result.Error = "Profile access denied."
			return nil
		}
		row, err := a.Users.GetByID(ctx, user)
		if err != nil {
			return err
		}
		result.Snapshot = map[string]any{"id": user, "profile": row.Public().Profile, "version": row.Version}
		result.ServerVersion = &row.Version
		if row.Version != m.Base {
			result.Status = "conflict"
			return nil
		}
		raw := m.Payload
		var wrapped struct {
			Profile json.RawMessage `json:"profile"`
		}
		if json.Unmarshal(raw, &wrapped) == nil && wrapped.Profile != nil {
			raw = wrapped.Profile
		}
		name, err := validateProfile(raw)
		if err != nil {
			result.Error = err.Error()
			return nil
		}
		version, err := a.updateProfile(ctx, user, name, raw, &m.Base)
		if err != nil {
			return err
		}
		result.Status = "applied"
		result.ServerVersion = &version
		result.Snapshot = map[string]any{"id": user, "profile": raw, "version": version}
	case "lecture_progress", "progress":
		var payload struct {
			Complete        *bool  `json:"complete"`
			LectureID       int64  `json:"lectureId"`
			ClientTimestamp *int64 `json:"clientTimestamp"`
		}
		if json.Unmarshal(m.Payload, &payload) != nil || payload.Complete == nil {
			result.Error = "Completion required."
			return nil
		}
		id := intID(m.ID)
		if id == 0 {
			id = payload.LectureID
		}
		if _, err := a.Catalog.GetLecture(ctx, id); err != nil {
			if errors.Is(err, errs.ErrNotFound) {
				result.Error = "Lecture not found."
				return nil
			}
			return err
		}
		version, completed, latest, err := a.Progress.Version(ctx, user, id)
		if err != nil {
			return err
		}
		now := time.Now()
		timestamp := now.UnixMilli()
		if payload.ClientTimestamp != nil {
			timestamp = *payload.ClientTimestamp
			if timestamp <= 0 || timestamp > now.Add(time.Minute).UnixMilli() {
				result.Error = "Invalid client timestamp."
				return nil
			}
		}
		if timestamp >= latest {
			if err := a.Learning.SetProgress(ctx, user, id, *payload.Complete, time.UnixMilli(timestamp)); err != nil {
				return err
			}
			version, completed, _, err = a.Progress.Version(ctx, user, id)
			if err != nil {
				return err
			}
		}
		result.Status = "applied"
		result.ServerVersion = &version
		result.Snapshot = map[string]any{"lectureId": id, "complete": completed, "version": version}

	case "discussion":
		var payload struct {
			LectureID int64  `json:"lectureId"`
			Body      string `json:"body"`
		}
		if json.Unmarshal(m.Payload, &payload) != nil || strings.TrimSpace(payload.Body) == "" || textLength(payload.Body) > 2000 {
			result.Error = "Valid message required (max 2000 characters)."
			return nil
		}
		if _, err := a.Catalog.GetLecture(ctx, payload.LectureID); err != nil {
			if errors.Is(err, errs.ErrNotFound) {
				result.Error = "Lecture not found."
				return nil
			}
			return err
		}
		snapshot, err := a.addDiscussion(ctx, user, payload.LectureID, payload.Body)
		if err != nil {
			return err
		}
		result.Status, result.Snapshot = "applied", snapshot
	default:
		result.Error = "Unsupported offline mutation."
	}
	return nil
}
