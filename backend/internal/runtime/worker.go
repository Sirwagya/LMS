package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"vedam/lms/internal/adapters/grader"
	"vedam/lms/internal/adapters/postgres"
	redisadapter "vedam/lms/internal/adapters/redis"
	"vedam/lms/internal/adapters/storage"
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
	for range 2 {
		wg.Go(func() { rt.consumeGrading(ctx, uuid.NewString()) })
	}
	wg.Go(func() { rt.reapSubmissionAssets(ctx) })
	if rt.Config.ContestAttempts {
		wg.Go(func() { rt.expireContestAttempts(ctx) })
		wg.Go(func() { rt.scoreContests(ctx) })
		if rt.contestJudge != nil {
			for range 2 {
				wg.Go(func() { rt.judgeContests(ctx) })
			}
		}
		if rt.Config.ContestIntegrity {
			wg.Go(func() { rt.purgeContestEvidence(ctx) })
		}
	}
	wg.Wait()
}

func (rt *Runtime) expireContestAttempts(ctx context.Context) {
	const batch = 50
	repo := postgres.NewContestAttemptRepo(rt.Tx)
	for ctx.Err() == nil {
		sweep, cancel := context.WithTimeout(ctx, 5*time.Second)
		count, err := repo.Expire(sweep, batch)
		cancel()
		if err != nil && ctx.Err() == nil {
			rt.Log.Warn("contest attempt expiration failed", "err", err)
		}
		if err != nil || count < batch {
			pause(ctx, 2*time.Second)
		}
	}
}

func (rt *Runtime) purgeContestEvidence(ctx context.Context) {
	repo := postgres.NewContestIntegrityRepo(rt.Tx)
	for ctx.Err() == nil {
		sweep, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := repo.PurgeExpiredEvidence(sweep)
		cancel()
		if err != nil && ctx.Err() == nil {
			rt.Log.Warn("contest evidence retention sweep failed", "err", err)
		}
		pause(ctx, 15*time.Minute)
	}
}

func pause(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

func (rt *Runtime) reapSubmissionAssets(ctx context.Context) {
	repo := postgres.NewAssetMetaRepo(rt.Tx)
	for ctx.Err() == nil {
		assets, err := storage.NewLocalAssets(rt.Config.Assets)
		if err == nil {
			cleanup, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = repo.ReapSubmissionAssets(cleanup, func(key string) error {
				err := assets.Remove(key)
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			})
			cancel()
			_ = assets.Close()
		}
		if err != nil && ctx.Err() == nil {
			rt.Log.Warn("submission asset cleanup failed", "err", err)
		}
		pause(ctx, time.Minute)
	}
}

func (rt *Runtime) RelayOutbox(ctx context.Context) (int, error) {
	count := 0
	for range 50 {
		found := false
		err := rt.Tx.WithinTx(ctx, func(ctx context.Context) error {
			events, err := rt.Sync.ClaimUnpublished(ctx, 1)
			if err != nil || len(events) == 0 {
				return err
			}
			found = true
			e := events[0]
			var metadata struct {
				UserID string `json:"userId"`
			}
			if err := json.Unmarshal(e.Payload, &metadata); err != nil {
				rt.Log.Warn("outbox event quarantined", "id", e.ID)
				return rt.Sync.DeferRelay(ctx, e.ID, "Invalid event metadata", true)
			}
			name := e.Topic
			if name == "cache.invalidate.bootstrap" || name == "cache.invalidate.catalog" {
				name = "sync.available"
			}
			delivery, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
			defer cancel()
			err = rt.Cache.Invalidate(delivery, metadata.UserID)
			if err == nil {
				err = redisadapter.Publish(delivery, rt.Redis, metadata.UserID, redisadapter.Event{ID: e.ID, Type: name, Data: e.Payload})
			}
			if err != nil {
				return rt.Sync.DeferRelay(ctx, e.ID, "Redis delivery failed", false)
			}
			if err := rt.Sync.MarkPublished(ctx, []string{e.ID}); err != nil {
				return err
			}
			count++
			return nil
		})
		if err != nil || !found {
			return count, err
		}
	}
	return count, nil
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
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, context.DeadlineExceeded):
		state.Status, state.Error = "failed", "Grading timed out."
	default:
		attempts, countErr := rt.Queue.Deliveries(ctx, message.ID)
		if countErr != nil {
			return countErr
		}
		if attempts < 3 {
			return err
		}
		state.Status, state.Error = "failed", "Grading failed after several attempts."
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
