package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"vedam/lms/internal/adapters/grader"
	httpadapter "vedam/lms/internal/adapters/http"
	"vedam/lms/internal/adapters/postgres"
	redisadapter "vedam/lms/internal/adapters/redis"
	"vedam/lms/internal/domain/assessment"
	"vedam/lms/internal/domain/errs"
	syncdomain "vedam/lms/internal/domain/sync"
)

type coding struct {
	tx          *postgres.TxManager
	changes     *postgres.SyncRepo
	cache       *redisadapter.Cache
	client      *redis.Client
	bridge      grader.Bridge
	assignments *postgres.AssignmentRepo
	attempts    *postgres.CodingAttemptRepo
	submissions *postgres.SubmissionRepo
	slots       chan struct{}
}

func (c *coding) History(ctx context.Context, user, id string) ([]assessment.CodingAttempt, error) {
	if _, err := c.assignments.GetPublicByID(ctx, id); err != nil {
		return nil, err
	}
	return c.attempts.History(ctx, user, id, 20)
}
func (c *coding) Run(ctx context.Context, job redisadapter.GradingJob, custom *string, submitting bool) (grader.Result, error) {
	unlock, locked, err := redisadapter.LockJudge(ctx, c.client, job.UserID)
	if err != nil {
		return grader.Result{}, err
	}
	if !locked {
		return grader.Result{}, httpadapter.ErrEvaluationBusy
	}
	defer unlock()

	if submitting {
		raw, err := c.attempts.Result(ctx, job.UserID, job.ID)
		if err == nil {
			var result grader.Result
			err = json.Unmarshal(raw, &result)
			result.AttemptID = job.ID
			return result, err
		}
		if !errors.Is(err, errs.ErrNotFound) {
			return grader.Result{}, err
		}
	}
	assignment, err := c.assignments.GetWithHiddenTests(ctx, job.AssignmentID)
	if err != nil {
		return grader.Result{}, err
	}
	req := grader.Request{Language: job.Language, Source: job.Source, Entrypoint: assignment.Entrypoint}
	if custom != nil && !submitting {
		req.Cases = []grader.Case{{Input: *custom}}
	} else {
		for _, tc := range assignment.AllTestCases {
			if submitting || !tc.Hidden {
				req.Cases = append(req.Cases, grader.Case{Input: tc.Input, Expected: &tc.Expected, Hidden: tc.Hidden})
			}
		}
	}
	if len(req.Cases) == 0 {
		return grader.Result{}, errs.New(errs.ErrValidation, "NO_TESTS", "The teacher must configure test cases before running this assignment.")
	}
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	default:
		return grader.Result{}, httpadapter.ErrRunnerBusy
	}
	gradeCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	result, err := c.bridge.Evaluate(gradeCtx, req)
	if err != nil || !submitting {
		return result, err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return grader.Result{}, err
	}
	if err := c.record(ctx, job, assignment.CourseCode, result, raw); err != nil {
		return grader.Result{}, err
	}
	result.AttemptID = job.ID
	invalidate, cancelInvalidate := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancelInvalidate()
	_ = c.cache.Invalidate(invalidate, job.UserID)
	return result, nil
}

func (c *coding) record(ctx context.Context, job redisadapter.GradingJob, course string, result grader.Result, raw json.RawMessage) error {
	return c.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := c.changes.LockMutations(ctx); err != nil {
			return err
		}
		now := time.Now()
		attempt := &assessment.CodingAttempt{
			ID: job.ID, UserID: job.UserID, AssignmentID: job.AssignmentID, Language: job.Language, Source: job.Source,
			Verdict: result.Verdict, Passed: result.Passed, Total: result.Total, Result: raw, CreatedAt: now,
		}
		if err := c.attempts.Save(ctx, attempt); err != nil {
			return err
		}
		if result.Verdict != "Accepted" {
			return nil
		}
		sub := &assessment.Submission{ID: uuid.NewString(), UserID: job.UserID, AssignmentID: job.AssignmentID, Course: course, Answer: fmt.Sprintf("Language: %s\n\n%s", job.Language, job.Source)}
		if err := c.submissions.Upsert(ctx, sub); err != nil {
			return err
		}
		sub, err := c.submissions.GetByUserAndAssignment(ctx, job.UserID, job.AssignmentID)
		if err != nil {
			return err
		}
		score := fmt.Sprintf("%d/%d", result.Passed, result.Total)
		feedback := fmt.Sprintf("Accepted · %s tests passed (%s).", score, job.Language)
		if err := c.submissions.Grade(ctx, sub.ID, score, feedback); err != nil {
			return err
		}
		sub.Score, sub.Feedback = &score, &feedback
		data, err := json.Marshal(sub)
		if err != nil {
			return err
		}
		user := job.UserID
		seq, err := c.changes.AppendChangelog(ctx, "user:"+user, &user, "submission", sub.ID, "upsert", data)
		if err != nil {
			return err
		}
		event, err := json.Marshal(map[string]any{"userId": user, "entityId": sub.ID, "entityType": "submission", "cursor": seq})
		if err != nil {
			return err
		}
		return c.changes.EnqueueEvent(ctx, syncdomain.OutboxEvent{ID: uuid.NewString(), AggregateID: sub.ID, Topic: "grade.updated", Payload: event, CreatedAt: now})
	})
}
