package memory

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"vedam/lms/internal/domain/assessment"
	"vedam/lms/internal/domain/errs"
	"vedam/lms/internal/domain/finance"
	"vedam/lms/internal/domain/learning"
	syncdomain "vedam/lms/internal/domain/sync"
)

type AttendanceRepo struct{ s *Store }

func (s *Store) Attendance() *AttendanceRepo { return &AttendanceRepo{s: s} }

func (r *AttendanceRepo) GetForCourse(ctx context.Context, userID, courseCode string) (*learning.AttendanceSummary, error) {
	defer r.s.lockRead(ctx)()
	m, ok := r.s.active(ctx).attendance[userID]
	if !ok {
		return nil, errs.ErrNotFound
	}
	sum, ok := m[courseCode]
	if !ok {
		return nil, errs.ErrNotFound
	}
	return &sum, nil
}

func (r *AttendanceRepo) ListForStudent(ctx context.Context, userID string) (map[string]learning.AttendanceSummary, error) {
	defer r.s.lockRead(ctx)()
	out := make(map[string]learning.AttendanceSummary)
	if m, ok := r.s.active(ctx).attendance[userID]; ok {
		maps.Copy(out, m)
	}
	return out, nil
}

func (r *AttendanceRepo) ListAll(ctx context.Context) ([]learning.AttendanceSummary, error) {
	defer r.s.lockRead(ctx)()
	var out []learning.AttendanceSummary
	for _, m := range r.s.active(ctx).attendance {
		for _, v := range m {
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *AttendanceRepo) Upsert(ctx context.Context, summary learning.AttendanceSummary) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, ok := st.users[summary.UserID]; !ok {
		return errs.ErrNotFound
	}
	if _, ok := st.courses[summary.CourseCode]; !ok {
		return errs.ErrNotFound
	}
	if st.attendance[summary.UserID] == nil {
		st.attendance[summary.UserID] = make(map[string]learning.AttendanceSummary)
	}
	st.attendance[summary.UserID][summary.CourseCode] = summary
	return nil
}

type ProgressRepo struct{ s *Store }

func (s *Store) Progress() *ProgressRepo { return &ProgressRepo{s: s} }

func (r *ProgressRepo) ListCompletedLectures(ctx context.Context, userID string) ([]int64, error) {
	defer r.s.lockRead(ctx)()
	m := r.s.active(ctx).progress[userID]
	out := make([]int64, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	slices.Sort(out)
	return out, nil
}

func (r *ProgressRepo) MarkComplete(ctx context.Context, userID string, lectureID int64) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if st.progress[userID] == nil {
		st.progress[userID] = make(map[int64]struct{})
	}
	st.progress[userID][lectureID] = struct{}{}
	return nil
}

func (r *ProgressRepo) MarkIncomplete(ctx context.Context, userID string, lectureID int64) error {
	defer r.s.lockWrite(ctx)()
	if m := r.s.active(ctx).progress[userID]; m != nil {
		delete(m, lectureID)
	}
	return nil
}

type DiscussionRepo struct{ s *Store }

func (s *Store) Discussions() *DiscussionRepo { return &DiscussionRepo{s: s} }

func (r *DiscussionRepo) ListForLecture(ctx context.Context, lectureID int64) ([]learning.Discussion, error) {
	defer r.s.lockRead(ctx)()
	st := r.s.active(ctx)
	var out []learning.Discussion
	for _, d := range st.discussions {
		if d.LectureID == lectureID {
			if u, ok := st.users[d.UserID]; ok {
				d.AuthorName = u.Name
			}
			out = append(out, d)
		}
	}
	return out, nil
}

func (r *DiscussionRepo) Create(ctx context.Context, d *learning.Discussion) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	st.discussions = append(st.discussions, *d)
	return nil
}

type AssignmentRepo struct{ s *Store }

func (s *Store) Assignments() *AssignmentRepo { return &AssignmentRepo{s: s} }

