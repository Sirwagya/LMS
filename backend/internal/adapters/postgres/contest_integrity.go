package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"vedam/lms/internal/domain/contest"
	"vedam/lms/internal/domain/errs"
)

type ContestIntegrityRepo struct {
	tx *TxManager
}

func NewContestIntegrityRepo(tx *TxManager) *ContestIntegrityRepo {
	return &ContestIntegrityRepo{tx: tx}
}

var _ contest.IntegrityRepository = (*ContestIntegrityRepo)(nil)

func parseCanonicalUUID(raw string) (string, bool) {
	u, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	return u.String(), true
}

func requireReviewerAccess(ctx context.Context, q Querier, actor contest.Actor, assessmentID string) error {
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return errs.ErrNotFound
	}
	var allowed bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM assessments a
			WHERE a.id = $1::uuid
			  AND (
			      $3::boolean
			      OR a.owner_id = $2
			      OR EXISTS (
			          SELECT 1 FROM assessment_reviewers r
			          WHERE r.assessment_id = a.id AND r.reviewer_id = $2
			      )
			  )
		)
	`, aid, actor.ID, actor.Admin).Scan(&allowed)
	if err != nil {
		return mapPGError(err)
	}
	if !allowed {
		return errs.ErrNotFound
	}
	return nil
}

func (r *ContestIntegrityRepo) IngestEvents(ctx context.Context, assessmentID, studentID string, batch contest.EventBatchInput) (contest.EventBatchResult, error) {
	var result contest.EventBatchResult
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return result, errs.ErrNotFound
	}
	if err := contest.ValidateEventBatch(batch); err != nil {
		return result, err
	}
	attemptID, _ := parseCanonicalUUID(batch.AttemptID)
	deviceID, _ := parseCanonicalUUID(batch.DeviceID)
	batchID, _ := parseCanonicalUUID(batch.BatchID)

	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		var lockedDevice string
		var submittedAt *time.Time
		var now time.Time
		err := q.QueryRow(ctx, `
			SELECT device_id::text, submitted_at, clock_timestamp()
			FROM assessment_attempts
			WHERE id = $1::uuid AND assessment_id = $2::uuid AND student_id = $3
			FOR UPDATE
		`, attemptID, aid, studentID).Scan(&lockedDevice, &submittedAt, &now)
		if err != nil {
			return mapPGError(err)
		}
		if lockedDevice != deviceID {
			return errs.New(errs.ErrConflict, "ATTEMPT_DEVICE_MISMATCH", "This device session is no longer active for the attempt.")
		}
		if submittedAt != nil && now.After(submittedAt.Add(120*time.Second)) {
			return errs.New(errs.ErrConflict, "ATTEMPT_CLOSED", "The event ingestion window for this submitted attempt has closed.")
		}

		var alreadyProcessed bool
		if err := q.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM assessment_event_batches
				WHERE attempt_id = $1::uuid AND batch_id = $2::uuid
			)
		`, attemptID, batchID).Scan(&alreadyProcessed); err != nil {
			return mapPGError(err)
		}
		if alreadyProcessed {
			seqs, err := listClientSeqs(ctx, q, attemptID)
			if err != nil {
				return err
			}
			highest, missing := contest.MissingSeqs(seqs)
			result = contest.EventBatchResult{
				Accepted:    0,
				Duplicate:   true,
				HighestSeq:  highest,
				MissingSeqs: missing,
			}
			return nil
		}

		var existingCount int
		if err := q.QueryRow(ctx, `
			SELECT count(*) FROM assessment_attempt_events
			WHERE attempt_id = $1::uuid AND source_kind = 'client'
		`, attemptID).Scan(&existingCount); err != nil {
			return mapPGError(err)
		}
		if existingCount+len(batch.Events) > contest.MaxAttemptClientEvents {
			return errs.New(errs.ErrRateLimited, "CONTEST_EVENT_QUOTA", "Client event quota exceeded for this attempt.")
		}

		startSeq, endSeq := batch.Events[0].Seq, batch.Events[0].Seq
		seqs := make([]int64, len(batch.Events))
		eventTypes := make([]string, len(batch.Events))
		qIndexes := make([]*int, len(batch.Events))
		clientAts := make([]*time.Time, len(batch.Events))
		payloads := make([]string, len(batch.Events))

		for i, ev := range batch.Events {
			if ev.Seq < startSeq {
				startSeq = ev.Seq
			}
			if ev.Seq > endSeq {
				endSeq = ev.Seq
			}
			seqs[i] = ev.Seq
			eventTypes[i] = ev.EventType
			qIndexes[i] = ev.QuestionIndex
			clientAts[i] = ev.ClientAt
			payload := ev.Payload
			if payload == nil {
				payload = map[string]any{}
			}
			rawPayload, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			payloads[i] = string(rawPayload)
		}

		if _, err := q.Exec(ctx, `
			INSERT INTO assessment_event_batches (attempt_id, batch_id, start_seq, end_seq)
			VALUES ($1::uuid, $2::uuid, $3, $4)
		`, attemptID, batchID, startSeq, endSeq); err != nil {
			return mapPGError(err)
		}

		tag, err := q.Exec(ctx, `
			INSERT INTO assessment_attempt_events (
				attempt_id, seq, batch_id, source_kind, event_type, question_index, client_at, payload
			)
			SELECT $1::uuid, u.seq, $2::uuid, 'client', u.event_type, u.question_index, u.client_at, u.payload::jsonb
			FROM unnest($3::bigint[], $4::text[], $5::int[], $6::timestamptz[], $7::text[])
				AS u(seq, event_type, question_index, client_at, payload)
			ON CONFLICT (attempt_id, seq) DO NOTHING
		`, attemptID, batchID, seqs, eventTypes, qIndexes, clientAts, payloads)
		if err != nil {
			return mapPGError(err)
		}

		signals := contest.DetectBatchSignals(batch.Events)
		var blurCount, takeoverCount int
		if err := q.QueryRow(ctx, `
			SELECT
				(SELECT count(*) FROM assessment_attempt_events WHERE attempt_id = $1::uuid AND source_kind = 'client' AND event_type = 'window.blur'),
				(SELECT count(*) FROM assessment_attempt_takeovers WHERE attempt_id = $1::uuid)
		`, attemptID).Scan(&blurCount, &takeoverCount); err != nil {
			return mapPGError(err)
		}
		if blurCount >= 3 {
			signals = append(signals, contest.ReviewSignal{
				Kind:     "window_blur",
				Summary:  fmt.Sprintf("Repeated window focus losses (%d)", blurCount),
				Severity: "medium",
			})
		}
		if takeoverCount > 0 {
			signals = append(signals, contest.ReviewSignal{
				Kind:     "session_takeover",
				Summary:  fmt.Sprintf("Device session takeover recorded (%d)", takeoverCount),
				Severity: "high",
			})
		}

		if err := upsertReviewSignals(ctx, q, attemptID, signals, len(signals) > 0); err != nil {
			return err
		}

		allSeqs, err := listClientSeqs(ctx, q, attemptID)
		if err != nil {
			return err
		}
		highest, missing := contest.MissingSeqs(allSeqs)
		result = contest.EventBatchResult{
			Accepted:    int(tag.RowsAffected()),
			Duplicate:   false,
			HighestSeq:  highest,
			MissingSeqs: missing,
		}
		return nil
	})
	return result, err
}

