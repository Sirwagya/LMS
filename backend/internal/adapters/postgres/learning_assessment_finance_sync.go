package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"maps"
	"strconv"
	"time"

	"vedam/lms/internal/domain/assessment"
	"vedam/lms/internal/domain/errs"
	"vedam/lms/internal/domain/finance"
	"vedam/lms/internal/domain/learning"
	syncdomain "vedam/lms/internal/domain/sync"
)

type AttendanceRepo struct{ tx *TxManager }

func NewAttendanceRepo(tx *TxManager) *AttendanceRepo { return &AttendanceRepo{tx: tx} }

func (r *AttendanceRepo) GetForCourse(ctx context.Context, userID, courseCode string) (*learning.AttendanceSummary, error) {
	var a learning.AttendanceSummary
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT user_id, course_code, attended, total FROM attendance_summaries
		WHERE user_id = $1 AND course_code = $2
	`, userID, courseCode).Scan(&a.UserID, &a.CourseCode, &a.Attended, &a.Total)
	if err != nil {
		return nil, mapPGError(err)
	}
	return &a, nil
}

func (r *AttendanceRepo) ListForStudent(ctx context.Context, userID string) (map[string]learning.AttendanceSummary, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT user_id, course_code, attended, total FROM attendance_summaries WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	out := make(map[string]learning.AttendanceSummary)
	for rows.Next() {
		var a learning.AttendanceSummary
		if err := rows.Scan(&a.UserID, &a.CourseCode, &a.Attended, &a.Total); err != nil {
			return nil, err
		}
		out[a.CourseCode] = a
	}
	return out, rows.Err()
}

func (r *AttendanceRepo) ListAll(ctx context.Context) ([]learning.AttendanceSummary, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `SELECT user_id, course_code, attended, total FROM attendance_summaries`)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []learning.AttendanceSummary
	for rows.Next() {
		var a learning.AttendanceSummary
		if err := rows.Scan(&a.UserID, &a.CourseCode, &a.Attended, &a.Total); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *AttendanceRepo) Upsert(ctx context.Context, s learning.AttendanceSummary) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO attendance_summaries (user_id, course_code, attended, total, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (user_id, course_code) DO UPDATE
		SET attended = EXCLUDED.attended, total = EXCLUDED.total, updated_at = NOW()
	`, s.UserID, s.CourseCode, s.Attended, s.Total)
	return mapPGError(err)
}

type ProgressRepo struct{ tx *TxManager }

func NewProgressRepo(tx *TxManager) *ProgressRepo { return &ProgressRepo{tx: tx} }

func (r *ProgressRepo) ListCompletedLectures(ctx context.Context, userID string) ([]int64, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `SELECT lecture_id FROM lecture_progress WHERE user_id = $1 ORDER BY lecture_id ASC`, userID)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *ProgressRepo) MarkComplete(ctx context.Context, userID string, lectureID int64) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `INSERT INTO lecture_progress (user_id, lecture_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, userID, lectureID)
	return mapPGError(err)
}

func (r *ProgressRepo) MarkIncomplete(ctx context.Context, userID string, lectureID int64) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `DELETE FROM lecture_progress WHERE user_id = $1 AND lecture_id = $2`, userID, lectureID)
	return mapPGError(err)
}

type DiscussionRepo struct{ tx *TxManager }

func NewDiscussionRepo(tx *TxManager) *DiscussionRepo { return &DiscussionRepo{tx: tx} }

func (r *DiscussionRepo) ListForLecture(ctx context.Context, lectureID int64) ([]learning.Discussion, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT d.id, d.user_id, d.lecture_id, u.name, d.body, to_char(d.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'), d.created_at
		FROM discussions d JOIN users u ON u.id = d.user_id
		WHERE d.lecture_id = $1 ORDER BY d.created_at ASC
	`, lectureID)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []learning.Discussion
	for rows.Next() {
		var d learning.Discussion
		if err := rows.Scan(&d.ID, &d.UserID, &d.LectureID, &d.AuthorName, &d.Body, &d.Created, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *DiscussionRepo) GetByID(ctx context.Context, id string) (learning.Discussion, error) {
	var d learning.Discussion
	err := r.tx.Querier(ctx).QueryRow(ctx, `
   SELECT d.id,d.user_id,d.lecture_id,u.name,d.body,to_char(d.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),d.created_at
   FROM discussions d JOIN users u ON u.id=d.user_id WHERE d.id=$1
 `, id).Scan(&d.ID, &d.UserID, &d.LectureID, &d.AuthorName, &d.Body, &d.Created, &d.CreatedAt)
	return d, mapPGError(err)
}

func (r *DiscussionRepo) Create(ctx context.Context, d *learning.Discussion) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO discussions (id, user_id, lecture_id, body, created_at)
		VALUES (COALESCE(NULLIF($1, ''), gen_random_uuid()::text), $2, $3, $4, COALESCE(NULLIF($5::timestamptz, '-infinity'), NOW()))
	`, d.ID, d.UserID, d.LectureID, d.Body, d.CreatedAt)
	return mapPGError(err)
}

type AssignmentRepo struct{ tx *TxManager }

func NewAssignmentRepo(tx *TxManager) *AssignmentRepo { return &AssignmentRepo{tx: tx} }

func (r *AssignmentRepo) CreateWritten(ctx context.Context, id string, lectureID int64, courseCode, title, dueDate string) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO assignments (id, lecture_id, course_code, kind, title, due_date)
		VALUES ($1, $2, $3, 'written', $4, $5)
	`, id, lectureID, courseCode, title, dueDate)
	return mapPGError(err)
}