func (r *AssignmentRepo) CreateWritten(ctx context.Context, id string, lectureID int64, courseCode, title, dueDate string) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, exists := st.writtenAssign[id]; exists {
		return errs.ErrConflict
	}
	if _, exists := st.codingAssign[id]; exists {
		return errs.ErrConflict
	}
	st.writtenAssign[id] = storedWrittenAssignment{id: id, lectureID: lectureID, courseCode: courseCode, title: title, dueDate: dueDate}
	return nil
}

func (r *AssignmentRepo) CreateCoding(ctx context.Context, id string, lectureID int64, courseCode, title, dueDate string, problem assessment.CodingProblemSpec) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, exists := st.writtenAssign[id]; exists {
		return errs.ErrConflict
	}
	if _, exists := st.codingAssign[id]; exists {
		return errs.ErrConflict
	}
	problem.TestCases = slices.Clone(problem.TestCases)
	problem.StarterCode = maps.Clone(problem.StarterCode)
	st.codingAssign[id] = storedCodingAssignment{id: id, lectureID: lectureID, courseCode: courseCode, title: title, dueDate: dueDate, problem: problem}
	return nil
}

func (r *AssignmentRepo) GetPublicByID(ctx context.Context, assignmentID string) (*assessment.PublicAssignment, error) {
	defer r.s.lockRead(ctx)()
	st := r.s.active(ctx)
	if w, ok := st.writtenAssign[assignmentID]; ok {
		return &assessment.PublicAssignment{
			ID:         w.id,
			LectureID:  w.lectureID,
			CourseCode: w.courseCode,
			Kind:       assessment.KindWritten,
			Title:      w.title,
			DueDate:    w.dueDate,
		}, nil
	}
	if c, ok := st.codingAssign[assignmentID]; ok {
		return toPublicCoding(c), nil
	}
	return nil, errs.ErrNotFound
}