func listClientSeqs(ctx context.Context, q Querier, attemptID string) ([]int64, error) {
	rows, err := q.Query(ctx, `
		SELECT seq FROM assessment_attempt_events
		WHERE attempt_id = $1::uuid AND source_kind = 'client'
		ORDER BY seq ASC
	`, attemptID)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	seqs := make([]int64, 0)
	for rows.Next() {
		var seq int64
		if err := rows.Scan(&seq); err != nil {
			return nil, err
		}
		seqs = append(seqs, seq)
	}
	return seqs, rows.Err()
}

func upsertReviewSignals(ctx context.Context, q Querier, attemptID string, incoming []contest.ReviewSignal, flagIfClear bool) error {
	var existingStatus string
	var existingRaw []byte
	err := q.QueryRow(ctx, `
		SELECT status, signals
		FROM assessment_reviews
		WHERE attempt_id = $1::uuid
		FOR UPDATE
	`, attemptID).Scan(&existingStatus, &existingRaw)

	exists := true
	if errors.Is(err, pgx.ErrNoRows) {
		exists = false
		existingStatus = "clear"
		existingRaw = []byte("[]")
	} else if err != nil {
		return mapPGError(err)
	}

	var currentSignals []contest.ReviewSignal
	if err := json.Unmarshal(existingRaw, &currentSignals); err != nil {
		currentSignals = []contest.ReviewSignal{}
	}
	merged := mergeSignals(currentSignals, incoming)

	nextStatus := existingStatus
	if flagIfClear && len(merged) > 0 && existingStatus == "clear" {
		// Auto-detection may flag for human triage, never confirm a violation.
		nextStatus = "flagged"
	}

	mergedRaw, err := json.Marshal(merged)
	if err != nil {
		return err
	}

	if !exists {
		_, err = q.Exec(ctx, `
			INSERT INTO assessment_reviews (attempt_id, status, signals, updated_at)
			VALUES ($1::uuid, $2, $3::jsonb, clock_timestamp())
		`, attemptID, nextStatus, mergedRaw)
		return mapPGError(err)
	}
	_, err = q.Exec(ctx, `
		UPDATE assessment_reviews
		SET status = $2, signals = $3::jsonb, updated_at = clock_timestamp()
		WHERE attempt_id = $1::uuid
	`, attemptID, nextStatus, mergedRaw)
	return mapPGError(err)
}

func mergeSignals(existing, incoming []contest.ReviewSignal) []contest.ReviewSignal {
	out := make([]contest.ReviewSignal, 0, len(existing)+len(incoming))
	seen := make(map[string]int, len(existing)+len(incoming))
	signalKey := func(s contest.ReviewSignal) string {
		// Aggregate signals update in place by kind; event-tied signals dedup by kind+summary+timestamp.
		if s.Kind == "window_blur" || s.Kind == "session_takeover" || s.Kind == "code_similarity" {
			return s.Kind
		}
		ts := ""
		if s.OccurredAt != nil {
			ts = s.OccurredAt.UTC().Format(time.RFC3339Nano)
		}
		return s.Kind + "|" + s.Summary + "|" + ts
	}
	for _, s := range existing {
		k := signalKey(s)
		seen[k] = len(out)
		out = append(out, s)
	}
	for _, s := range incoming {
		k := signalKey(s)
		if idx, ok := seen[k]; ok {
			out[idx] = s
			continue
		}
		if len(out) >= 100 {
			continue
		}
		seen[k] = len(out)
		out = append(out, s)
	}
	return out
}

func (r *ContestIntegrityRepo) SubmitAppeal(ctx context.Context, assessmentID, studentID string, cmd contest.AppealInput) (contest.Appeal, error) {
	var appeal contest.Appeal
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return appeal, errs.ErrNotFound
	}
	if err := contest.ValidateAppeal(&cmd); err != nil {
		return appeal, err
	}

	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		var attemptID, studentName string
		err := q.QueryRow(ctx, `
			SELECT p.id::text, u.name
			FROM assessment_attempts p
			JOIN users u ON u.id = p.student_id
			WHERE p.assessment_id = $1::uuid AND p.student_id = $2
			FOR UPDATE OF p
		`, aid, studentID).Scan(&attemptID, &studentName)
		if err != nil {
			return mapPGError(err)
		}

		appeal = contest.Appeal{
			AttemptID:   attemptID,
			StudentID:   studentID,
			StudentName: studentName,
			Kind:        cmd.Kind,
			Message:     cmd.Message,
		}
		if err := q.QueryRow(ctx, `
			INSERT INTO assessment_appeals (attempt_id, student_id, kind, message)
			VALUES ($1::uuid, $2, $3, $4)
			RETURNING id::text, status, created_at
		`, attemptID, studentID, cmd.Kind, cmd.Message).Scan(&appeal.ID, &appeal.Status, &appeal.CreatedAt); err != nil {
			return mapPGError(err)
		}

		if _, err := q.Exec(ctx, `
			INSERT INTO assessment_reviews (attempt_id, retain_until, updated_at)
			VALUES ($1::uuid, clock_timestamp() + interval '30 days', clock_timestamp())
			ON CONFLICT (attempt_id) DO UPDATE
			SET retain_until = GREATEST(assessment_reviews.retain_until, clock_timestamp() + interval '30 days'),
			    updated_at = clock_timestamp()
		`, attemptID); err != nil {
			return mapPGError(err)
		}

		_, err = q.Exec(ctx, `
			INSERT INTO assessment_evidence_access_log (assessment_id, attempt_id, actor_id, action, detail)
			VALUES ($1::uuid, $2::uuid, $3, 'appeal.submitted', $4)
		`, aid, attemptID, studentID, fmt.Sprintf("Retention extended by 30 days for %s %s", cmd.Kind, appeal.ID))
		return mapPGError(err)
	})
	return appeal, err
}

func (r *ContestIntegrityRepo) ListStudentAppeals(ctx context.Context, assessmentID, studentID string) (contest.StudentIntegrityStatus, error) {
	status := contest.StudentIntegrityStatus{Appeals: make([]contest.Appeal, 0)}
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return status, errs.ErrNotFound
	}
	q := r.tx.Querier(ctx)
	var attemptID string
	err := q.QueryRow(ctx, `
		SELECT p.id::text, COALESCE(rv.status, 'clear')
		FROM assessment_attempts p
		LEFT JOIN assessment_reviews rv ON rv.attempt_id = p.id
		WHERE p.assessment_id = $1::uuid AND p.student_id = $2
	`, aid, studentID).Scan(&attemptID, &status.ReviewStatus)
	if err != nil {
		return status, mapPGError(err)
	}

	rows, err := q.Query(ctx, `
		SELECT ap.id::text, ap.attempt_id::text, ap.student_id, u.name, ap.kind, ap.message,
		       ap.status, COALESCE(ap.reviewer_id, ''), ap.resolution, ap.created_at, ap.resolved_at
		FROM assessment_appeals ap
		JOIN users u ON u.id = ap.student_id
		WHERE ap.attempt_id = $1::uuid AND ap.student_id = $2
		ORDER BY ap.created_at DESC, ap.id DESC
	`, attemptID, studentID)
	if err != nil {
		return status, mapPGError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var ap contest.Appeal
		if err := rows.Scan(
			&ap.ID, &ap.AttemptID, &ap.StudentID, &ap.StudentName, &ap.Kind, &ap.Message,
			&ap.Status, &ap.ReviewerID, &ap.Resolution, &ap.CreatedAt, &ap.ResolvedAt,
		); err != nil {
			return status, err
		}
		status.Appeals = append(status.Appeals, ap)
	}
	return status, rows.Err()
}

