package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"vedam/lms/internal/domain/contest"
	"vedam/lms/internal/domain/errs"
)

type ContestResultsRepo struct {
	tx *TxManager
}

func NewContestResultsRepo(tx *TxManager) *ContestResultsRepo {
	return &ContestResultsRepo{tx: tx}
}

var _ contest.ResultsRepository = (*ContestResultsRepo)(nil)

type submittedAttemptData struct {
	id            string
	studentID     string
	displayName   string
	startedAt     time.Time
	submittedAt   time.Time
	answers       map[int]contest.Answer
	choiceSavedAt map[int]time.Time
	reviewStatus  string
	reviewReason  string
}

type storedAttemptResult struct {
	version int
	score   contest.Score
}

func (r *ContestResultsRepo) PublishOrRegrade(ctx context.Context, actor contest.Actor, assessmentID string, cmd contest.PublishResultsInput) (contest.ReleaseRecord, error) {
	var release contest.ReleaseRecord
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return release, errs.ErrNotFound
	}

	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		var title, ownerID, status string
		var questionsJSON []byte
		var prevVersion int
		var isReviewer bool
		var publishedAt time.Time

		err := q.QueryRow(ctx, `
			SELECT a.title, a.owner_id, a.status, a.questions, a.latest_result_version,
			       EXISTS (
			           SELECT 1 FROM assessment_reviewers rv
			           WHERE rv.assessment_id = a.id AND rv.reviewer_id = $2
			       ) AS is_reviewer,
			       clock_timestamp()
			FROM assessments a
			WHERE a.id = $1::uuid
			FOR UPDATE OF a
		`, aid, actor.ID).Scan(&title, &ownerID, &status, &questionsJSON, &prevVersion, &isReviewer, &publishedAt)
		if err != nil {
			return mapPGError(err)
		}
		if !actor.Admin && ownerID != actor.ID && !isReviewer {
			return errs.ErrNotFound
		}
		if status != "published" {
			return errs.New(errs.ErrConflict, "CONTEST_NOT_PUBLISHED", "Results can only be published for a published contest.")
		}
		if err := contest.ValidatePublishInput(&cmd, prevVersion); err != nil {
			return err
		}

		var questions []contest.Question
		if err := json.Unmarshal(questionsJSON, &questions); err != nil {
			return err
		}
		possiblePoints := 0
		for _, question := range questions {
			possiblePoints += question.Points
		}
		if possiblePoints <= 0 {
			return contest.Invalid("Cannot publish results for a contest with no scored questions.")
		}

		// Finalize any overdue active attempts before capturing release state.
		if _, err := q.Exec(ctx, `
			UPDATE assessment_attempts
			SET submitted_at = deadline_at, finish_reason = 'expired'
			WHERE assessment_id = $1::uuid
			  AND submitted_at IS NULL
			  AND deadline_at <= $2
		`, aid, publishedAt); err != nil {
			return mapPGError(err)
		}

		if len(cmd.WithholdAttemptIDs) > 0 {
			var matched int
			if err := q.QueryRow(ctx, `
				SELECT count(*)
				FROM assessment_attempts
				WHERE assessment_id = $1::uuid AND id = ANY($2::uuid[])
			`, aid, cmd.WithholdAttemptIDs).Scan(&matched); err != nil {
				return mapPGError(err)
			}
			if matched != len(cmd.WithholdAttemptIDs) {
				return contest.Invalid("Withheld attempt IDs must belong to this contest.")
			}
		}

		attempts, err := loadSubmittedAttemptsForPublish(ctx, q, aid)
		if err != nil {
			return err
		}
		subsByAttempt, pendingByAttempt, err := loadCodingSubmissionsByAttempt(ctx, q, aid)
		if err != nil {
			return err
		}
		latestResults, err := loadLatestAttemptResults(ctx, q, aid)
		if err != nil {
			return err
		}

		explicitWithheld := make(map[string]bool, len(cmd.WithholdAttemptIDs))
		for _, id := range cmd.WithholdAttemptIDs {
			explicitWithheld[id] = true
		}

		effectiveScores := make(map[string]contest.Score, len(attempts))
		withheldSet := make(map[string]bool, len(cmd.WithholdAttemptIDs))
		for _, id := range cmd.WithholdAttemptIDs {
			withheldSet[id] = true
		}
		candidates := make([]contest.RankCandidate, 0, len(attempts))

		for _, att := range attempts {
			details := subsByAttempt[att.id]
			codingSubs := make([]contest.CodingSubmission, len(details))
			for i, d := range details {
				codingSubs[i] = contest.CodingSubmission{
					ID:          d.ID,
					Question:    d.Question,
					Language:    d.Language,
					SubmittedAt: d.SubmittedAt,
					Status:      d.Status,
				}
			}

			fresh := contest.ScoreAttempt(att.id, att.startedAt, att.submittedAt, questions, att.answers, att.choiceSavedAt, codingSubs)
			var effective contest.Score
			if pendingByAttempt[att.id] {
				// Leave score_computed_at untouched while official judge jobs are still in flight.
				effective = contest.Score{
					Status:   "needs_review",
					Possible: fresh.Possible,
					Details:  fresh.Details,
				}
			} else {
				prevRes, hasPrev := latestResults[att.id]
				if !hasPrev || !scoresEqual(prevRes.score, fresh) {
					nextAttemptVer := 1
					if hasPrev {
						nextAttemptVer = prevRes.version + 1
					}
					detailsJSON, err := json.Marshal(fresh.Details)
					if err != nil {
						return err
					}
					if _, err := q.Exec(ctx, `
						WITH inserted AS (
							INSERT INTO assessment_attempt_results (
								attempt_id, version, status, points, possible_points, elapsed_ms, question_scores
							) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::jsonb)
							RETURNING attempt_id, computed_at
						)
						UPDATE assessment_attempts p
						SET score_computed_at = inserted.computed_at
						FROM inserted
						WHERE p.id = inserted.attempt_id
					`, att.id, nextAttemptVer, fresh.Status, fresh.Points, fresh.Possible, fresh.Elapsed, detailsJSON); err != nil {
						return mapPGError(err)
					}
					effective = fresh
				} else {
					effective = prevRes.score
				}
			}
			effectiveScores[att.id] = effective

			isWithheld := explicitWithheld[att.id] ||
				att.reviewStatus == "under_review" ||
				att.reviewStatus == "confirmed_violation" ||
				effective.Status == "needs_review"

			if isWithheld {
				withheldSet[att.id] = true
				continue
			}
			if effective.Status == "ready" && effective.Points != nil && effective.Elapsed != nil {
				candidates = append(candidates, contest.RankCandidate{
					AttemptID:      att.id,
					StudentID:      att.studentID,
					DisplayName:    att.displayName,
					Points:         *effective.Points,
					PossiblePoints: effective.Possible,
					ElapsedMs:      *effective.Elapsed,
				})
			}
		}

		ranked := contest.RankLeaderboard(candidates)
		totalRanked := len(ranked)
		rankByAttempt := make(map[string]int, len(ranked))
		for _, entry := range ranked {
			rankByAttempt[entry.AttemptID] = entry.Rank
		}

		nextVersion := prevVersion + 1
		withheldIDs := make([]string, 0, len(withheldSet))
		for id := range withheldSet {
			withheldIDs = append(withheldIDs, id)
		}
		slices.Sort(withheldIDs)
		withheldJSON, err := json.Marshal(withheldIDs)
		if err != nil {
			return err
		}

		if _, err := q.Exec(ctx, `
			UPDATE assessments
			SET latest_result_version = $2,
			    release_explanations = $3,
			    results_published_at = $4
			WHERE id = $1::uuid
		`, aid, nextVersion, cmd.ReleaseExplanations, publishedAt); err != nil {
			return mapPGError(err)
		}

		if _, err := q.Exec(ctx, `
			INSERT INTO assessment_result_releases (
				assessment_id, version, published_by, reason, release_explanations, withheld_attempt_ids, published_at
			) VALUES ($1::uuid, $2, $3, $4, $5, $6::jsonb, $7)
		`, aid, nextVersion, actor.ID, cmd.Reason, cmd.ReleaseExplanations, withheldJSON, publishedAt); err != nil {
			return mapPGError(err)
		}

		for _, entry := range ranked {
			if _, err := q.Exec(ctx, `
				INSERT INTO assessment_leaderboard_entries (
					assessment_id, version, attempt_id, student_id, rank, display_name, points, possible_points, elapsed_ms
				) VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, $8, $9)
			`, aid, nextVersion, entry.AttemptID, entry.StudentID, entry.Rank, entry.DisplayName, entry.Points, entry.PossiblePoints, entry.ElapsedMs); err != nil {
				return mapPGError(err)
			}
		}

		for _, att := range attempts {
			var rankPtr *int
			isWithheld := withheldSet[att.id]
			if !isWithheld {
				if rVal, ok := rankByAttempt[att.id]; ok {
					rankCopy := rVal
					rankPtr = &rankCopy
				}
			}
			report := contest.BuildStudentReport(contest.ReportBuildInput{
				AssessmentID:        aid,
				Title:               title,
				Version:             nextVersion,
				AttemptID:           att.id,
				PublishedAt:         publishedAt,
				ReleaseExplanations: cmd.ReleaseExplanations,
				Withheld:            isWithheld,
				Rank:                rankPtr,
				TotalRanked:         totalRanked,
				ReviewStatus:        att.reviewStatus,
				ReviewNote:          att.reviewReason,
				Questions:           questions,
				Answers:             att.answers,
				Score:               effectiveScores[att.id],
				Submissions:         subsByAttempt[att.id],
			})
			reportJSON, err := json.Marshal(report)
			if err != nil {
				return err
			}
			if _, err := q.Exec(ctx, `
				INSERT INTO assessment_student_reports (
					assessment_id, version, attempt_id, student_id, status, rank, total_ranked, report
				) VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, $8::jsonb)
			`, aid, nextVersion, att.id, att.studentID, report.Status, rankPtr, totalRanked, reportJSON); err != nil {
				return mapPGError(err)
			}
		}

		release = contest.ReleaseRecord{
			Version:             nextVersion,
			PublishedBy:         actor.ID,
			Reason:              cmd.Reason,
			ReleaseExplanations: cmd.ReleaseExplanations,
			WithheldAttemptIDs:  withheldIDs,
			PublishedAt:         publishedAt,
		}
		return nil
	})
	return release, err
}