func (r *AssignmentRepo) ListAllPublic(ctx context.Context) ([]assessment.PublicAssignment, error) {
	defer r.s.lockRead(ctx)()
	st := r.s.active(ctx)
	var out []assessment.PublicAssignment
	for _, w := range st.writtenAssign {
		out = append(out, assessment.PublicAssignment{
			ID: w.id, LectureID: w.lectureID, CourseCode: w.courseCode,
			Kind: assessment.KindWritten, Title: w.title, DueDate: w.dueDate,
		})
	}
	for _, c := range st.codingAssign {
		out = append(out, *toPublicCoding(c))
	}
	slices.SortFunc(out, func(a, b assessment.PublicAssignment) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func (r *AssignmentRepo) GetWithHiddenTests(ctx context.Context, assignmentID string) (*assessment.StaffAssignment, error) {
	defer r.s.lockRead(ctx)()
	st := r.s.active(ctx)
	if w, ok := st.writtenAssign[assignmentID]; ok {
		return &assessment.StaffAssignment{
			PublicAssignment: assessment.PublicAssignment{
				ID: w.id, LectureID: w.lectureID, CourseCode: w.courseCode,
				Kind: assessment.KindWritten, Title: w.title, DueDate: w.dueDate,
			},
		}, nil
	}
	if c, ok := st.codingAssign[assignmentID]; ok {
		return toStaffCoding(c), nil
	}
	return nil, errs.ErrNotFound
}

func (r *AssignmentRepo) ListAllWithHiddenTests(ctx context.Context) ([]assessment.StaffAssignment, error) {
	defer r.s.lockRead(ctx)()
	st := r.s.active(ctx)
	var out []assessment.StaffAssignment
	for _, w := range st.writtenAssign {
		out = append(out, assessment.StaffAssignment{
			PublicAssignment: assessment.PublicAssignment{
				ID: w.id, LectureID: w.lectureID, CourseCode: w.courseCode,
				Kind: assessment.KindWritten, Title: w.title, DueDate: w.dueDate,
			},
		})
	}
	for _, c := range st.codingAssign {
		out = append(out, *toStaffCoding(c))
	}
	slices.SortFunc(out, func(a, b assessment.StaffAssignment) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func toPublicCoding(c storedCodingAssignment) *assessment.PublicAssignment {
	var examples []assessment.TestCase
	for _, tc := range c.problem.TestCases {
		if !tc.Hidden {
			examples = append(examples, assessment.TestCase{Input: tc.Input, Expected: tc.Expected})
		}
	}
	return &assessment.PublicAssignment{
		ID: c.id, LectureID: c.lectureID, CourseCode: c.courseCode,
		Kind: assessment.KindCoding, Title: c.title, DueDate: c.dueDate,
		Difficulty: c.problem.Difficulty, Entrypoint: c.problem.Entrypoint,
		Description: c.problem.Description, InputFormat: c.problem.InputFormat,
		OutputFmt: c.problem.OutputFormat, Constraints: c.problem.Constraints,
		StarterCode: maps.Clone(c.problem.StarterCode),
		Examples:    examples,
		TotalTests:  len(c.problem.TestCases),
	}
}

func toStaffCoding(c storedCodingAssignment) *assessment.StaffAssignment {
	pub := toPublicCoding(c)
	return &assessment.StaffAssignment{
		PublicAssignment: *pub,
		AllTestCases:     slices.Clone(c.problem.TestCases),
	}
}

type SubmissionRepo struct{ s *Store }

func (s *Store) Submissions() *SubmissionRepo { return &SubmissionRepo{s: s} }

func (r *SubmissionRepo) ListForStudent(ctx context.Context, userID string) ([]assessment.Submission, error) {
	defer r.s.lockRead(ctx)()
	var out []assessment.Submission
	for _, sub := range r.s.active(ctx).submissions {
		if sub.UserID == userID {
			sub.UserID = ""
			out = append(out, sub)
		}
	}
	return out, nil
}

func (r *SubmissionRepo) ListAll(ctx context.Context) ([]assessment.Submission, error) {
	defer r.s.lockRead(ctx)()
	st := r.s.active(ctx)
	var out []assessment.Submission
	for _, sub := range st.submissions {
		if u, ok := st.users[sub.UserID]; ok {
			sub.StudentName = u.Name
		}
		out = append(out, sub)
	}
	slices.SortFunc(out, func(a, b assessment.Submission) int { return strings.Compare(b.Created, a.Created) })
	return out, nil
}

func (r *SubmissionRepo) GetByID(ctx context.Context, id string) (*assessment.Submission, error) {
	defer r.s.lockRead(ctx)()
	sub, ok := r.s.active(ctx).submissions[id]
	if !ok {
		return nil, errs.ErrNotFound
	}
	return &sub, nil
}

func (r *SubmissionRepo) GetByUserAndAssignment(ctx context.Context, userID, assignmentID string) (*assessment.Submission, error) {
	defer r.s.lockRead(ctx)()
	for _, sub := range r.s.active(ctx).submissions {
		if sub.UserID == userID && sub.AssignmentID == assignmentID {
			cp := sub
			return &cp, nil
		}
	}
	return nil, errs.ErrNotFound
}

func (r *SubmissionRepo) Upsert(ctx context.Context, sub *assessment.Submission) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	for id, existing := range st.submissions {
		if existing.UserID == sub.UserID && existing.AssignmentID == sub.AssignmentID {
			cp := *sub
			cp.ID = id
			cp.Score = existing.Score
			cp.Feedback = existing.Feedback
			st.submissions[id] = cp
			return nil
		}
	}
	st.submissions[sub.ID] = *sub
	return nil
}

func (r *SubmissionRepo) Grade(ctx context.Context, submissionID, score, feedback string) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	sub, ok := st.submissions[submissionID]
	if !ok {
		return errs.ErrNotFound
	}
	sc := score
	fb := feedback
	sub.Score = &sc
	sub.Feedback = &fb
	st.submissions[submissionID] = sub
	return nil
}

type CodingAttemptRepo struct{ s *Store }

func (s *Store) CodingAttempts() *CodingAttemptRepo { return &CodingAttemptRepo{s: s} }

func (r *CodingAttemptRepo) Save(ctx context.Context, attempt *assessment.CodingAttempt) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	cp := *attempt
	cp.Result = slices.Clone(attempt.Result)
	st.codingAttempts = append(st.codingAttempts, cp)
	return nil
}

func (r *CodingAttemptRepo) History(ctx context.Context, userID, assignmentID string, limit int) ([]assessment.CodingAttempt, error) {
	defer r.s.lockRead(ctx)()
	var out []assessment.CodingAttempt
	for _, a := range r.s.active(ctx).codingAttempts {
		if a.UserID == userID && a.AssignmentID == assignmentID {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b assessment.CodingAttempt) int { return b.CreatedAt.Compare(a.CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type ContestRepo struct{ s *Store }

func (s *Store) Contests() *ContestRepo { return &ContestRepo{s: s} }

func (r *ContestRepo) SaveSummary(ctx context.Context, userID, courseCode string, item assessment.ContestSummaryItem) {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if st.contestSums[userID] == nil {
		st.contestSums[userID] = make(map[string]assessment.ContestSummaryItem)
	}
	st.contestSums[userID][courseCode] = item
}

func (r *ContestRepo) SaveRecent(ctx context.Context, userID string, records []assessment.ContestRecord) {
	defer r.s.lockWrite(ctx)()
	r.s.active(ctx).recentContests[userID] = slices.Clone(records)
}

func (r *ContestRepo) ListSummariesForStudent(ctx context.Context, userID string) (map[string]assessment.ContestSummaryItem, error) {
	defer r.s.lockRead(ctx)()
	out := make(map[string]assessment.ContestSummaryItem)
	if m, ok := r.s.active(ctx).contestSums[userID]; ok {
		maps.Copy(out, m)
	}
	return out, nil
}

func (r *ContestRepo) ListRecentForStudent(ctx context.Context, userID string) ([]assessment.ContestRecord, error) {
	defer r.s.lockRead(ctx)()
	return slices.Clone(r.s.active(ctx).recentContests[userID]), nil
}

type InvoiceRepo struct{ s *Store }

func (s *Store) Invoices() *InvoiceRepo { return &InvoiceRepo{s: s} }

func (r *InvoiceRepo) Create(ctx context.Context, invoice *finance.FeeInvoice) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, exists := st.invoices[invoice.ID]; exists {
		return errs.ErrConflict
	}
	cp := *invoice
	if cp.Version == 0 {
		cp.Version = 1
	}
	st.invoices[cp.ID] = cp
	return nil
}

func (r *InvoiceRepo) ListForStudent(ctx context.Context, userID string) ([]finance.FeeInvoice, error) {
	defer r.s.lockRead(ctx)()
	var out []finance.FeeInvoice
	for _, inv := range r.s.active(ctx).invoices {
		if inv.UserID == userID {
			out = append(out, inv)
		}
	}
	slices.SortFunc(out, func(a, b finance.FeeInvoice) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func (r *InvoiceRepo) ListAll(ctx context.Context) ([]finance.FeeInvoice, error) {
	defer r.s.lockRead(ctx)()
	var out []finance.FeeInvoice
	for _, inv := range r.s.active(ctx).invoices {
		out = append(out, inv)
	}
	return out, nil
}

func (r *InvoiceRepo) GetOwned(ctx context.Context, feeID, userID string) (*finance.FeeInvoice, error) {
	defer r.s.lockRead(ctx)()
	inv, ok := r.s.active(ctx).invoices[feeID]
	if !ok || inv.UserID != userID {
		return nil, errs.ErrNotFound
	}
	return &inv, nil
}

func (r *InvoiceRepo) MarkPaid(ctx context.Context, feeID, paidDate string) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	inv, ok := st.invoices[feeID]
	if !ok {
		return errs.ErrNotFound
	}
	inv.Status = finance.StatusPaid
	pd := paidDate
	inv.PaidDate = &pd
	inv.ReceiptAvailable = true
	inv.Simulated = true
	inv.Version++
	st.invoices[feeID] = inv
	return nil
}

type PaymentRepo struct{ s *Store }

func (s *Store) Payments() *PaymentRepo { return &PaymentRepo{s: s} }

func (r *PaymentRepo) Record(ctx context.Context, payment *finance.PaymentRecord) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	for _, existing := range st.payments {
		if existing.FeeID == payment.FeeID || existing.IdempotencyKey == payment.IdempotencyKey {
			return errs.ErrConflict
		}
	}
	st.payments[payment.ID] = *payment
	return nil
}

type SyncRepo struct{ s *Store }

func (s *Store) Sync() *SyncRepo { return &SyncRepo{s: s} }

func (r *SyncRepo) EnqueueEvent(ctx context.Context, event syncdomain.OutboxEvent) error {
	defer r.s.lockWrite(ctx)()
	r.s.active(ctx).outbox = append(r.s.active(ctx).outbox, event)
	return nil
}

func (r *SyncRepo) AppendChangelog(ctx context.Context, scope string, targetUserID *string, entityType, entityID, op string, payload json.RawMessage) (int64, error) {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	seq := st.nextSeq
	st.nextSeq++
	if targetUserID != nil && scope == "" {
		scope = "user:" + *targetUserID
	}
	st.changelog = append(st.changelog, syncdomain.ChangelogEntry{
		Seq:        seq,
		Scope:      scope,
		EntityType: entityType,
		EntityID:   entityID,
		Op:         op,
		Payload:    slices.Clone(payload),
		UpdatedAt:  time.Now().UTC(),
	})
	return seq, nil
}

func (r *SyncRepo) ClaimUnpublished(ctx context.Context, limit int) ([]syncdomain.OutboxEvent, error) {
	defer r.s.lockRead(ctx)()
	st := r.s.active(ctx)
	n := len(st.outbox)
	if limit > 0 && limit < n {
		n = limit
	}
	return slices.Clone(st.outbox[:n]), nil
}

func (r *SyncRepo) MarkPublished(ctx context.Context, ids []string) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	rm := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		rm[id] = struct{}{}
	}
	var kept []syncdomain.OutboxEvent
	for _, e := range st.outbox {
		if _, ok := rm[e.ID]; !ok {
			kept = append(kept, e)
		}
	}
	st.outbox = kept
	return nil
}

func (r *SyncRepo) ReadSince(ctx context.Context, userID string, cursor int64, limit int) ([]syncdomain.ChangelogEntry, int64, error) {
	defer r.s.lockRead(ctx)()
	st := r.s.active(ctx)
	userScope := "user:" + userID
	var out []syncdomain.ChangelogEntry
	latest := cursor
	for _, entry := range st.changelog {
		if entry.Seq <= cursor {
			continue
		}
		if entry.Scope == "global" || entry.Scope == userScope {
			out = append(out, entry)
			latest = entry.Seq
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, latest, nil
}

func (r *SyncRepo) LookupMutation(ctx context.Context, key string) (json.RawMessage, bool, error) {
	defer r.s.lockRead(ctx)()
	val, ok := r.s.active(ctx).idem[key]
	if !ok {
		return nil, false, nil
	}
	return slices.Clone(val), true, nil
}

func (r *SyncRepo) SaveMutation(ctx context.Context, key, _ string, response json.RawMessage) error {
	defer r.s.lockWrite(ctx)()
	r.s.active(ctx).idem[key] = slices.Clone(response)
	return nil
}