func (r *AssignmentRepo) CreateCoding(ctx context.Context, id string, lectureID int64, courseCode, title, dueDate string, problem assessment.CodingProblemSpec) error {
	starterJSON, err := json.Marshal(problem.StarterCode)
	if err != nil {
		return err
	}
	return r.tx.WithinTx(ctx, func(txCtx context.Context) error {
		q := r.tx.Querier(txCtx)
		if _, err := q.Exec(txCtx, `
			INSERT INTO assignments (id, lecture_id, course_code, kind, title, due_date)
			VALUES ($1, $2, $3, 'coding', $4, $5)
		`, id, lectureID, courseCode, title, dueDate); err != nil {
			return mapPGError(err)
		}
		if _, err := q.Exec(txCtx, `
			INSERT INTO coding_problems (assignment_id, entrypoint, description, input_format, output_format, constraints, difficulty, starter_code)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, id, problem.Entrypoint, problem.Description, problem.InputFormat, problem.OutputFormat, problem.Constraints, problem.Difficulty, starterJSON); err != nil {
			return mapPGError(err)
		}
		for i, tc := range problem.TestCases {
			ordinal := tc.Ordinal
			if ordinal == 0 {
				ordinal = i + 1
			}
			if _, err := q.Exec(txCtx, `
				INSERT INTO coding_test_cases (assignment_id, ordinal, input_data, expected_output, is_hidden)
				VALUES ($1, $2, $3, $4, $5)
			`, id, ordinal, tc.Input, tc.Expected, tc.Hidden); err != nil {
				return mapPGError(err)
			}
		}
		return nil
	})
}

func (r *AssignmentRepo) GetPublicByID(ctx context.Context, assignmentID string) (*assessment.PublicAssignment, error) {
	sa, err := r.GetWithHiddenTests(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	pub := sa.PublicAssignment
	return &pub, nil
}

func (r *AssignmentRepo) ListAllPublic(ctx context.Context) ([]assessment.PublicAssignment, error) {
	all, err := r.ListAllWithHiddenTests(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]assessment.PublicAssignment, 0, len(all))
	for _, sa := range all {
		out = append(out, sa.PublicAssignment)
	}
	return out, nil
}

func (r *AssignmentRepo) GetWithHiddenTests(ctx context.Context, assignmentID string) (*assessment.StaffAssignment, error) {
	var sa assessment.StaffAssignment
	var kind string
	var ep, desc, infmt, outfmt, constr, diff *string
	var starterRaw []byte
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT a.id, a.lecture_id, a.course_code, a.kind, a.title, a.due_date,
		       cp.entrypoint, cp.description, cp.input_format, cp.output_format, cp.constraints, cp.difficulty, cp.starter_code
		FROM assignments a
		LEFT JOIN coding_problems cp ON cp.assignment_id = a.id
		WHERE a.id = $1
	`, assignmentID).Scan(&sa.ID, &sa.LectureID, &sa.CourseCode, &kind, &sa.Title, &sa.DueDate, &ep, &desc, &infmt, &outfmt, &constr, &diff, &starterRaw)
	if err != nil {
		return nil, mapPGError(err)
	}
	sa.Kind = assessment.AssignmentKind(kind)
	if sa.Kind == assessment.KindCoding && ep != nil {
		sa.Entrypoint = *ep
		sa.Description = *desc
		sa.InputFormat = *infmt
		sa.OutputFmt = *outfmt
		sa.Constraints = *constr
		sa.Difficulty = *diff
		_ = json.Unmarshal(starterRaw, &sa.StarterCode)

		tcRows, err := r.tx.Querier(ctx).Query(ctx, `
			SELECT id, input_data, expected_output, is_hidden, ordinal
			FROM coding_test_cases WHERE assignment_id = $1 ORDER BY ordinal ASC
		`, assignmentID)
		if err != nil {
			return nil, mapPGError(err)
		}
		defer tcRows.Close()
		for tcRows.Next() {
			var tc assessment.TestCase
			if err := tcRows.Scan(&tc.ID, &tc.Input, &tc.Expected, &tc.Hidden, &tc.Ordinal); err != nil {
				return nil, err
			}
			sa.AllTestCases = append(sa.AllTestCases, tc)
			if !tc.Hidden {
				sa.Examples = append(sa.Examples, assessment.TestCase{Input: tc.Input, Expected: tc.Expected})
			}
		}
		sa.TotalTests = len(sa.AllTestCases)
	}
	return &sa, nil
}

func (r *AssignmentRepo) ListAllWithHiddenTests(ctx context.Context) ([]assessment.StaffAssignment, error) {
	tcByAssign := make(map[string][]assessment.TestCase)
	tcRows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT id, assignment_id, input_data, expected_output, is_hidden, ordinal
		FROM coding_test_cases ORDER BY assignment_id ASC, ordinal ASC
	`)
	if err != nil {
		return nil, mapPGError(err)
	}
	for tcRows.Next() {
		var aid string
		var tc assessment.TestCase
		if err := tcRows.Scan(&tc.ID, &aid, &tc.Input, &tc.Expected, &tc.Hidden, &tc.Ordinal); err != nil {
			tcRows.Close()
			return nil, err
		}
		tcByAssign[aid] = append(tcByAssign[aid], tc)
	}
	tcRows.Close()

	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT a.id, a.lecture_id, a.course_code, a.kind, a.title, a.due_date,
		       cp.entrypoint, cp.description, cp.input_format, cp.output_format, cp.constraints, cp.difficulty, cp.starter_code
		FROM assignments a
		LEFT JOIN coding_problems cp ON cp.assignment_id = a.id
		ORDER BY a.id ASC
	`)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []assessment.StaffAssignment
	for rows.Next() {
		var sa assessment.StaffAssignment
		var kind string
		var ep, desc, infmt, outfmt, constr, diff *string
		var starterRaw []byte
		if err := rows.Scan(&sa.ID, &sa.LectureID, &sa.CourseCode, &kind, &sa.Title, &sa.DueDate, &ep, &desc, &infmt, &outfmt, &constr, &diff, &starterRaw); err != nil {
			return nil, err
		}
		sa.Kind = assessment.AssignmentKind(kind)
		if sa.Kind == assessment.KindCoding && ep != nil {
			sa.Entrypoint = *ep
			sa.Description = *desc
			sa.InputFormat = *infmt
			sa.OutputFmt = *outfmt
			sa.Constraints = *constr
			sa.Difficulty = *diff
			_ = json.Unmarshal(starterRaw, &sa.StarterCode)
			sa.StarterCode = maps.Clone(sa.StarterCode)
			sa.AllTestCases = tcByAssign[sa.ID]
			sa.TotalTests = len(sa.AllTestCases)
			for _, tc := range sa.AllTestCases {
				if !tc.Hidden {
					sa.Examples = append(sa.Examples, assessment.TestCase{Input: tc.Input, Expected: tc.Expected})
				}
			}
		}
		out = append(out, sa)
	}
	return out, rows.Err()
}

type SubmissionRepo struct{ tx *TxManager }

func NewSubmissionRepo(tx *TxManager) *SubmissionRepo { return &SubmissionRepo{tx: tx} }

func (r *SubmissionRepo) ListForStudent(ctx context.Context, userID string) ([]assessment.Submission, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT id, assignment_id, course_code, answer, filename, asset_id, score, feedback,
		       to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
		FROM submissions WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []assessment.Submission
	for rows.Next() {
		var s assessment.Submission
		if err := rows.Scan(&s.ID, &s.AssignmentID, &s.Course, &s.Answer, &s.Filename, &s.AssetID, &s.Score, &s.Feedback, &s.Created); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SubmissionRepo) ListAll(ctx context.Context) ([]assessment.Submission, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT s.id, s.user_id, u.name, s.assignment_id, s.course_code, s.answer, s.filename, s.asset_id, s.score, s.feedback,
		       to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
		FROM submissions s JOIN users u ON u.id = s.user_id ORDER BY s.created_at DESC
	`)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []assessment.Submission
	for rows.Next() {
		var s assessment.Submission
		if err := rows.Scan(&s.ID, &s.UserID, &s.StudentName, &s.AssignmentID, &s.Course, &s.Answer, &s.Filename, &s.AssetID, &s.Score, &s.Feedback, &s.Created); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SubmissionRepo) GetByID(ctx context.Context, id string) (*assessment.Submission, error) {
	var s assessment.Submission
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT id, user_id, assignment_id, course_code, answer, filename, asset_id, score, feedback,
		       to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
		FROM submissions WHERE id = $1
	`, id).Scan(&s.ID, &s.UserID, &s.AssignmentID, &s.Course, &s.Answer, &s.Filename, &s.AssetID, &s.Score, &s.Feedback, &s.Created)
	if err != nil {
		return nil, mapPGError(err)
	}
	return &s, nil
}

func (r *SubmissionRepo) GetByUserAndAssignment(ctx context.Context, userID, assignmentID string) (*assessment.Submission, error) {
	var s assessment.Submission
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT id, user_id, assignment_id, course_code, answer, filename, asset_id, score, feedback,
		       to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
		FROM submissions WHERE user_id = $1 AND assignment_id = $2
	`, userID, assignmentID).Scan(&s.ID, &s.UserID, &s.AssignmentID, &s.Course, &s.Answer, &s.Filename, &s.AssetID, &s.Score, &s.Feedback, &s.Created)
	if err != nil {
		return nil, mapPGError(err)
	}
	return &s, nil
}

func (r *SubmissionRepo) Upsert(ctx context.Context, sub *assessment.Submission) error {
	tag, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO submissions (id, user_id, assignment_id, course_code, answer, filename, asset_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (user_id, assignment_id) DO UPDATE
		SET answer = EXCLUDED.answer, filename = EXCLUDED.filename, asset_id = EXCLUDED.asset_id, created_at = NOW(), updated_at = NOW()
        WHERE submissions.score IS NULL OR submissions.score = '' OR EXISTS (SELECT 1 FROM assignments WHERE id=EXCLUDED.assignment_id AND kind='coding')
	`, sub.ID, sub.UserID, sub.AssignmentID, sub.Course, sub.Answer, sub.Filename, sub.AssetID)
	if err == nil && tag.RowsAffected() == 0 {
		return errs.New(errs.ErrAlreadyGraded, "ALREADY_GRADED", "Graded submissions cannot be changed.")
	}
	return mapPGError(err)
}