func (r *ContestIntegrityRepo) AssignReviewer(ctx context.Context, actor contest.Actor, assessmentID, reviewerID string) error {
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return errs.ErrNotFound
	}
	reviewerID = strings.TrimSpace(reviewerID)
	return r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		var canManage bool
		if err := q.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM assessments
				WHERE id = $1::uuid AND (owner_id = $2 OR $3::boolean)
			)
		`, aid, actor.ID, actor.Admin).Scan(&canManage); err != nil {
			return mapPGError(err)
		}
		if !canManage {
			return errs.ErrNotFound
		}

		var validReviewer bool
		if reviewerID != "" {
			if err := q.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM users
					WHERE id = $1 AND role IN ('teacher', 'admin')
				)
			`, reviewerID).Scan(&validReviewer); err != nil {
				return mapPGError(err)
			}
		}
		if !validReviewer {
			return contest.Invalid("Select an existing teacher or administrator account as reviewer.")
		}

		if _, err := q.Exec(ctx, `
			INSERT INTO assessment_reviewers (assessment_id, reviewer_id, assigned_by)
			VALUES ($1::uuid, $2, $3)
			ON CONFLICT (assessment_id, reviewer_id) DO NOTHING
		`, aid, reviewerID, actor.ID); err != nil {
			return mapPGError(err)
		}

		_, err := q.Exec(ctx, `
			INSERT INTO assessment_evidence_access_log (assessment_id, actor_id, action, detail)
			VALUES ($1::uuid, $2, 'reviewer.assigned', $3)
		`, aid, actor.ID, fmt.Sprintf("Assigned reviewer %s", reviewerID))
		return mapPGError(err)
	})
}

func (r *ContestIntegrityRepo) ListReviewQueue(ctx context.Context, actor contest.Actor, assessmentID string) (contest.ReviewQueueView, error) {
	view := contest.ReviewQueueView{
		Reviews:         make([]contest.ReviewSummary, 0),
		SimilarityPairs: make([]contest.SimilarityPair, 0),
		Appeals:         make([]contest.Appeal, 0),
		Reviewers:       make([]contest.ReviewerInfo, 0),
		AccessLog:       make([]contest.EvidenceAccessEntry, 0),
	}
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return view, errs.ErrNotFound
	}
	view.AssessmentID = aid

	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		if err := requireReviewerAccess(ctx, q, actor, aid); err != nil {
			return err
		}

		if _, err := q.Exec(ctx, `
			INSERT INTO assessment_evidence_access_log (assessment_id, actor_id, action, detail)
			VALUES ($1::uuid, $2, 'review_queue.viewed', '')
		`, aid, actor.ID); err != nil {
			return mapPGError(err)
		}

		revRows, err := q.Query(ctx, `
			SELECT p.id::text, p.student_id, u.name,
			       COALESCE(rv.status, 'clear'),
			       COALESCE(rv.similarity_max, 0),
			       COALESCE(rv.signals, '[]'::jsonb),
			       COALESCE(rv.reason, ''),
			       COALESCE(rv.reviewer_id, ''),
			       rv.reviewed_at,
			       COALESCE(rv.retain_until, COALESCE(p.submitted_at, p.deadline_at) + interval '30 days'),
			       COALESCE((
			           SELECT count(*) FROM assessment_appeals ap
			           WHERE ap.attempt_id = p.id AND ap.status IN ('open', 'in_review')
			       ), 0)
			FROM assessment_attempts p
			JOIN users u ON u.id = p.student_id
			LEFT JOIN assessment_reviews rv ON rv.attempt_id = p.id
			WHERE p.assessment_id = $1::uuid
			ORDER BY
				CASE COALESCE(rv.status, 'clear')
					WHEN 'flagged' THEN 0
					WHEN 'under_review' THEN 1
					WHEN 'confirmed_violation' THEN 2
					WHEN 'clear' THEN 3
					WHEN 'cleared' THEN 4
					ELSE 5
				END ASC,
				COALESCE(rv.similarity_max, 0) DESC,
				u.name ASC,
				p.id ASC
		`, aid)
		if err != nil {
			return mapPGError(err)
		}
		for revRows.Next() {
			item, err := scanReviewSummary(revRows)
			if err != nil {
				revRows.Close()
				return err
			}
			view.Reviews = append(view.Reviews, item)
		}
		if err := revRows.Err(); err != nil {
			revRows.Close()
			return err
		}
		revRows.Close()

		pairs, err := querySimilarityPairs(ctx, q, `
			SELECT assessment_id::text, question_index, attempt_a::text, attempt_b::text,
			       similarity, matched_tokens, snippet_a, snippet_b, computed_at
			FROM assessment_similarity_pairs
			WHERE assessment_id = $1::uuid
			ORDER BY similarity DESC, question_index ASC, id ASC
		`, aid)
		if err != nil {
			return err
		}
		view.SimilarityPairs = pairs

		appeals, err := queryAppeals(ctx, q, `
			SELECT ap.id::text, ap.attempt_id::text, ap.student_id, u.name, ap.kind, ap.message,
			       ap.status, COALESCE(ap.reviewer_id, ''), ap.resolution, ap.created_at, ap.resolved_at
			FROM assessment_appeals ap
			JOIN assessment_attempts p ON p.id = ap.attempt_id
			JOIN users u ON u.id = ap.student_id
			WHERE p.assessment_id = $1::uuid
			ORDER BY ap.created_at DESC, ap.id DESC
		`, aid)
		if err != nil {
			return err
		}
		view.Appeals = appeals

		reviewerRows, err := q.Query(ctx, `
			SELECT ar.reviewer_id, u.name, ar.assigned_by, ar.assigned_at
			FROM assessment_reviewers ar
			JOIN users u ON u.id = ar.reviewer_id
			WHERE ar.assessment_id = $1::uuid
			ORDER BY ar.assigned_at ASC, ar.reviewer_id ASC
		`, aid)
		if err != nil {
			return mapPGError(err)
		}
		for reviewerRows.Next() {
			var info contest.ReviewerInfo
			if err := reviewerRows.Scan(&info.ReviewerID, &info.ReviewerName, &info.AssignedBy, &info.AssignedAt); err != nil {
				reviewerRows.Close()
				return err
			}
			view.Reviewers = append(view.Reviewers, info)
		}
		if err := reviewerRows.Err(); err != nil {
			reviewerRows.Close()
			return err
		}
		reviewerRows.Close()

		logRows, err := q.Query(ctx, `
			SELECT id, COALESCE(attempt_id::text, ''), actor_id, action, detail, occurred_at
			FROM assessment_evidence_access_log
			WHERE assessment_id = $1::uuid
			ORDER BY occurred_at DESC, id DESC
			LIMIT 100
		`, aid)
		if err != nil {
			return mapPGError(err)
		}
		for logRows.Next() {
			var entry contest.EvidenceAccessEntry
			if err := logRows.Scan(&entry.ID, &entry.AttemptID, &entry.ActorID, &entry.Action, &entry.Detail, &entry.OccurredAt); err != nil {
				logRows.Close()
				return err
			}
			view.AccessLog = append(view.AccessLog, entry)
		}
		err = logRows.Err()
		logRows.Close()
		return err
	})
	return view, err
}

