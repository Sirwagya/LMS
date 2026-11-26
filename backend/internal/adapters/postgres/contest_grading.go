package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"vedam/lms/internal/domain/contest"
	"vedam/lms/internal/domain/errs"
)

func (r *ContestAttemptRepo) admitCode(ctx context.Context, id string, cmd contest.CodingCommand, questions []contest.Question, now time.Time) (string, error) {
	if cmd.Question < 0 || cmd.Question >= len(questions) || questions[cmd.Question].Kind != "coding" {
		return "", contest.Invalid("Choose a coding question in this contest.")
	}
	if strings.TrimSpace(cmd.Source) == "" {
		return "", contest.Invalid("Write a solution before submitting it.")
	}
	if err := contest.ValidateAnswers(questions, map[int]contest.Answer{cmd.Question: {Lang: cmd.Language, Source: cmd.Source}}); err != nil {
		return "", err
	}

	q := r.tx.Querier(ctx)
	var existing contest.CodingCommand
	var submissionID string
	err := q.QueryRow(ctx, `
		SELECT id::text,question_index,language,source
		FROM assessment_coding_submissions WHERE attempt_id=$1 AND request_id=$2
	`, id, cmd.RequestID).Scan(&submissionID, &existing.Question, &existing.Language, &existing.Source)
	if err == nil {
		existing.RequestID = cmd.RequestID
		if cmd != existing {
			return "", errs.New(errs.ErrConflict, "CODING_REQUEST_CHANGED", "This submission was already used for different code.")
		}
		return submissionID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", mapPGError(err)
	}
	var total, perQuestion int
	if err := q.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE question_index=$2)
		FROM assessment_coding_submissions WHERE attempt_id=$1
	`, id, cmd.Question).Scan(&total, &perQuestion); err != nil {
		return "", err
	}
	if perQuestion >= 12 || total >= 60 {
		return "", errs.New(errs.ErrRateLimited, "CODING_SUBMISSION_LIMIT", "The official submission limit for this question or contest has been reached.")
	}
	err = q.QueryRow(ctx, `
		INSERT INTO assessment_coding_submissions(attempt_id,request_id,question_index,language,source,submitted_at)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text
	`, id, cmd.RequestID, cmd.Question, cmd.Language, cmd.Source, now).Scan(&submissionID)
	return submissionID, mapPGError(err)
}

func (r *ContestAttemptRepo) addGrading(ctx context.Context, active bool, view *contest.AttemptView, id string) error {
	view.OfficialCoding = active
	submissions, err := r.codingSubmissions(ctx, id)
	if err != nil {
		return err
	}
	view.CodingSubmissions = submissions
	if !active {
		status, err := r.scoreStatus(ctx, id)
		if err != nil {
			return err
		}
		view.GradingStatus = status
	}
	return nil
}

func (r *ContestAttemptRepo) scoreStatus(ctx context.Context, id string) (string, error) {
	var status string
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT CASE status WHEN 'ready' THEN 'awaiting_release' ELSE status END
		FROM assessment_attempt_results WHERE attempt_id=$1 ORDER BY version DESC LIMIT 1
	`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "grading", nil
	}
	return status, mapPGError(err)
}