func (r *SubmissionRepo) Grade(ctx context.Context, submissionID, score, feedback string) error {
	tag, err := r.tx.Querier(ctx).Exec(ctx, `
		UPDATE submissions SET score = $1, feedback = $2, updated_at = NOW() WHERE id = $3
	`, score, feedback, submissionID)
	if err != nil {
		return mapPGError(err)
	}
	if tag.RowsAffected() == 0 {
		return errs.ErrNotFound
	}
	return nil
}

type CodingAttemptRepo struct{ tx *TxManager }

func NewCodingAttemptRepo(tx *TxManager) *CodingAttemptRepo { return &CodingAttemptRepo{tx: tx} }

func (r *CodingAttemptRepo) Save(ctx context.Context, a *assessment.CodingAttempt) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO coding_attempts (id, user_id, assignment_id, language, source, verdict, passed, total, result_json, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, COALESCE(NULLIF($10::timestamptz, '-infinity'), NOW()))
	`, a.ID, a.UserID, a.AssignmentID, a.Language, a.Source, a.Verdict, a.Passed, a.Total, []byte(a.Result), a.CreatedAt)
	return mapPGError(err)
}

func (r *CodingAttemptRepo) History(ctx context.Context, userID, assignmentID string, limit int) ([]assessment.CodingAttempt, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT id, language, source, verdict, passed, total, result_json,
		       to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'), created_at
		FROM coding_attempts WHERE user_id = $1 AND assignment_id = $2
		ORDER BY created_at DESC LIMIT $3
	`, userID, assignmentID, limit)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []assessment.CodingAttempt
	for rows.Next() {
		var a assessment.CodingAttempt
		if err := rows.Scan(&a.ID, &a.Language, &a.Source, &a.Verdict, &a.Passed, &a.Total, &a.Result, &a.Created, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type ContestRepo struct{ tx *TxManager }

func NewContestRepo(tx *TxManager) *ContestRepo { return &ContestRepo{tx: tx} }

func (r *ContestRepo) ListSummariesForStudent(ctx context.Context, userID string) (map[string]assessment.ContestSummaryItem, error) {
	out := make(map[string]assessment.ContestSummaryItem)
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT course_code, percent, detail, completed, total, best_rank
		FROM contest_summaries WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, mapPGError(err)
	}
	for rows.Next() {
		var code string
		var item assessment.ContestSummaryItem
		var bestRank *string
		if err := rows.Scan(&code, &item.Percent, &item.Detail, &item.Completed, &item.Total, &bestRank); err != nil {
			rows.Close()
			return nil, err
		}
		if bestRank != nil {
			item.BestRank = *bestRank
		}
		out[code] = item
	}
	rows.Close()

	attRows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT course_code, label, title, contest_date, score, percent, rank_label
		FROM contest_attempts WHERE user_id = $1 ORDER BY course_code ASC, ordinal ASC
	`, userID)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer attRows.Close()
	for attRows.Next() {
		var code string
		var att assessment.ContestAttempt
		if err := attRows.Scan(&code, &att.Label, &att.Title, &att.Date, &att.Score, &att.Percent, &att.Rank); err != nil {
			return nil, err
		}
		item := out[code]
		item.History = append(item.History, att)
		out[code] = item
	}
	return out, attRows.Err()
}

func (r *ContestRepo) ListRecentForStudent(ctx context.Context, userID string) ([]assessment.ContestRecord, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT id, title, course_code, contest_date, score, percent, rank_label, status
		FROM recent_contests WHERE user_id = $1 ORDER BY ordinal ASC
	`, userID)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []assessment.ContestRecord
	for rows.Next() {
		var c assessment.ContestRecord
		if err := rows.Scan(&c.ID, &c.Title, &c.SubjectCode, &c.Date, &c.Score, &c.Percent, &c.Rank, &c.Status); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type FinanceRepo struct{ tx *TxManager }

func NewFinanceRepo(tx *TxManager) *FinanceRepo { return &FinanceRepo{tx: tx} }

func (r *FinanceRepo) Create(ctx context.Context, inv *finance.FeeInvoice) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO fee_invoices (id, user_id, invoice_no, description, semester, amount, due_date, paid_date, status, receipt_ready, simulated)
		VALUES (COALESCE(NULLIF($1, ''), gen_random_uuid()::text), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, inv.ID, inv.UserID, inv.InvoiceNo, inv.Description, inv.Semester, inv.Amount, inv.DueDate, inv.PaidDate, string(inv.Status), inv.ReceiptAvailable, inv.Simulated)
	return mapPGError(err)
}

func (r *FinanceRepo) ListForStudent(ctx context.Context, userID string) ([]finance.FeeInvoice, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT id, invoice_no, description, semester, amount, due_date, paid_date, status, receipt_ready, simulated, version
		FROM fee_invoices WHERE user_id = $1 ORDER BY id ASC
	`, userID)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []finance.FeeInvoice
	for rows.Next() {
		var inv finance.FeeInvoice
		var st string
		if err := rows.Scan(&inv.ID, &inv.InvoiceNo, &inv.Description, &inv.Semester, &inv.Amount, &inv.DueDate, &inv.PaidDate, &st, &inv.ReceiptAvailable, &inv.Simulated, &inv.Version); err != nil {
			return nil, err
		}
		inv.Status = finance.InvoiceStatus(st)
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (r *FinanceRepo) ListAll(ctx context.Context) ([]finance.FeeInvoice, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT id, user_id, invoice_no, description, semester, amount, due_date, paid_date, status, receipt_ready, simulated, version
		FROM fee_invoices ORDER BY id ASC
	`)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []finance.FeeInvoice
	for rows.Next() {
		var inv finance.FeeInvoice
		var st string
		if err := rows.Scan(&inv.ID, &inv.UserID, &inv.InvoiceNo, &inv.Description, &inv.Semester, &inv.Amount, &inv.DueDate, &inv.PaidDate, &st, &inv.ReceiptAvailable, &inv.Simulated, &inv.Version); err != nil {
			return nil, err
		}
		inv.Status = finance.InvoiceStatus(st)
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (r *FinanceRepo) GetOwned(ctx context.Context, feeID, userID string) (*finance.FeeInvoice, error) {
	var inv finance.FeeInvoice
	var st string
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT id, user_id, invoice_no, description, semester, amount, due_date, paid_date, status, receipt_ready, simulated, version
		FROM fee_invoices WHERE id = $1 AND user_id = $2
	`, feeID, userID).Scan(&inv.ID, &inv.UserID, &inv.InvoiceNo, &inv.Description, &inv.Semester, &inv.Amount, &inv.DueDate, &inv.PaidDate, &st, &inv.ReceiptAvailable, &inv.Simulated, &inv.Version)
	if err != nil {
		return nil, mapPGError(err)
	}
	inv.Status = finance.InvoiceStatus(st)
	return &inv, nil
}

func (r *FinanceRepo) MarkPaid(ctx context.Context, feeID, paidDate string) error {
	tag, err := r.tx.Querier(ctx).Exec(ctx, `
		UPDATE fee_invoices
		SET status = 'Paid', paid_date = $1, receipt_ready = TRUE, simulated = TRUE, version = version + 1, updated_at = NOW()
		WHERE id = $2
	`, paidDate, feeID)
	if err != nil {
		return mapPGError(err)
	}
	if tag.RowsAffected() == 0 {
		return errs.ErrNotFound
	}
	return nil
}

func (r *FinanceRepo) Record(ctx context.Context, p *finance.PaymentRecord) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO payments (id, user_id, fee_id, idempotency_key, created_at)
		VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5::timestamptz, '-infinity'), NOW()))
	`, p.ID, p.UserID, p.FeeID, p.IdempotencyKey, p.CreatedAt)
	return mapPGError(err)
}

type SyncRepo struct{ tx *TxManager }

func NewSyncRepo(tx *TxManager) *SyncRepo { return &SyncRepo{tx: tx} }

func (r *SyncRepo) EnqueueEvent(ctx context.Context, event syncdomain.OutboxEvent) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO outbox_events (id, aggregate_id, topic, payload, created_at)
		VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5::timestamptz, '-infinity'), NOW()))
	`, event.ID, event.AggregateID, event.Topic, []byte(event.Payload), event.CreatedAt)
	return mapPGError(err)
}

func (r *SyncRepo) AppendChangelog(ctx context.Context, scope string, targetUserID *string, entityType, entityID, op string, payload json.RawMessage) (int64, error) {
	var seq int64
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		INSERT INTO sync_changelog (scope, target_user_id, entity_type, entity_id, op, payload)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING seq
	`, scope, targetUserID, entityType, entityID, op, []byte(payload)).Scan(&seq)
	return seq, mapPGError(err)
}

