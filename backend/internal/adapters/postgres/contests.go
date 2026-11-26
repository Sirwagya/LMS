package postgres

import (
	"context"
	"encoding/json"

	"vedam/lms/internal/domain/contest"
	"vedam/lms/internal/domain/errs"
)

type ContestAuthoringRepo struct{ tx *TxManager }

func NewContestAuthoringRepo(tx *TxManager) *ContestAuthoringRepo {
	return &ContestAuthoringRepo{tx: tx}
}

func (r *ContestAuthoringRepo) ListStaff(ctx context.Context, actor contest.Actor, limit, offset int) ([]contest.Summary, error) {
	return r.list(ctx, `
		SELECT id::text,title,opens_at,closes_at,duration_minutes,status,''
		FROM assessments WHERE (owner_id=$1 OR $2::boolean)
		ORDER BY opens_at DESC,id DESC LIMIT $3 OFFSET $4
	`, actor.ID, actor.Admin, limit, offset)
}

func (r *ContestAuthoringRepo) ListStudent(ctx context.Context, studentID string, limit, offset int) ([]contest.Summary, error) {
	return r.list(ctx, `
		SELECT a.id::text,a.title,a.opens_at,a.closes_at,a.duration_minutes,a.status,
   CASE
    WHEN p.id IS NULL THEN ''
    WHEN p.submitted_at IS NOT NULL THEN 'submitted'
    WHEN a.cancelled_at < p.deadline_at THEN 'cancelled'
    WHEN p.deadline_at <= statement_timestamp() THEN 'submitted'
    ELSE 'active'
   END
		FROM assessment_roster r JOIN assessments a ON a.id=r.assessment_id
  LEFT JOIN assessment_attempts p ON p.assessment_id=r.assessment_id AND p.student_id=r.student_id
		WHERE r.student_id=$1 AND a.published_at IS NOT NULL
		ORDER BY a.opens_at DESC,a.id DESC LIMIT $2 OFFSET $3
	`, studentID, limit, offset)
}

func (r *ContestAuthoringRepo) list(ctx context.Context, sql string, args ...any) ([]contest.Summary, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	out := make([]contest.Summary, 0)
	for rows.Next() {
		var item contest.Summary
		if err := rows.Scan(&item.ID, &item.Title, &item.OpensAt, &item.ClosesAt, &item.DurationMinutes, &item.Status, &item.AttemptState); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *ContestAuthoringRepo) GetStaff(ctx context.Context, actor contest.Actor, id string) (contest.Record, error) {
	var record contest.Record
	var questions []byte
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT id::text,title,instructions,opens_at,closes_at,duration_minutes,questions,revision,status,
		       COALESCE(cancellation_reason,''),
		       ARRAY(SELECT student_id FROM assessment_roster WHERE assessment_id=a.id ORDER BY student_id)
		FROM assessments a WHERE id=$1::uuid AND (owner_id=$2 OR $3::boolean)
	`, id, actor.ID, actor.Admin).Scan(&record.ID, &record.Title, &record.Instructions, &record.OpensAt, &record.ClosesAt,
		&record.DurationMinutes, &questions, &record.Revision, &record.Status, &record.CancellationReason, &record.StudentIDs)
	if err != nil {
		return record, mapPGError(err)
	}
	return record, json.Unmarshal(questions, &record.Questions)
}

func (r *ContestAuthoringRepo) Create(ctx context.Context, actor contest.Actor, id string, d contest.Draft) error {
	questions, err := json.Marshal(nonNilQuestions(d.Questions))
	if err != nil {
		return err
	}
	_, err = r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO assessments(id,owner_id,title,instructions,opens_at,closes_at,duration_minutes,questions)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8)
	`, id, actor.ID, d.Title, d.Instructions, d.OpensAt, d.ClosesAt, d.DurationMinutes, questions)
	if err != nil {
		return mapPGError(err)
	}
	return r.replaceRoster(ctx, id, d.StudentIDs)
}

func (r *ContestAuthoringRepo) Update(ctx context.Context, actor contest.Actor, id string, revision int64, d contest.Draft) error {
	questions, err := json.Marshal(nonNilQuestions(d.Questions))
	if err != nil {
		return err
	}
	tag, err := r.tx.Querier(ctx).Exec(ctx, `
		UPDATE assessments SET title=$5,instructions=$6,opens_at=$7,closes_at=$8,duration_minutes=$9,
		       questions=$10, revision=revision+1
		WHERE id=$1::uuid AND (owner_id=$2 OR $3::boolean) AND revision=$4 AND status='draft'
	`, id, actor.ID, actor.Admin, revision, d.Title, d.Instructions, d.OpensAt, d.ClosesAt, d.DurationMinutes, questions)
	if err != nil {
		return mapPGError(err)
	}
	if tag.RowsAffected() != 1 {
		return r.conflict(ctx, actor, id)
	}
	return r.replaceRoster(ctx, id, d.StudentIDs)
}

func nonNilQuestions(questions []contest.Question) []contest.Question {
	if questions == nil {
		return []contest.Question{}
	}
	return questions
}

func (r *ContestAuthoringRepo) replaceRoster(ctx context.Context, id string, students []string) error {
	q := r.tx.Querier(ctx)
	if _, err := q.Exec(ctx, `DELETE FROM assessment_roster WHERE assessment_id=$1::uuid`, id); err != nil {
		return mapPGError(err)
	}
	if len(students) == 0 {
		return nil
	}
	tag, err := q.Exec(ctx, `
		INSERT INTO assessment_roster(assessment_id,student_id)
		SELECT $1::uuid,id FROM users WHERE id=ANY($2::text[]) AND role='student'
	`, id, students)
	if err != nil {
		return mapPGError(err)
	}
	if tag.RowsAffected() != int64(len(students)) {
		return contest.Invalid("The roster may contain only existing student accounts.")
	}
	return nil
}

func (r *ContestAuthoringRepo) Publish(ctx context.Context, actor contest.Actor, id string, revision int64) error {
	tag, err := r.tx.Querier(ctx).Exec(ctx, `
		UPDATE assessments SET status='published', published_at=clock_timestamp(), revision=revision+1
		WHERE id=$1::uuid AND (owner_id=$2 OR $3::boolean) AND revision=$4 AND status='draft' AND opens_at>clock_timestamp()
	`, id, actor.ID, actor.Admin, revision)
	if err != nil {
		return mapPGError(err)
	}
	if tag.RowsAffected() != 1 {
		return r.conflict(ctx, actor, id)
	}
	return nil
}

func (r *ContestAuthoringRepo) Cancel(ctx context.Context, actor contest.Actor, id string, revision int64, reason string) error {
	tag, err := r.tx.Querier(ctx).Exec(ctx, `
		UPDATE assessments SET status='cancelled', cancelled_at=clock_timestamp(), cancelled_by=$2,
		       cancellation_reason=$5, revision=revision+1
		WHERE id=$1::uuid AND (owner_id=$2 OR $3::boolean) AND revision=$4 AND status IN ('draft','published')
	`, id, actor.ID, actor.Admin, revision, reason)
	if err != nil {
		return mapPGError(err)
	}
	if tag.RowsAffected() != 1 {
		return r.conflict(ctx, actor, id)
	}
	return nil
}

func (r *ContestAuthoringRepo) conflict(ctx context.Context, actor contest.Actor, id string) error {
	if _, err := r.GetStaff(ctx, actor, id); err != nil {
		return err
	}
	return errs.New(errs.ErrConflict, "CONTEST_REVISION", "The contest changed or can no longer be edited. Reload it and try again.")
}