func (r *ContestAttemptRepo) codingSubmissions(ctx context.Context, id string) ([]contest.CodingSubmission, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT id::text,question_index,language,submitted_at,status
		FROM assessment_coding_submissions WHERE attempt_id=$1 ORDER BY submitted_at DESC,id
	`, id)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var items []contest.CodingSubmission
	for rows.Next() {
		var s contest.CodingSubmission
		if err := rows.Scan(&s.ID, &s.Question, &s.Language, &s.SubmittedAt, &s.Status); err != nil {
			return nil, err
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

type ContestJudgeJob struct {
	ID         string
	Generation int
	Language   string
	Source     string
	Question   contest.Question
}

const ContestJudgeRetries = 3

// Claim time doubles as the expiry time for an in-flight claim. A newer
// generation fences any late response from a worker that lost its claim.
func (r *ContestAttemptRepo) ClaimCode(ctx context.Context) (ContestJudgeJob, bool, error) {
	var job ContestJudgeJob
	var data []byte
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		WITH candidate AS (
			SELECT id FROM assessment_coding_submissions
			WHERE status IN ('queued','judging') AND claim_after <= statement_timestamp()
			ORDER BY claim_after,id LIMIT 1 FOR UPDATE SKIP LOCKED
		), claimed AS (
			UPDATE assessment_coding_submissions s
			SET status='judging', generation=LEAST(generation+1,$1+1),
			    claim_after=statement_timestamp()+interval '60 seconds'
			FROM candidate c WHERE s.id=c.id RETURNING s.*
		)
		SELECT c.id::text,c.generation,c.language,c.source,a.questions->c.question_index
		FROM claimed c JOIN assessment_attempts p ON p.id=c.attempt_id
		JOIN assessments a ON a.id=p.assessment_id
	`, ContestJudgeRetries).Scan(&job.ID, &job.Generation, &job.Language, &job.Source, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return job, false, nil
	}
	if err != nil {
		return job, false, err
	}
	return job, true, json.Unmarshal(data, &job.Question)
}

func (r *ContestAttemptRepo) FinishCode(ctx context.Context, job ContestJudgeJob, verdict string) error {
	status := "rejected"
	switch {
	case verdict == "Accepted":
		status = "accepted"
	case verdict == "" && job.Generation >= ContestJudgeRetries:
		status = "failed"
	case verdict == "":
		status = "queued"
	}
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		UPDATE assessment_coding_submissions
		SET status=$3,verdict=NULLIF($4,''),claim_after=statement_timestamp()+make_interval(secs=>5*generation)
		WHERE id=$1 AND generation=$2 AND status='judging'
	`, job.ID, job.Generation, status, verdict)
	return mapPGError(err)
}

// ComputeScore materializes the first immutable result version once the
// attempt and all official submissions have reached terminal states.
func (r *ContestAttemptRepo) ComputeScore(ctx context.Context) (bool, error) {
	found := false
	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		var id string
		var started, submitted time.Time
		var questionsJSON, answersJSON, savedJSON []byte
		q := r.tx.Querier(ctx)
		err := q.QueryRow(ctx, `
			SELECT p.id::text,p.started_at,p.submitted_at,a.questions,p.answers,p.choice_saved_at
			FROM assessment_attempts p JOIN assessments a ON a.id=p.assessment_id
			WHERE p.scoring_enabled AND p.submitted_at IS NOT NULL AND p.score_computed_at IS NULL
			  AND NOT EXISTS (
			    SELECT FROM assessment_coding_submissions c
			    WHERE c.attempt_id=p.id AND c.status IN ('queued','judging')
			  )
			ORDER BY p.submitted_at,p.id LIMIT 1 FOR UPDATE OF p SKIP LOCKED
		`).Scan(&id, &started, &submitted, &questionsJSON, &answersJSON, &savedJSON)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		var questions []contest.Question
		var answers map[int]contest.Answer
		var saved map[int]time.Time
		if err := json.Unmarshal(questionsJSON, &questions); err != nil {
			return err
		}
		if err := json.Unmarshal(answersJSON, &answers); err != nil {
			return err
		}
		if err := json.Unmarshal(savedJSON, &saved); err != nil {
			return err
		}
		submissions, err := r.codingSubmissions(ctx, id)
		if err != nil {
			return err
		}
		score := contest.ScoreAttempt(id, started, submitted, questions, answers, saved, submissions)
		details, err := json.Marshal(score.Details)
		if err != nil {
			return err
		}
		_, err = q.Exec(ctx, `
			WITH result AS (
				INSERT INTO assessment_attempt_results(attempt_id,version,status,points,possible_points,elapsed_ms,question_scores)
				VALUES ($1,1,$2,$3,$4,$5,$6) RETURNING attempt_id,computed_at
			)
			UPDATE assessment_attempts p SET score_computed_at=result.computed_at FROM result WHERE p.id=result.attempt_id
		`, id, score.Status, score.Points, score.Possible, score.Elapsed, details)
		found = err == nil
		return err
	})
	return found, err
}