func (r *ContestIntegrityRepo) GetAttemptReplay(ctx context.Context, actor contest.Actor, assessmentID, attemptID string) (contest.AttemptReplayView, error) {
	view := contest.AttemptReplayView{
		Events:           make([]contest.ReplayEvent, 0),
		MissingIntervals: make([]contest.SequenceGap, 0),
		SimilarityPairs:  make([]contest.SimilarityPair, 0),
		Appeals:          make([]contest.Appeal, 0),
	}
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return view, errs.ErrNotFound
	}
	attID, ok := parseCanonicalUUID(attemptID)
	if !ok {
		return view, errs.ErrNotFound
	}

	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		if err := requireReviewerAccess(ctx, q, actor, aid); err != nil {
			return err
		}

		var startedAt, deadlineAt time.Time
		var submittedAt *time.Time
		var finishReason, deviceID string
		var answerRevision int64
		var signalsRaw []byte

		err := q.QueryRow(ctx, `
			SELECT p.id::text, p.student_id, u.name,
			       COALESCE(rv.status, 'clear'),
			       COALESCE(rv.similarity_max, 0),
			       COALESCE(rv.signals, '[]'::jsonb),
			       COALESCE(rv.reason, ''),
			       COALESCE(rv.reviewer_id, ''),
			       rv.reviewed_at,
			       COALESCE(rv.retain_until, COALESCE(p.submitted_at, p.deadline_at) + interval '30 days'),
			       COALESCE((
			           SELECT count(*) FROM assessment_appeals ap
			           WHERE ap.attempt_id = p.id AND ap.status IN ('open', 'in_review')
			       ), 0),
			       p.started_at, p.deadline_at, p.submitted_at, COALESCE(p.finish_reason, ''),
			       p.device_id::text, p.answer_revision
			FROM assessment_attempts p
			JOIN users u ON u.id = p.student_id
			LEFT JOIN assessment_reviews rv ON rv.attempt_id = p.id
			WHERE p.id = $1::uuid AND p.assessment_id = $2::uuid
		`, attID, aid).Scan(
			&view.Review.AttemptID, &view.Review.StudentID, &view.Review.StudentName,
			&view.Review.Status, &view.Review.SimilarityMax, &signalsRaw,
			&view.Review.Reason, &view.Review.ReviewerID, &view.Review.ReviewedAt,
			&view.Review.RetainUntil, &view.Review.OpenAppeals,
			&startedAt, &deadlineAt, &submittedAt, &finishReason, &deviceID, &answerRevision,
		)
		if err != nil {
			return mapPGError(err)
		}
		if err := json.Unmarshal(signalsRaw, &view.Review.Signals); err != nil || view.Review.Signals == nil {
			view.Review.Signals = []contest.ReviewSignal{}
		}

		if _, err := q.Exec(ctx, `
			INSERT INTO assessment_evidence_access_log (assessment_id, attempt_id, actor_id, action, detail)
			VALUES ($1::uuid, $2::uuid, $3, 'attempt_replay.viewed', '')
		`, aid, attID, actor.ID); err != nil {
			return mapPGError(err)
		}

		storedRows, err := q.Query(ctx, `
			SELECT seq, source_kind, event_type, question_index, client_at, server_received_at, payload
			FROM assessment_attempt_events
			WHERE attempt_id = $1::uuid
			ORDER BY seq ASC
		`, attID)
		if err != nil {
			return mapPGError(err)
		}
		clientSeqs := make([]int64, 0)
		hasStoredServerType := make(map[string]bool)
		for storedRows.Next() {
			var ev contest.ReplayEvent
			var payloadRaw []byte
			if err := storedRows.Scan(
				&ev.Seq, &ev.SourceKind, &ev.EventType, &ev.QuestionIndex,
				&ev.ClientAt, &ev.ServerReceivedAt, &payloadRaw,
			); err != nil {
				storedRows.Close()
				return err
			}
			if err := json.Unmarshal(payloadRaw, &ev.Payload); err != nil || ev.Payload == nil {
				ev.Payload = map[string]any{}
			}
			if ev.SourceKind == "client" {
				clientSeqs = append(clientSeqs, ev.Seq)
			} else {
				hasStoredServerType[ev.EventType] = true
			}
			view.Events = append(view.Events, ev)
		}
		if err := storedRows.Err(); err != nil {
			storedRows.Close()
			return err
		}
		storedRows.Close()

		view.MissingIntervals = contest.MissingIntervals(clientSeqs)

		if !hasStoredServerType["attempt.started"] {
			view.Events = append(view.Events, contest.ReplayEvent{
				Seq:              0,
				SourceKind:       "server",
				EventType:        "attempt.started",
				ServerReceivedAt: startedAt,
				Payload: map[string]any{
					"deviceId":   deviceID,
					"deadlineAt": deadlineAt.UTC().Format(time.RFC3339),
				},
			})
		}

		if !hasStoredServerType["attempt.takeover"] {
			tkRows, err := q.Query(ctx, `
				SELECT lease, replaced_at
				FROM assessment_attempt_takeovers
				WHERE attempt_id = $1::uuid
				ORDER BY lease ASC
			`, attID)
			if err != nil {
				return mapPGError(err)
			}
			for tkRows.Next() {
				var lease int64
				var replacedAt time.Time
				if err := tkRows.Scan(&lease, &replacedAt); err != nil {
					tkRows.Close()
					return err
				}
				view.Events = append(view.Events, contest.ReplayEvent{
					Seq:              0,
					SourceKind:       "server",
					EventType:        "attempt.takeover",
					ServerReceivedAt: replacedAt,
					Payload:          map[string]any{"lease": lease},
				})
			}
			if err := tkRows.Err(); err != nil {
				tkRows.Close()
				return err
			}
			tkRows.Close()
		}

		if !hasStoredServerType["attempt.code_submitted"] {
			subRows, err := q.Query(ctx, `
				SELECT id::text, question_index, language, submitted_at, status, COALESCE(verdict, '')
				FROM assessment_coding_submissions
				WHERE attempt_id = $1::uuid
				ORDER BY submitted_at ASC, id ASC
			`, attID)
			if err != nil {
				return mapPGError(err)
			}
			for subRows.Next() {
				var subID, lang, status, verdict string
				var qIdx int
				var subAt time.Time
				if err := subRows.Scan(&subID, &qIdx, &lang, &subAt, &status, &verdict); err != nil {
					subRows.Close()
					return err
				}
				qCopy := qIdx
				payload := map[string]any{
					"submissionId": subID,
					"language":     lang,
					"status":       status,
				}
				if verdict != "" {
					payload["verdict"] = verdict
				}
				view.Events = append(view.Events, contest.ReplayEvent{
					Seq:              0,
					SourceKind:       "server",
					EventType:        "attempt.code_submitted",
					QuestionIndex:    &qCopy,
					ServerReceivedAt: subAt,
					Payload:          payload,
				})
			}
			if err := subRows.Err(); err != nil {
				subRows.Close()
				return err
			}
			subRows.Close()
		}

		if submittedAt != nil {
			endType := "attempt.submitted"
			if finishReason == "expired" {
				endType = "attempt.expired"
			}
			if !hasStoredServerType[endType] {
				view.Events = append(view.Events, contest.ReplayEvent{
					Seq:              0,
					SourceKind:       "server",
					EventType:        endType,
					ServerReceivedAt: *submittedAt,
					Payload: map[string]any{
						"finishReason":   finishReason,
						"answerRevision": answerRevision,
					},
				})
			}
		}

		sort.SliceStable(view.Events, func(i, j int) bool {
			a, b := view.Events[i], view.Events[j]
			if !a.ServerReceivedAt.Equal(b.ServerReceivedAt) {
				return a.ServerReceivedAt.Before(b.ServerReceivedAt)
			}
			if a.Seq != b.Seq {
				return a.Seq < b.Seq
			}
			if a.SourceKind != b.SourceKind {
				return a.SourceKind == "server"
			}
			return a.EventType < b.EventType
		})

		pairs, err := querySimilarityPairs(ctx, q, `
			SELECT assessment_id::text, question_index, attempt_a::text, attempt_b::text,
			       similarity, matched_tokens, snippet_a, snippet_b, computed_at
			FROM assessment_similarity_pairs
			WHERE assessment_id = $1::uuid AND (attempt_a = $2::uuid OR attempt_b = $2::uuid)
			ORDER BY similarity DESC, question_index ASC, id ASC
		`, aid, attID)
		if err != nil {
			return err
		}
		view.SimilarityPairs = pairs

		appeals, err := queryAppeals(ctx, q, `
			SELECT ap.id::text, ap.attempt_id::text, ap.student_id, u.name, ap.kind, ap.message,
			       ap.status, COALESCE(ap.reviewer_id, ''), ap.resolution, ap.created_at, ap.resolved_at
			FROM assessment_appeals ap
			JOIN users u ON u.id = ap.student_id
			WHERE ap.attempt_id = $1::uuid
			ORDER BY ap.created_at DESC, ap.id DESC
		`, attID)
		if err != nil {
			return err
		}
		view.Appeals = appeals
		return nil
	})
	return view, err
}