func (r *SyncRepo) ClaimUnpublished(ctx context.Context, limit int) ([]syncdomain.OutboxEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT id, aggregate_id, topic, payload, created_at
		FROM outbox_events WHERE published_at IS NULL AND relay_failed_at IS NULL AND next_relay_at <= NOW()
        ORDER BY next_relay_at, created_at, id LIMIT $1 FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []syncdomain.OutboxEvent
	for rows.Next() {
		var e syncdomain.OutboxEvent
		if err := rows.Scan(&e.ID, &e.AggregateID, &e.Topic, &e.Payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *SyncRepo) MarkPublished(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.tx.Querier(ctx).Exec(ctx, `UPDATE outbox_events SET published_at = NOW() WHERE id = ANY($1)`, ids)
	return mapPGError(err)
}

func (r *SyncRepo) ReadSince(ctx context.Context, userID string, cursor int64, limit int) ([]syncdomain.ChangelogEntry, int64, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT seq, scope, entity_type, entity_id, op, payload, updated_at
		FROM sync_changelog
		WHERE seq > $1 AND (scope = 'global' OR target_user_id = $2)
		ORDER BY seq ASC LIMIT $3
	`, cursor, userID, limit)
	if err != nil {
		return nil, cursor, mapPGError(err)
	}
	defer rows.Close()
	var out []syncdomain.ChangelogEntry
	latest := cursor
	for rows.Next() {
		var e syncdomain.ChangelogEntry
		if err := rows.Scan(&e.Seq, &e.Scope, &e.EntityType, &e.EntityID, &e.Op, &e.Payload, &e.UpdatedAt); err != nil {
			return nil, cursor, err
		}
		out = append(out, e)
		latest = e.Seq
	}
	return out, latest, rows.Err()
}

func (r *SyncRepo) LookupMutation(ctx context.Context, key string) (json.RawMessage, bool, error) {
	var raw []byte
	err := r.tx.Querier(ctx).QueryRow(ctx, `SELECT response_json FROM processed_mutations WHERE idempotency_key = $1`, key).Scan(&raw)
	if err != nil {
		mapped := mapPGError(err)
		if mapped == errs.ErrNotFound {
			return nil, false, nil
		}
		return nil, false, mapped
	}
	return json.RawMessage(raw), true, nil
}

func (r *SyncRepo) SaveMutation(ctx context.Context, key, userID string, response json.RawMessage) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO processed_mutations (idempotency_key, user_id, response_json)
		VALUES ($1, $2, $3) ON CONFLICT (idempotency_key) DO NOTHING
	`, key, userID, []byte(response))
	return mapPGError(err)
}

var _ = time.Now

// Serialize changelog writers before allocating sequence values. Pull cursors
// only advance past committed entries, so they cannot skip a late commit.
func (r *SyncRepo) LockMutations(ctx context.Context) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock(24851927)`)
	return err
}

func (r *ProgressRepo) Version(ctx context.Context, userID string, lectureID int64) (int64, bool, int64, error) {
	var version, timestamp int64
	var complete bool
	err := r.tx.Querier(ctx).QueryRow(ctx, `
 SELECT seq, (payload->>'complete')::boolean,
 COALESCE((payload->>'clientTimestamp')::bigint, floor(extract(epoch FROM updated_at)*1000)::bigint) FROM sync_changelog
 WHERE target_user_id=$1 AND entity_type='lecture_progress' AND entity_id=$2
 ORDER BY seq DESC LIMIT 1`, userID, strconv.FormatInt(lectureID, 10)).Scan(&version, &complete, &timestamp)
	if errors.Is(err, pgx.ErrNoRows) {
		err = r.tx.Querier(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lecture_progress WHERE user_id=$1 AND lecture_id=$2)`, userID, lectureID).Scan(&complete)
	}
	return version, complete, timestamp, err
}

func (r *CodingAttemptRepo) Result(ctx context.Context, userID, id string) (json.RawMessage, error) {
	var result json.RawMessage
	err := r.tx.Querier(ctx).QueryRow(ctx, `SELECT result_json FROM coding_attempts WHERE id=$1 AND user_id=$2`, id, userID).Scan(&result)
	return result, mapPGError(err)
}

func (r *SyncRepo) DeferRelay(ctx context.Context, id, reason string, permanent bool) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
   UPDATE outbox_events SET relay_attempts=relay_attempts+1, relay_error=$2,
     relay_failed_at=CASE WHEN $3::boolean THEN NOW() ELSE NULL END,
     next_relay_at=NOW()+make_interval(secs => (1 << LEAST(relay_attempts,8)))
   WHERE id=$1
 `, id, reason, permanent)
	return mapPGError(err)
}
