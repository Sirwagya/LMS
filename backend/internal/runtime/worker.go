package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"vedam/lms/internal/adapters/grader"
	redisadapter "vedam/lms/internal/adapters/redis"
	"vedam/lms/internal/domain/errs"
)

func (rt *Runtime) Work(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Go(func() {
		for ctx.Err() == nil {
			relay, cancel := context.WithTimeout(ctx, 5*time.Second)
			count, err := rt.RelayOutbox(relay)
			cancel()
			if err != nil && ctx.Err() == nil {
				rt.Log.Error("outbox relay failed", "err", err)
			}
			if count < 50 || err != nil {
				pause(ctx, 250*time.Millisecond)
			}
		}
	})
	wg.Go(func() { rt.consumeGrading(ctx, uuid.NewString()) })
	wg.Wait()
}

func pause(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

func (rt *Runtime) RelayOutbox(ctx context.Context) (int, error) {
	count := 0
	err := rt.Tx.WithinTx(ctx, func(ctx context.Context) error {
		events, err := rt.Sync.ClaimUnpublished(ctx, 50)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(events))
		for _, e := range events {
			var metadata struct {
				UserID string `json:"userId"`
			}
			if err := json.Unmarshal(e.Payload, &metadata); err != nil {
				return err
			}
			name := e.Topic
			if name == "cache.invalidate.bootstrap" || name == "cache.invalidate.catalog" {
				name = "sync.available"
			}
			if err := rt.Cache.Invalidate(ctx, metadata.UserID); err != nil {
				return err
			}
			if err := redisadapter.Publish(ctx, rt.Redis, metadata.UserID, redisadapter.Event{ID: e.ID, Type: name, Data: e.Payload}); err != nil {
				return err
			}
			ids = append(ids, e.ID)
		}
		count = len(ids)
		return rt.Sync.MarkPublished(ctx, ids)
	})
	return count, err
}

func (rt *Runtime) consumeGrading(ctx context.Context, consumer string) {
	for ctx.Err() == nil {
		if err := rt.Queue.Initialize(ctx); err != nil {
			if ctx.Err() == nil {
				rt.Log.Error("grading stream unavailable", "err", err)
			}
			pause(ctx, time.Second)
			continue
		}
		if err := rt.readGrading(ctx, consumer); err != nil && ctx.Err() == nil {
			rt.Log.Error("grading job failed", "err", err)
			pause(ctx, time.Second)
		}
	}
}
func (rt *Runtime) readGrading(ctx context.Context, consumer string) error {
	for ctx.Err() == nil {
		message, err := rt.Queue.Next(ctx, consumer)
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return err
		}
		if err := rt.processJob(ctx, message); err != nil {
			rt.Log.WarnContext(ctx, "grading job pending retry", "message_id", message.ID, "err", err)
		}
	}
	return ctx.Err()
}
func (rt *Runtime) processJob(ctx context.Context, message redis.XMessage) error {
	raw, _ := message.Values["job"].(string)
	var job redisadapter.GradingJob
	if json.Unmarshal([]byte(raw), &job) != nil || job.ID == "" || job.UserID == "" {
		return rt.Queue.Drop(ctx, message.ID)
	}
	result, err := rt.Coding.Run(ctx, job, nil, true)
	state := redisadapter.JobState{Status: "complete"}
	var unavailable grader.Unavailable
	switch {
	case err == nil:
		state.Result, err = json.Marshal(result)
		if err != nil {
			return err
		}
	case errors.Is(err, errs.ErrValidation), errors.Is(err, errs.ErrNotFound), errors.As(err, &unavailable):
		state.Status, state.Error = "failed", err.Error()
	default:
		return err
	}
	if err := rt.Queue.Finish(ctx, message.ID, job, state); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]string{"id": job.ID, "status": state.Status})
	if err != nil {
		return err
	}
	if err := redisadapter.Publish(ctx, rt.Redis, job.UserID, redisadapter.Event{ID: job.ID, Type: "grading.completed", Data: data}); err != nil {
		return fmt.Errorf("publish grading completion: %w", err)
	}
	return nil
}