func (r *ContestIntegrityRepo) DecideReview(ctx context.Context, actor contest.Actor, assessmentID, attemptID string, cmd contest.ReviewDecisionInput) (contest.ReviewSummary, error) {
	var summary contest.ReviewSummary
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return summary, errs.ErrNotFound
	}
	attID, ok := parseCanonicalUUID(attemptID)
	if !ok {
		return summary, errs.ErrNotFound
	}
	if err := contest.ValidateReviewDecision(&cmd); err != nil {
		return summary, err
	}

	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		if err := requireReviewerAccess(ctx, q, actor, aid); err != nil {
			return err
		}

		var studentID, studentName string
		err := q.QueryRow(ctx, `
			SELECT p.student_id, u.name
			FROM assessment_attempts p
			JOIN users u ON u.id = p.student_id
			WHERE p.id = $1::uuid AND p.assessment_id = $2::uuid
			FOR UPDATE OF p
		`, attID, aid).Scan(&studentID, &studentName)
		if err != nil {
			return mapPGError(err)
		}

		var signalsRaw []byte
		err = q.QueryRow(ctx, `
			INSERT INTO assessment_reviews (attempt_id, status, reason, reviewer_id, reviewed_at, updated_at)
			VALUES ($1::uuid, $2, $3, $4, clock_timestamp(), clock_timestamp())
			ON CONFLICT (attempt_id) DO UPDATE
			SET status = EXCLUDED.status,
			    reason = EXCLUDED.reason,
			    reviewer_id = EXCLUDED.reviewer_id,
			    reviewed_at = EXCLUDED.reviewed_at,
			    updated_at = EXCLUDED.updated_at
			RETURNING status, similarity_max, signals, reason, COALESCE(reviewer_id, ''), reviewed_at, retain_until
		`, attID, cmd.Status, cmd.Reason, actor.ID).Scan(
			&summary.Status, &summary.SimilarityMax, &signalsRaw,
			&summary.Reason, &summary.ReviewerID, &summary.ReviewedAt, &summary.RetainUntil,
		)
		if err != nil {
			return mapPGError(err)
		}
		summary.AttemptID = attID
		summary.StudentID = studentID
		summary.StudentName = studentName
		if err := json.Unmarshal(signalsRaw, &summary.Signals); err != nil || summary.Signals == nil {
			summary.Signals = []contest.ReviewSignal{}
		}

		if err := q.QueryRow(ctx, `
			SELECT count(*) FROM assessment_appeals
			WHERE attempt_id = $1::uuid AND status IN ('open', 'in_review')
		`, attID).Scan(&summary.OpenAppeals); err != nil {
			return mapPGError(err)
		}

		_, err = q.Exec(ctx, `
			INSERT INTO assessment_evidence_access_log (assessment_id, attempt_id, actor_id, action, detail)
			VALUES ($1::uuid, $2::uuid, $3, 'review.decided', $4)
		`, aid, attID, actor.ID, fmt.Sprintf("Set review status to %s", cmd.Status))
		return mapPGError(err)
	})
	return summary, err
}

func (r *ContestIntegrityRepo) ResolveAppeal(ctx context.Context, actor contest.Actor, assessmentID, appealID string, cmd contest.ResolveAppealInput) (contest.Appeal, error) {
	var appeal contest.Appeal
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return appeal, errs.ErrNotFound
	}
	apID, ok := parseCanonicalUUID(appealID)
	if !ok {
		return appeal, errs.ErrNotFound
	}
	if err := contest.ValidateResolveAppeal(&cmd); err != nil {
		return appeal, err
	}

	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		if err := requireReviewerAccess(ctx, q, actor, aid); err != nil {
			return err
		}

		var reviewReviewerID string
		err := q.QueryRow(ctx, `
			SELECT ap.id::text, ap.attempt_id::text, ap.student_id, u.name, ap.kind, ap.message,
			       COALESCE(rv.reviewer_id, '')
			FROM assessment_appeals ap
			JOIN assessment_attempts p ON p.id = ap.attempt_id
			JOIN users u ON u.id = ap.student_id
			LEFT JOIN assessment_reviews rv ON rv.attempt_id = ap.attempt_id
			WHERE ap.id = $1::uuid AND p.assessment_id = $2::uuid
			FOR UPDATE OF ap
		`, apID, aid).Scan(
			&appeal.ID, &appeal.AttemptID, &appeal.StudentID, &appeal.StudentName,
			&appeal.Kind, &appeal.Message, &reviewReviewerID,
		)
		if err != nil {
			return mapPGError(err)
		}

		if reviewReviewerID != "" && reviewReviewerID == actor.ID {
			var otherReviewers int
			if err := q.QueryRow(ctx, `
				SELECT count(*) FROM assessment_reviewers
				WHERE assessment_id = $1::uuid AND reviewer_id <> $2
			`, aid, actor.ID).Scan(&otherReviewers); err != nil {
				return mapPGError(err)
			}
			if otherReviewers > 0 {
				return contest.Invalid("Assign a different reviewer to resolve an appeal when staffing permits.")
			}
		}

		if err := q.QueryRow(ctx, `
			UPDATE assessment_appeals
			SET status = $2,
			    resolution = $3,
			    reviewer_id = $4,
			    resolved_at = CASE WHEN $2 IN ('resolved', 'dismissed') THEN clock_timestamp() ELSE NULL END
			WHERE id = $1::uuid
			RETURNING status, COALESCE(reviewer_id, ''), resolution, created_at, resolved_at
		`, apID, cmd.Status, cmd.Resolution, actor.ID).Scan(
			&appeal.Status, &appeal.ReviewerID, &appeal.Resolution, &appeal.CreatedAt, &appeal.ResolvedAt,
		); err != nil {
			return mapPGError(err)
		}

		_, err = q.Exec(ctx, `
			INSERT INTO assessment_evidence_access_log (assessment_id, attempt_id, actor_id, action, detail)
			VALUES ($1::uuid, $2::uuid, $3, 'appeal.resolved', $4)
		`, aid, appeal.AttemptID, actor.ID, fmt.Sprintf("Updated appeal %s to %s", appeal.ID, cmd.Status))
		return mapPGError(err)
	})
	return appeal, err
}

type attemptSolution struct {
	language string
	source   string
}