func (r *ContestResultsRepo) GetStaffResults(ctx context.Context, actor contest.Actor, assessmentID string, page int) (contest.StaffResultsView, error) {
	var view contest.StaffResultsView
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return view, errs.ErrNotFound
	}
	if page < 0 {
		page = 0
	}

	err := r.tx.WithinReadSnapshot(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		var title string
		var latestVersion int
		var releaseExplanations bool
		var publishedAt *time.Time
		var allowed bool

		err := q.QueryRow(ctx, `
			SELECT a.title, a.latest_result_version, a.release_explanations, a.results_published_at,
			       (
			           $3::boolean
			           OR a.owner_id = $2
			           OR EXISTS (
			               SELECT 1 FROM assessment_reviewers rv
			               WHERE rv.assessment_id = a.id AND rv.reviewer_id = $2
			           )
			       ) AS allowed
			FROM assessments a
			WHERE a.id = $1::uuid
		`, aid, actor.ID, actor.Admin).Scan(&title, &latestVersion, &releaseExplanations, &publishedAt, &allowed)
		if err != nil {
			return mapPGError(err)
		}
		if !allowed {
			return errs.ErrNotFound
		}

		rows, err := q.Query(ctx, `
			SELECT version, published_by, reason, release_explanations, withheld_attempt_ids, published_at
			FROM assessment_result_releases
			WHERE assessment_id = $1::uuid
			ORDER BY version DESC
		`, aid)
		if err != nil {
			return mapPGError(err)
		}
		defer rows.Close()

		releases := make([]contest.ReleaseRecord, 0)
		for rows.Next() {
			var rel contest.ReleaseRecord
			var withheldJSON []byte
			if err := rows.Scan(&rel.Version, &rel.PublishedBy, &rel.Reason, &rel.ReleaseExplanations, &withheldJSON, &rel.PublishedAt); err != nil {
				return err
			}
			rel.WithheldAttemptIDs = make([]string, 0)
			if len(withheldJSON) > 0 {
				if err := json.Unmarshal(withheldJSON, &rel.WithheldAttemptIDs); err != nil {
					return err
				}
			}
			releases = append(releases, rel)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		lb := contest.LeaderboardPage{
			AssessmentID: aid,
			Title:        title,
			Version:      latestVersion,
			Page:         page,
			PageSize:     contest.LeaderboardPageSize,
			Items:        make([]contest.LeaderboardRow, 0),
		}
		if latestVersion > 0 && publishedAt != nil {
			lb, err = fetchLeaderboardPage(ctx, q, aid, title, latestVersion, *publishedAt, actor.ID, true, page)
			if err != nil {
				return err
			}
		}

		view = contest.StaffResultsView{
			AssessmentID:        aid,
			LatestVersion:       latestVersion,
			ReleaseExplanations: releaseExplanations,
			Releases:            releases,
			Leaderboard:         lb,
		}
		return nil
	})
	return view, err
}

