package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"vedam/lms/internal/domain/contest"
)

type ContestAttemptRepo struct{ tx *TxManager }

func NewContestAttemptRepo(tx *TxManager) *ContestAttemptRepo {
	return &ContestAttemptRepo{tx: tx}
}

type attemptRow struct {
	id, device, reason string
	lease, revision    int64
	started, deadline  time.Time
	submitted          *time.Time
	answers            map[int]contest.Answer
}

func (r *ContestAttemptRepo) Execute(ctx context.Context, id, student string, cmd contest.AttemptCommand) (contest.AttemptView, error) {
	var view contest.AttemptView
	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		var revision int64
		var cancelledAt *time.Time
		var questionData []byte
		q := r.tx.Querier(ctx)
		err := q.QueryRow(ctx, `
			SELECT a.id::text,a.title,a.instructions,a.opens_at,a.closes_at,
			       a.duration_minutes,a.status,a.revision,a.cancelled_at,a.questions
			FROM assessments a JOIN assessment_roster s ON s.assessment_id=a.id
			WHERE a.id=$1 AND s.student_id=$2 AND a.published_at IS NOT NULL
			FOR SHARE OF a
		`, id, student).Scan(&view.ID, &view.Title, &view.Instructions, &view.OpensAt, &view.ClosesAt,
			&view.DurationMinutes, &view.Status, &revision, &cancelledAt, &questionData)
		if err != nil {
			return mapPGError(err)
		}

		attempt, exists, err := r.lock(ctx, id, student)
		if err != nil {
			return err
		}
		if err := q.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&view.ServerTime); err != nil {
			return err
		}
		view.State = "ready"
		switch {
		case view.Status == "cancelled":
			view.State = "cancelled"
		case view.ServerTime.Before(view.OpensAt):
			view.State = "upcoming"
		case !view.ServerTime.Before(view.ClosesAt):
			view.State = "closed"
		}

		if !exists && cmd.Action == "start" && view.State == "ready" {
			// Recheck in the insert itself and honor the unique roster entry on simultaneous starts.
			if _, err := q.Exec(ctx, `
				WITH admission AS (SELECT clock_timestamp() AS now)
				INSERT INTO assessment_attempts(assessment_id,student_id,contest_revision,device_id,started_at,deadline_at)
				SELECT $1,$2,$3,$4,now,LEAST($6,now+make_interval(mins=>$7))
				FROM admission WHERE now >= $5 AND now < $6
				ON CONFLICT (assessment_id,student_id) DO NOTHING
			`, id, student, revision, cmd.DeviceID, view.OpensAt, view.ClosesAt, view.DurationMinutes); err != nil {
				return mapPGError(err)
			}
			attempt, exists, err = r.lock(ctx, id, student)
			if err != nil {
				return err
			}
			if err := q.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&view.ServerTime); err != nil {
				return err
			}
			if !exists {
				view.State = "closed"
			}
		}
		if !exists {
			return nil
		}

		view.AttemptID, view.StartedAt, view.DeadlineAt = attempt.id, &attempt.started, &attempt.deadline
		if attempt.submitted == nil && cancelledAt != nil && cancelledAt.Before(attempt.deadline) {
			view.State = "cancelled"
			return nil
		}
		if attempt.submitted == nil && !view.ServerTime.Before(attempt.deadline) {
			if _, err := q.Exec(ctx, `
				UPDATE assessment_attempts SET submitted_at=deadline_at,finish_reason='expired' WHERE id=$1
			`, attempt.id); err != nil {
				return mapPGError(err)
			}
			attempt.submitted, attempt.reason = &attempt.deadline, "expired"
		}
		if attempt.submitted != nil {
			attempt.apply(&view)
			return nil
		}

		if cmd.Action == "takeover" && cmd.DeviceID != attempt.device && cmd.Lease == attempt.lease {
			attempt.lease++
			attempt.device = cmd.DeviceID
			if _, err := q.Exec(ctx, `
				WITH changed AS (
					UPDATE assessment_attempts SET device_id=$2,lease=$3 WHERE id=$1 RETURNING id
				)
				INSERT INTO assessment_attempt_takeovers(attempt_id,lease,replaced_at)
				SELECT id,$3,$4 FROM changed
			`, attempt.id, attempt.device, attempt.lease, view.ServerTime); err != nil {
				return mapPGError(err)
			}
		}
		view.Lease = attempt.lease
		writing := cmd.Action == "save" || cmd.Action == "submit"
		if cmd.DeviceID != attempt.device || (writing && cmd.Lease != attempt.lease) {
			view.State = "occupied"
			return nil
		}

		var questions []contest.Question
		if err := json.Unmarshal(questionData, &questions); err != nil {
			return err
		}
		if writing {
			if err := r.write(ctx, &attempt, cmd, questions, view.ServerTime); err != nil {
				return err
			}
		} else {
			view.Questions = contest.StudentQuestions(attempt.id, questions)
		}
		view.State, view.Answers, view.Revision = "active", attempt.answers, attempt.revision
		if attempt.submitted != nil {
			attempt.apply(&view)
		}
		return nil
	})
	return view, err
}