func (r *ContestIntegrityRepo) ComputeSimilarity(ctx context.Context, actor contest.Actor, assessmentID string) ([]contest.SimilarityPair, error) {
	pairs := make([]contest.SimilarityPair, 0)
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return pairs, errs.ErrNotFound
	}

	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		if err := requireReviewerAccess(ctx, q, actor, aid); err != nil {
			return err
		}

		var questionsJSON []byte
		if err := q.QueryRow(ctx, `
			SELECT questions FROM assessments WHERE id = $1::uuid FOR SHARE
		`, aid).Scan(&questionsJSON); err != nil {
			return mapPGError(err)
		}
		var questions []contest.Question
		if err := json.Unmarshal(questionsJSON, &questions); err != nil {
			return err
		}

		type attemptEntry struct {
			id      string
			answers map[int]contest.Answer
		}
		attemptRows, err := q.Query(ctx, `
			SELECT id::text, answers
			FROM assessment_attempts
			WHERE assessment_id = $1::uuid
			ORDER BY id ASC
		`, aid)
		if err != nil {
			return mapPGError(err)
		}
		attempts := make([]attemptEntry, 0)
		for attemptRows.Next() {
			var entry attemptEntry
			var answersRaw []byte
			if err := attemptRows.Scan(&entry.id, &answersRaw); err != nil {
				attemptRows.Close()
				return err
			}
			if err := json.Unmarshal(answersRaw, &entry.answers); err != nil {
				attemptRows.Close()
				return err
			}
			attempts = append(attempts, entry)
		}
		if err := attemptRows.Err(); err != nil {
			attemptRows.Close()
			return err
		}
		attemptRows.Close()

		// Prefer the latest accepted official coding submission (or latest submitted) over draft code.
		official := make(map[string]map[int]attemptSolution)
		subRows, err := q.Query(ctx, `
			SELECT c.attempt_id::text, c.question_index, c.language, c.source, c.status
			FROM assessment_coding_submissions c
			JOIN assessment_attempts p ON p.id = c.attempt_id
			WHERE p.assessment_id = $1::uuid
			ORDER BY (c.status = 'accepted') DESC, c.submitted_at DESC, c.id ASC
		`, aid)
		if err != nil {
			return mapPGError(err)
		}
		for subRows.Next() {
			var attID, lang, src, status string
			var qIdx int
			if err := subRows.Scan(&attID, &qIdx, &lang, &src, &status); err != nil {
				subRows.Close()
				return err
			}
			perAttempt := official[attID]
			if perAttempt == nil {
				perAttempt = make(map[int]attemptSolution)
				official[attID] = perAttempt
			}
			if _, exists := perAttempt[qIdx]; !exists {
				perAttempt[qIdx] = attemptSolution{language: lang, source: src}
			}
		}
		if err := subRows.Err(); err != nil {
			subRows.Close()
			return err
		}
		subRows.Close()

		if _, err := q.Exec(ctx, `
			DELETE FROM assessment_similarity_pairs WHERE assessment_id = $1::uuid
		`, aid); err != nil {
			return mapPGError(err)
		}

		maxSimByAttempt := make(map[string]float64, len(attempts))
		for _, a := range attempts {
			maxSimByAttempt[a.id] = 0
		}

		for qIdx, question := range questions {
			if question.Kind != "coding" {
				continue
			}
			type candidate struct {
				attemptID string
				sol       attemptSolution
			}
			candidates := make([]candidate, 0, len(attempts))
			for _, att := range attempts {
				var sol attemptSolution
				if off, ok := official[att.id][qIdx]; ok {
					sol = off
				} else if ans, ok := att.answers[qIdx]; ok && strings.TrimSpace(ans.Source) != "" {
					sol = attemptSolution{language: ans.Lang, source: ans.Source}
				}
				if strings.TrimSpace(sol.source) == "" {
					continue
				}
				candidates = append(candidates, candidate{attemptID: att.id, sol: sol})
			}

			for i := 0; i < len(candidates); i++ {
				for j := i + 1; j < len(candidates); j++ {
					left, right := candidates[i], candidates[j]
					if left.sol.language != "" && right.sol.language != "" && left.sol.language != right.sol.language {
						continue
					}
					lang := left.sol.language
					if lang == "" {
						lang = right.sol.language
					}
					sim, matched, snipA, snipB := contest.CompareSolutions(lang, "", left.sol.source, right.sol.source)
					if matched == 0 || sim < 0.25 {
						continue
					}

					attA, attB := left.attemptID, right.attemptID
					if attA > attB {
						attA, attB = attB, attA
						snipA, snipB = snipB, snipA
					}
					var computedAt time.Time
					if err := q.QueryRow(ctx, `
						INSERT INTO assessment_similarity_pairs (
							assessment_id, question_index, attempt_a, attempt_b,
							similarity, matched_tokens, snippet_a, snippet_b
						)
						VALUES ($1::uuid, $2, $3::uuid, $4::uuid, $5, $6, $7, $8)
						ON CONFLICT (assessment_id, question_index, attempt_a, attempt_b) DO UPDATE
						SET similarity = EXCLUDED.similarity,
						    matched_tokens = EXCLUDED.matched_tokens,
						    snippet_a = EXCLUDED.snippet_a,
						    snippet_b = EXCLUDED.snippet_b,
						    computed_at = clock_timestamp()
						RETURNING computed_at
					`, aid, qIdx, attA, attB, sim, matched, snipA, snipB).Scan(&computedAt); err != nil {
						return mapPGError(err)
					}

					pairs = append(pairs, contest.SimilarityPair{
						AssessmentID:  aid,
						QuestionIndex: qIdx,
						AttemptA:      attA,
						AttemptB:      attB,
						Similarity:    sim,
						MatchedTokens: matched,
						SnippetA:      snipA,
						SnippetB:      snipB,
						ComputedAt:    computedAt,
					})
					if sim > maxSimByAttempt[attA] {
						maxSimByAttempt[attA] = sim
					}
					if sim > maxSimByAttempt[attB] {
						maxSimByAttempt[attB] = sim
					}
				}
			}
		}

		for attID, simMax := range maxSimByAttempt {
			if simMax >= contest.SimilarityFlagThreshold {
				sig := contest.ReviewSignal{
					Kind:     "code_similarity",
					Summary:  fmt.Sprintf("High code similarity detected (%.0f%%)", simMax*100),
					Severity: "high",
				}
				if err := upsertReviewSignals(ctx, q, attID, []contest.ReviewSignal{sig}, true); err != nil {
					return err
				}
			}
			if _, err := q.Exec(ctx, `
				INSERT INTO assessment_reviews (attempt_id, similarity_max, updated_at)
				VALUES ($1::uuid, $2, clock_timestamp())
				ON CONFLICT (attempt_id) DO UPDATE
				SET similarity_max = EXCLUDED.similarity_max,
				    updated_at = clock_timestamp()
			`, attID, simMax); err != nil {
				return mapPGError(err)
			}
		}

		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].Similarity != pairs[j].Similarity {
				return pairs[i].Similarity > pairs[j].Similarity
			}
			if pairs[i].QuestionIndex != pairs[j].QuestionIndex {
				return pairs[i].QuestionIndex < pairs[j].QuestionIndex
			}
			if pairs[i].AttemptA != pairs[j].AttemptA {
				return pairs[i].AttemptA < pairs[j].AttemptA
			}
			return pairs[i].AttemptB < pairs[j].AttemptB
		})

		_, err = q.Exec(ctx, `
			INSERT INTO assessment_evidence_access_log (assessment_id, actor_id, action, detail)
			VALUES ($1::uuid, $2, 'similarity.computed', $3)
		`, aid, actor.ID, fmt.Sprintf("Computed %d similarity pair(s)", len(pairs)))
		return mapPGError(err)
	})
	return pairs, err
}