func (r *ContestResultsRepo) GetStudentReport(ctx context.Context, assessmentID, studentID string) (contest.StudentReport, error) {
	var report contest.StudentReport
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return report, errs.ErrNotFound
	}

	var reportJSON []byte
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT sr.report
		FROM assessments a
		JOIN assessment_roster r ON r.assessment_id = a.id AND r.student_id = $2
		JOIN assessment_student_reports sr
		  ON sr.assessment_id = a.id
		 AND sr.version = a.latest_result_version
		 AND sr.student_id = r.student_id
		WHERE a.id = $1::uuid
		  AND a.latest_result_version > 0
	`, aid, studentID).Scan(&reportJSON)
	if err != nil {
		return report, mapPGError(err)
	}
	if err := json.Unmarshal(reportJSON, &report); err != nil {
		return report, err
	}
	return report, nil
}

func (r *ContestResultsRepo) GetLeaderboard(ctx context.Context, assessmentID, viewerID string, isStaff bool, page int) (contest.LeaderboardPage, error) {
	var lb contest.LeaderboardPage
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return lb, errs.ErrNotFound
	}
	if page < 0 {
		page = 0
	}

	err := r.tx.WithinReadSnapshot(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		var title string
		var latestVersion int
		var publishedAt *time.Time
		var allowed bool

		err := q.QueryRow(ctx, `
			SELECT a.title, a.latest_result_version, a.results_published_at,
			       CASE
			           WHEN $3::boolean THEN (
			               a.owner_id = $2
			               OR EXISTS (
			                   SELECT 1 FROM assessment_reviewers rv
			                   WHERE rv.assessment_id = a.id AND rv.reviewer_id = $2
			               )
			               OR EXISTS (
			                   SELECT 1 FROM users u
			                   WHERE u.id = $2 AND u.role = 'admin'
			               )
			           )
			           ELSE EXISTS (
			               SELECT 1 FROM assessment_roster r
			               WHERE r.assessment_id = a.id AND r.student_id = $2
			           )
			       END AS allowed
			FROM assessments a
			WHERE a.id = $1::uuid
		`, aid, viewerID, isStaff).Scan(&title, &latestVersion, &publishedAt, &allowed)
		if err != nil {
			return mapPGError(err)
		}
		if !allowed || latestVersion == 0 || publishedAt == nil {
			return errs.ErrNotFound
		}

		lb, err = fetchLeaderboardPage(ctx, q, aid, title, latestVersion, *publishedAt, viewerID, isStaff, page)
		return err
	})
	return lb, err
}

func (r *ContestResultsRepo) SetAlias(ctx context.Context, assessmentID, studentID string, alias string) error {
	aid, ok := parseCanonicalUUID(assessmentID)
	if !ok {
		return errs.ErrNotFound
	}
	cleanAlias, err := contest.ValidateAlias(alias)
	if err != nil {
		return err
	}

	return r.tx.WithinTx(ctx, func(ctx context.Context) error {
		q := r.tx.Querier(ctx)
		tag, err := q.Exec(ctx, `
			UPDATE assessment_roster
			SET display_alias = $3
			WHERE assessment_id = $1::uuid AND student_id = $2
		`, aid, studentID, cleanAlias)
		if err != nil {
			return mapPGError(err)
		}
		if tag.RowsAffected() != 1 {
			return errs.ErrNotFound
		}

		_, err = q.Exec(ctx, `
			UPDATE assessment_leaderboard_entries e
			SET display_name = COALESCE(NULLIF($3, ''), u.name)
			FROM assessments a, users u
			WHERE e.assessment_id = a.id
			  AND u.id = e.student_id
			  AND e.assessment_id = $1::uuid
			  AND e.version = a.latest_result_version
			  AND a.latest_result_version > 0
			  AND e.student_id = $2
		`, aid, studentID, cleanAlias)
		return mapPGError(err)
	})
}

func (r *ContestResultsRepo) ListStudentResultsSummary(ctx context.Context, studentID string) ([]contest.StudentResultSummary, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT a.id::text,
		       a.title,
		       sr.version,
		       sr.status,
		       sr.rank,
		       (sr.report->>'points')::int,
		       (sr.report->>'possiblePoints')::int,
		       a.results_published_at
		FROM assessment_roster r
		JOIN assessments a ON a.id = r.assessment_id
		JOIN assessment_student_reports sr
		  ON sr.assessment_id = a.id
		 AND sr.version = a.latest_result_version
		 AND sr.student_id = r.student_id
		WHERE r.student_id = $1
		  AND a.latest_result_version > 0
		  AND a.results_published_at IS NOT NULL
		ORDER BY a.results_published_at DESC, a.id DESC
	`, studentID)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()

	summaries := make([]contest.StudentResultSummary, 0)
	for rows.Next() {
		var item contest.StudentResultSummary
		if err := rows.Scan(
			&item.AssessmentID,
			&item.Title,
			&item.Version,
			&item.Status,
			&item.Rank,
			&item.Points,
			&item.PossiblePoints,
			&item.PublishedAt,
		); err != nil {
			return nil, err
		}
		summaries = append(summaries, item)
	}
	return summaries, rows.Err()
}

