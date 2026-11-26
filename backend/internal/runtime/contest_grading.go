package runtime

import (
	"context"
	"time"

	"vedam/lms/internal/adapters/grader"
	"vedam/lms/internal/adapters/postgres"
)

func (rt *Runtime) judgeContests(ctx context.Context) {
	repo := postgres.NewContestAttemptRepo(rt.Tx)
	for ctx.Err() == nil {
		found, err := rt.processContestSubmission(ctx, repo)
		if err != nil && ctx.Err() == nil {
			rt.Log.Warn("contest judge queue operation failed")
		}
		if err != nil || !found {
			pause(ctx, 500*time.Millisecond)
		}
	}
}

func (rt *Runtime) processContestSubmission(ctx context.Context, repo *postgres.ContestAttemptRepo) (bool, error) {
	job, found, err := repo.ClaimCode(ctx)
	if err != nil || !found {
		return found, err
	}
	verdict := ""
	if job.Generation <= postgres.ContestJudgeRetries {
		req := grader.Request{Language: job.Language, Source: job.Source, Entrypoint: "stdin"}
		for _, tc := range job.Question.TestCases {
			if tc.Input != nil && tc.Expected != nil {
				req.Cases = append(req.Cases, grader.Case{Input: *tc.Input, Expected: tc.Expected, Hidden: tc.Hidden})
			}
		}
		if len(req.Cases) == len(job.Question.TestCases) && len(req.Cases) > 0 {
			evaluation, cancel := context.WithTimeout(ctx, 25*time.Second)
			result, err := rt.contestJudge.Judge(evaluation, job.ID, req)
			cancel()
			if err == nil {
				verdict = result
			}
		}
	}
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	return true, repo.FinishCode(ctx, job, verdict)
}

func (rt *Runtime) scoreContests(ctx context.Context) {
	repo := postgres.NewContestAttemptRepo(rt.Tx)
	for ctx.Err() == nil {
		computation, cancel := context.WithTimeout(ctx, 5*time.Second)
		found, err := repo.ComputeScore(computation)
		cancel()
		if err != nil && ctx.Err() == nil {
			rt.Log.Warn("contest score calculation failed")
		}
		if err != nil || !found {
			pause(ctx, 500*time.Millisecond)
		}
	}
}