func (r *ContestIntegrityRepo) PurgeExpiredEvidence(ctx context.Context) (int64, error) {
	var totalDeleted int64
	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)

		rows, err := q.Query(ctx, `
			SELECT rv.attempt_id::text, p.assessment_id::text
			FROM assessment_reviews rv
			JOIN assessment_attempts p ON p.id = rv.attempt_id
			WHERE rv.retain_until <= clock_timestamp()
			  AND NOT EXISTS (
			      SELECT 1 FROM assessment_appeals ap
			      WHERE ap.attempt_id = rv.attempt_id
			        AND ap.status IN ('open', 'in_review')
			  )
			FOR UPDATE OF rv
		`)
		if err != nil {
			return mapPGError(err)
		}
		expiredAttempts := make([]string, 0)
		perAssessment := make(map[string]int64)
		attemptAssessment := make(map[string]string)
		for rows.Next() {
			var attID, aid string
			if err := rows.Scan(&attID, &aid); err != nil {
				rows.Close()
				return err
			}
			expiredAttempts = append(expiredAttempts, attID)
			attemptAssessment[attID] = aid
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		if len(expiredAttempts) > 0 {
			evTag, err := q.Exec(ctx, `
				DELETE FROM assessment_attempt_events
				WHERE attempt_id = ANY($1::uuid[])
			`, expiredAttempts)
			if err != nil {
				return mapPGError(err)
			}
			totalDeleted += evTag.RowsAffected()

			if _, err := q.Exec(ctx, `
				DELETE FROM assessment_event_batches
				WHERE attempt_id = ANY($1::uuid[])
			`, expiredAttempts); err != nil {
				return mapPGError(err)
			}

			pairTag, err := q.Exec(ctx, `
				DELETE FROM assessment_similarity_pairs sp
				WHERE (sp.attempt_a = ANY($1::uuid[]) OR sp.attempt_b = ANY($1::uuid[]))
				  AND NOT EXISTS (
				      SELECT 1 FROM assessment_appeals ap
				      WHERE ap.attempt_id IN (sp.attempt_a, sp.attempt_b)
				        AND ap.status IN ('open', 'in_review')
				  )
			`, expiredAttempts)
			if err != nil {
				return mapPGError(err)
			}
			totalDeleted += pairTag.RowsAffected()

			revTag, err := q.Exec(ctx, `
				DELETE FROM assessment_reviews
				WHERE attempt_id = ANY($1::uuid[])
			`, expiredAttempts)
			if err != nil {
				return mapPGError(err)
			}
			totalDeleted += revTag.RowsAffected()

			for _, aid := range attemptAssessment {
				perAssessment[aid]++
			}
		}

		// Also remove any orphaned events older than 30 days without an assessment_reviews row or active appeal.
		orphanRows, err := q.Query(ctx, `
			WITH doomed AS (
				DELETE FROM assessment_attempt_events e
				WHERE e.server_received_at <= clock_timestamp() - interval '30 days'
				  AND NOT EXISTS (SELECT 1 FROM assessment_reviews rv WHERE rv.attempt_id = e.attempt_id)
				  AND NOT EXISTS (
				      SELECT 1 FROM assessment_appeals ap
				      WHERE ap.attempt_id = e.attempt_id AND ap.status IN ('open', 'in_review')
				  )
				RETURNING e.attempt_id
			)
			SELECT p.assessment_id::text, count(*)
			FROM doomed d
			JOIN assessment_attempts p ON p.id = d.attempt_id
			GROUP BY p.assessment_id
		`)
		if err != nil {
			return mapPGError(err)
		}
		for orphanRows.Next() {
			var aid string
			var cnt int64
			if err := orphanRows.Scan(&aid, &cnt); err != nil {
				orphanRows.Close()
				return err
			}
			totalDeleted += cnt
			perAssessment[aid] += cnt
		}
		if err := orphanRows.Err(); err != nil {
			orphanRows.Close()
			return err
		}
		orphanRows.Close()

		if totalDeleted > 0 {
			for aid, cnt := range perAssessment {
				if _, err := q.Exec(ctx, `
					INSERT INTO assessment_evidence_access_log (assessment_id, actor_id, action, detail)
					VALUES ($1::uuid, 'system', 'evidence.purged', $2)
				`, aid, fmt.Sprintf("Purged expired evidence for %d attempt/event record(s)", cnt)); err != nil {
					return mapPGError(err)
				}
			}
		}
		return nil
	})
	return totalDeleted, err
}

func (r *ContestIntegrityRepo) RecordProctorMedia(ctx context.Context, assessmentID, studentID string, in contest.ProctorMediaInput) (contest.ProctorMediaItem, error) {
	var item contest.ProctorMediaItem
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return item, errs.ErrNotFound
	}
	if _, err := contest.ValidateProctorMediaInput(&in); err != nil {
		return item, err
	}
	if strings.TrimSpace(in.StorageKey) == "" {
		return item, contest.Invalid("Provide a valid proctoring media storage key.")
	}
	attemptID, _ := parseCanonicalUUID(in.AttemptID)
	deviceID, _ := parseCanonicalUUID(in.DeviceID)

	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		var lockedDevice string
		var submittedAt *time.Time
		var now time.Time
		err := q.QueryRow(ctx, `
			SELECT device_id::text, submitted_at, clock_timestamp()
			FROM assessment_attempts
			WHERE id = $1::uuid AND assessment_id = $2::uuid AND student_id = $3
			FOR UPDATE
		`, attemptID, aid, studentID).Scan(&lockedDevice, &submittedAt, &now)
		if err != nil {
			return mapPGError(err)
		}
		if lockedDevice != deviceID {
			return errs.New(errs.ErrConflict, "ATTEMPT_DEVICE_MISMATCH", "This device session is no longer active for the attempt.")
		}
		if submittedAt != nil && now.After(submittedAt.Add(120*time.Second)) {
			return errs.New(errs.ErrConflict, "ATTEMPT_CLOSED", "The proctoring media upload window for this submitted attempt has closed.")
		}

		var existingCount int
		if err := q.QueryRow(ctx, `
			SELECT count(*) FROM assessment_proctor_media
			WHERE attempt_id = $1::uuid
		`, attemptID).Scan(&existingCount); err != nil {
			return mapPGError(err)
		}
		if existingCount >= contest.MaxProctorMediaItemsPerAttempt {
			return errs.New(errs.ErrRateLimited, "PROCTOR_MEDIA_QUOTA", "Proctoring media quota exceeded for this attempt.")
		}

		capturedAt := in.CapturedAt.UTC()
		item = contest.ProctorMediaItem{
			AttemptID:    attemptID,
			AssessmentID: aid,
			Kind:         in.Kind,
			Seq:          in.Seq,
			ContentType:  in.ContentType,
			ByteSize:     in.ByteSize,
			DurationMS:   in.DurationMS,
			CapturedAt:   capturedAt,
			StorageKey:   in.StorageKey,
		}
		err = q.QueryRow(ctx, `
			INSERT INTO assessment_proctor_media (
				attempt_id, assessment_id, student_id, kind, seq,
				content_type, storage_key, byte_size, duration_ms, captured_at
			)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id::text, created_at
		`, attemptID, aid, studentID, in.Kind, in.Seq, in.ContentType, in.StorageKey, in.ByteSize, in.DurationMS, capturedAt).Scan(&item.ID, &item.CreatedAt)
		if err != nil {
			return mapPGError(err)
		}
		return nil
	})
	return item, err
}