func (a *attemptRow) apply(view *contest.AttemptView) {
	view.State, view.SubmittedAt, view.FinishReason = "submitted", a.submitted, a.reason
	view.Revision, view.Answers = a.revision, a.answers
}

func (r *ContestAttemptRepo) lock(ctx context.Context, assessmentID, student string) (attemptRow, bool, error) {
	var a attemptRow
	var data []byte
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT id::text,device_id::text,lease,answer_revision,started_at,deadline_at,
		       submitted_at,COALESCE(finish_reason,''),answers
		FROM assessment_attempts WHERE assessment_id=$1 AND student_id=$2 FOR UPDATE
	`, assessmentID, student).Scan(&a.id, &a.device, &a.lease, &a.revision, &a.started, &a.deadline,
		&a.submitted, &a.reason, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, false, nil
	}
	if err != nil {
		return a, false, mapPGError(err)
	}
	return a, true, json.Unmarshal(data, &a.answers)
}

func (r *ContestAttemptRepo) write(ctx context.Context, a *attemptRow, cmd contest.AttemptCommand, questions []contest.Question, now time.Time) error {
	if err := contest.ValidateAnswers(questions, cmd.Answers); err != nil {
		return err
	}
	same := contest.EqualAnswers(a.answers, cmd.Answers)
	if cmd.Revision != a.revision && !(cmd.Revision == a.revision-1 && same) {
		return contest.AttemptConflict()
	}
	if cmd.Action == "save" && same {
		return nil
	}
	if !same {
		a.revision++
		a.answers = cmd.Answers
	}
	if cmd.Action == "submit" {
		a.submitted, a.reason = &now, "manual"
	}
	data, err := json.Marshal(a.answers)
	if err != nil {
		return err
	}
	_, err = r.tx.Querier(ctx).Exec(ctx, `
		UPDATE assessment_attempts SET answers=$2,answer_revision=$3,submitted_at=$4,finish_reason=NULLIF($5,'')
		WHERE id=$1
	`, a.id, data, a.revision, a.submitted, a.reason)
	return mapPGError(err)
}

func (r *ContestAttemptRepo) Expire(ctx context.Context, limit int) (int64, error) {
	tag, err := r.tx.Querier(ctx).Exec(ctx, `
		WITH due AS (
			SELECT p.id FROM assessment_attempts p JOIN assessments a ON a.id=p.assessment_id
			WHERE p.submitted_at IS NULL AND p.deadline_at <= statement_timestamp()
			      AND (a.cancelled_at IS NULL OR a.cancelled_at >= p.deadline_at)
			ORDER BY p.deadline_at,p.id LIMIT $1 FOR UPDATE OF p SKIP LOCKED
		)
		UPDATE assessment_attempts p SET submitted_at=p.deadline_at,finish_reason='expired'
		FROM due WHERE p.id=due.id
	`, limit)
	return tag.RowsAffected(), mapPGError(err)
}