func fetchLeaderboardPage(ctx context.Context, q Querier, aid, title string, version int, publishedAt time.Time, viewerID string, isStaff bool, page int) (contest.LeaderboardPage, error) {
	pageSize := contest.LeaderboardPageSize
	offset := page * pageSize

	var totalRanked int
	if err := q.QueryRow(ctx, `
		SELECT count(*)
		FROM assessment_leaderboard_entries
		WHERE assessment_id = $1::uuid AND version = $2
	`, aid, version).Scan(&totalRanked); err != nil {
		return contest.LeaderboardPage{}, mapPGError(err)
	}

	rows, err := q.Query(ctx, `
		SELECT student_id, rank, display_name, points, possible_points, elapsed_ms
		FROM assessment_leaderboard_entries
		WHERE assessment_id = $1::uuid AND version = $2
		ORDER BY rank ASC, attempt_id ASC
		LIMIT $3 OFFSET $4
	`, aid, version, pageSize, offset)
	if err != nil {
		return contest.LeaderboardPage{}, mapPGError(err)
	}
	defer rows.Close()

	items := make([]contest.LeaderboardRow, 0)
	for rows.Next() {
		var rowStudentID string
		var row contest.LeaderboardRow
		if err := rows.Scan(&rowStudentID, &row.Rank, &row.DisplayName, &row.Points, &row.PossiblePoints, &row.ElapsedMs); err != nil {
			return contest.LeaderboardPage{}, err
		}
		row.IsSelf = rowStudentID == viewerID
		if isStaff || row.IsSelf {
			row.StudentID = rowStudentID
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return contest.LeaderboardPage{}, err
	}

	var selfRow *contest.LeaderboardRow
	if viewerID != "" {
		var rowStudentID string
		var sr contest.LeaderboardRow
		err := q.QueryRow(ctx, `
			SELECT student_id, rank, display_name, points, possible_points, elapsed_ms
			FROM assessment_leaderboard_entries
			WHERE assessment_id = $1::uuid AND version = $2 AND student_id = $3
			LIMIT 1
		`, aid, version, viewerID).Scan(&rowStudentID, &sr.Rank, &sr.DisplayName, &sr.Points, &sr.PossiblePoints, &sr.ElapsedMs)
		if err == nil {
			sr.IsSelf = true
			sr.StudentID = rowStudentID
			selfRow = &sr
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return contest.LeaderboardPage{}, mapPGError(err)
		}
	}

	return contest.LeaderboardPage{
		AssessmentID: aid,
		Title:        title,
		Version:      version,
		PublishedAt:  publishedAt,
		Page:         page,
		PageSize:     pageSize,
		TotalRanked:  totalRanked,
		HasMore:      offset+len(items) < totalRanked,
		Items:        items,
		SelfRow:      selfRow,
	}, nil
}

func loadSubmittedAttemptsForPublish(ctx context.Context, q Querier, aid string) ([]submittedAttemptData, error) {
	rows, err := q.Query(ctx, `
		SELECT p.id::text, p.student_id,
		       COALESCE(NULLIF(r.display_alias, ''), u.name) AS display_name,
		       p.started_at, p.submitted_at, p.answers, p.choice_saved_at,
		       COALESCE(rv.status, 'clear') AS review_status,
		       COALESCE(rv.reason, '') AS review_reason
		FROM assessment_attempts p
		JOIN assessment_roster r ON r.assessment_id = p.assessment_id AND r.student_id = p.student_id
		JOIN users u ON u.id = p.student_id
		LEFT JOIN assessment_reviews rv ON rv.attempt_id = p.id
		WHERE p.assessment_id = $1::uuid AND p.submitted_at IS NOT NULL
		ORDER BY p.id
		FOR UPDATE OF p
	`, aid)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()

	attempts := make([]submittedAttemptData, 0)
	for rows.Next() {
		var att submittedAttemptData
		var answersJSON, savedJSON []byte
		if err := rows.Scan(
			&att.id,
			&att.studentID,
			&att.displayName,
			&att.startedAt,
			&att.submittedAt,
			&answersJSON,
			&savedJSON,
			&att.reviewStatus,
			&att.reviewReason,
		); err != nil {
			return nil, err
		}
		att.answers = make(map[int]contest.Answer)
		if len(answersJSON) > 0 {
			if err := json.Unmarshal(answersJSON, &att.answers); err != nil {
				return nil, err
			}
		}
		att.choiceSavedAt = make(map[int]time.Time)
		if len(savedJSON) > 0 {
			if err := json.Unmarshal(savedJSON, &att.choiceSavedAt); err != nil {
				return nil, err
			}
		}
		attempts = append(attempts, att)
	}
	return attempts, rows.Err()
}

func loadCodingSubmissionsByAttempt(ctx context.Context, q Querier, aid string) (map[string][]contest.CodingSubmissionDetail, map[string]bool, error) {
	rows, err := q.Query(ctx, `
		SELECT c.id::text, c.attempt_id::text, c.question_index, c.language, c.source,
		       c.submitted_at, c.status, COALESCE(c.verdict, '')
		FROM assessment_coding_submissions c
		JOIN assessment_attempts p ON p.id = c.attempt_id
		WHERE p.assessment_id = $1::uuid AND p.submitted_at IS NOT NULL
		ORDER BY c.submitted_at DESC, c.id ASC
	`, aid)
	if err != nil {
		return nil, nil, mapPGError(err)
	}
	defer rows.Close()

	byAttempt := make(map[string][]contest.CodingSubmissionDetail)
	pending := make(map[string]bool)
	for rows.Next() {
		var attID string
		var sub contest.CodingSubmissionDetail
		if err := rows.Scan(&sub.ID, &attID, &sub.Question, &sub.Language, &sub.Source, &sub.SubmittedAt, &sub.Status, &sub.Verdict); err != nil {
			return nil, nil, err
		}
		byAttempt[attID] = append(byAttempt[attID], sub)
		if sub.Status == "queued" || sub.Status == "judging" {
			pending[attID] = true
		}
	}
	return byAttempt, pending, rows.Err()
}

func loadLatestAttemptResults(ctx context.Context, q Querier, aid string) (map[string]storedAttemptResult, error) {
	rows, err := q.Query(ctx, `
		SELECT DISTINCT ON (ar.attempt_id)
		       ar.attempt_id::text, ar.version, ar.status, ar.points, ar.possible_points,
		       ar.elapsed_ms, ar.question_scores
		FROM assessment_attempt_results ar
		JOIN assessment_attempts p ON p.id = ar.attempt_id
		WHERE p.assessment_id = $1::uuid AND p.submitted_at IS NOT NULL
		ORDER BY ar.attempt_id, ar.version DESC
	`, aid)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()

	results := make(map[string]storedAttemptResult)
	for rows.Next() {
		var attID string
		var item storedAttemptResult
		var detailsJSON []byte
		if err := rows.Scan(
			&attID,
			&item.version,
			&item.score.Status,
			&item.score.Points,
			&item.score.Possible,
			&item.score.Elapsed,
			&detailsJSON,
		); err != nil {
			return nil, err
		}
		if len(detailsJSON) > 0 {
			if err := json.Unmarshal(detailsJSON, &item.score.Details); err != nil {
				return nil, err
			}
		}
		results[attID] = item
	}
	return results, rows.Err()
}

func scoresEqual(a, b contest.Score) bool {
	if a.Status != b.Status || a.Possible != b.Possible {
		return false
	}
	if !equalPtr(a.Points, b.Points) || !equalPtr(a.Elapsed, b.Elapsed) {
		return false
	}
	return slices.Equal(a.Details, b.Details)
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