func (r *ContestIntegrityRepo) ListProctorMedia(ctx context.Context, actor contest.Actor, assessmentID, attemptID string) ([]contest.ProctorMediaItem, error) {
	items := make([]contest.ProctorMediaItem, 0)
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return items, errs.ErrNotFound
	}
	attID, ok := parseCanonicalUUID(attemptID)
	if !ok {
		return items, errs.ErrNotFound
	}

	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		if err := requireReviewerAccess(ctx, q, actor, aid); err != nil {
			return err
		}

		var attemptExists bool
		if err := q.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM assessment_attempts
				WHERE id = $1::uuid AND assessment_id = $2::uuid
			)
		`, attID, aid).Scan(&attemptExists); err != nil {
			return mapPGError(err)
		}
		if !attemptExists {
			return errs.ErrNotFound
		}

		if _, err := q.Exec(ctx, `
			INSERT INTO assessment_evidence_access_log (assessment_id, attempt_id, actor_id, action, detail)
			VALUES ($1::uuid, $2::uuid, $3, 'proctor_media.list', '')
		`, aid, attID, actor.ID); err != nil {
			return mapPGError(err)
		}

		rows, err := q.Query(ctx, `
			SELECT id::text, attempt_id::text, assessment_id::text, kind, seq,
			       content_type, byte_size, duration_ms, captured_at, created_at, storage_key
			FROM assessment_proctor_media
			WHERE attempt_id = $1::uuid AND assessment_id = $2::uuid
			ORDER BY captured_at ASC, kind ASC, seq ASC
		`, attID, aid)
		if err != nil {
			return mapPGError(err)
		}
		defer rows.Close()
		for rows.Next() {
			var item contest.ProctorMediaItem
			if err := rows.Scan(
				&item.ID, &item.AttemptID, &item.AssessmentID, &item.Kind, &item.Seq,
				&item.ContentType, &item.ByteSize, &item.DurationMS, &item.CapturedAt, &item.CreatedAt, &item.StorageKey,
			); err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (r *ContestIntegrityRepo) GetProctorMedia(ctx context.Context, actor contest.Actor, assessmentID, mediaID string) (contest.ProctorMediaItem, error) {
	var item contest.ProctorMediaItem
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return item, errs.ErrNotFound
	}
	mid, ok := parseCanonicalUUID(mediaID)
	if !ok {
		return item, errs.ErrNotFound
	}

	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		if err := requireReviewerAccess(ctx, q, actor, aid); err != nil {
			return err
		}

		err := q.QueryRow(ctx, `
			SELECT id::text, attempt_id::text, assessment_id::text, kind, seq,
			       content_type, byte_size, duration_ms, captured_at, created_at, storage_key
			FROM assessment_proctor_media
			WHERE id = $1::uuid AND assessment_id = $2::uuid
		`, mid, aid).Scan(
			&item.ID, &item.AttemptID, &item.AssessmentID, &item.Kind, &item.Seq,
			&item.ContentType, &item.ByteSize, &item.DurationMS, &item.CapturedAt, &item.CreatedAt, &item.StorageKey,
		)
		if err != nil {
			return mapPGError(err)
		}

		_, err = q.Exec(ctx, `
			INSERT INTO assessment_evidence_access_log (assessment_id, attempt_id, actor_id, action, detail)
			VALUES ($1::uuid, $2::uuid, $3, 'proctor_media.view', $4)
		`, aid, item.AttemptID, actor.ID, fmt.Sprintf("Viewed %s seq %d (%s)", item.Kind, item.Seq, item.ID))
		return mapPGError(err)
	})
	return item, err
}

func (r *ContestIntegrityRepo) PurgeExpiredProctorMedia(ctx context.Context, removeFile func(string) error) (int64, error) {
	const batchLimit = 200
	var totalDeleted int64

	for {
		var batchCount int
		err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
			q := r.tx.Querier(ctx)
			rows, err := q.Query(ctx, `
				SELECT m.id::text, m.assessment_id::text, m.storage_key
				FROM assessment_proctor_media m
				LEFT JOIN assessment_reviews rv ON rv.attempt_id = m.attempt_id
				WHERE (
					(rv.attempt_id IS NOT NULL AND rv.retain_until <= clock_timestamp())
					OR (rv.attempt_id IS NULL AND m.created_at <= clock_timestamp() - interval '30 days')
				)
				  AND NOT EXISTS (
				      SELECT 1 FROM assessment_appeals ap
				      WHERE ap.attempt_id = m.attempt_id
				        AND ap.status IN ('open', 'in_review')
				  )
				ORDER BY m.created_at ASC, m.id ASC
				LIMIT $1
				FOR UPDATE OF m SKIP LOCKED
			`, batchLimit)
			if err != nil {
				return mapPGError(err)
			}

			type doomedMedia struct {
				id           string
				assessmentID string
				storageKey   string
			}
			doomed := make([]doomedMedia, 0)
			for rows.Next() {
				var row doomedMedia
				if err := rows.Scan(&row.id, &row.assessmentID, &row.storageKey); err != nil {
					rows.Close()
					return err
				}
				doomed = append(doomed, row)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()

			batchCount = len(doomed)
			if batchCount == 0 {
				return nil
			}

			perAssessment := make(map[string]int64)
			for _, item := range doomed {
				if removeFile != nil {
					if err := removeFile(item.storageKey); err != nil && !errors.Is(err, fs.ErrNotExist) {
						return err
					}
				}
				if _, err := q.Exec(ctx, `
					DELETE FROM assessment_proctor_media WHERE id = $1::uuid
				`, item.id); err != nil {
					return mapPGError(err)
				}
				perAssessment[item.assessmentID]++
				totalDeleted++
			}

			for aid, cnt := range perAssessment {
				if _, err := q.Exec(ctx, `
					INSERT INTO assessment_evidence_access_log (assessment_id, actor_id, action, detail)
					VALUES ($1::uuid, 'system', 'proctor_media.purged', $2)
				`, aid, fmt.Sprintf("Purged %d expired proctoring media item(s)", cnt)); err != nil {
					return mapPGError(err)
				}
			}
			return nil
		})
		if err != nil {
			return totalDeleted, err
		}
		if batchCount < batchLimit {
			break
		}
	}
	return totalDeleted, nil
}

func scanReviewSummary(row pgx.Rows) (contest.ReviewSummary, error) {
	var item contest.ReviewSummary
	var signalsRaw []byte
	if err := row.Scan(
		&item.AttemptID, &item.StudentID, &item.StudentName,
		&item.Status, &item.SimilarityMax, &signalsRaw,
		&item.Reason, &item.ReviewerID, &item.ReviewedAt,
		&item.RetainUntil, &item.OpenAppeals,
	); err != nil {
		return item, err
	}
	if err := json.Unmarshal(signalsRaw, &item.Signals); err != nil || item.Signals == nil {
		item.Signals = []contest.ReviewSignal{}
	}
	return item, nil
}

func querySimilarityPairs(ctx context.Context, q Querier, sql string, args ...any) ([]contest.SimilarityPair, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	pairs := make([]contest.SimilarityPair, 0)
	for rows.Next() {
		var p contest.SimilarityPair
		if err := rows.Scan(
			&p.AssessmentID, &p.QuestionIndex, &p.AttemptA, &p.AttemptB,
			&p.Similarity, &p.MatchedTokens, &p.SnippetA, &p.SnippetB, &p.ComputedAt,
		); err != nil {
			return nil, err
		}
		pairs = append(pairs, p)
	}
	return pairs, rows.Err()
}

func queryAppeals(ctx context.Context, q Querier, sql string, args ...any) ([]contest.Appeal, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	appeals := make([]contest.Appeal, 0)
	for rows.Next() {
		var ap contest.Appeal
		if err := rows.Scan(
			&ap.ID, &ap.AttemptID, &ap.StudentID, &ap.StudentName, &ap.Kind, &ap.Message,
			&ap.Status, &ap.ReviewerID, &ap.Resolution, &ap.CreatedAt, &ap.ResolvedAt,
		); err != nil {
			return nil, err
		}
		appeals = append(appeals, ap)
	}
	return appeals, rows.Err()
}
