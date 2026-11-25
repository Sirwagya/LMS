package contracttest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"vedam/lms/internal/domain/academic"
	"vedam/lms/internal/domain/assessment"
	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/envelope"
	"vedam/lms/internal/domain/errs"
	"vedam/lms/internal/domain/finance"
	"vedam/lms/internal/domain/iam"
	"vedam/lms/internal/domain/learning"
	syncdomain "vedam/lms/internal/domain/sync"
	"vedam/lms/internal/domain/uow"
)

type Harness struct {
	Tx             uow.TxManager
	UsersRead      iam.UserReader
	UsersWrite     iam.UserWriter
	CatRead        catalog.CatalogReader
	CatWrite       catalog.CatalogWriter
	Schedule       catalog.ScheduleRepository
	Attendance     learning.AttendanceWriter
	AttRead        learning.AttendanceReader
	Progress       learning.ProgressRepository
	AssignRead     assessment.StudentAssignmentReader
	AssignAuthor   assessment.StaffAssignmentAuthor
	Submissions    assessment.SubmissionRepository
	InvoiceRead    finance.InvoiceReader
	InvoiceWrite   finance.InvoiceWriter
	PaymentRec     finance.PaymentRecorder
	Outbox         syncdomain.OutboxWriter
	SyncReader     syncdomain.SyncLogReader
	Idempotency    syncdomain.IdempotencyStore
	AcadWriter     academic.AcademicHierarchyWriter
	EnrolRead      academic.EnrolmentReader
	SessionRead    academic.ClassSessionReader
	Biometrics     academic.BiometricRepository
	EnsureSem      func(ctx context.Context, num int)
}

func RunAll(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Run("IAM_OptimisticVersionAndDuplicateEmail", func(t *testing.T) {
		h := newHarness(t)
		ctx := context.Background()
		u := &iam.User{
			ID:           "user-iam-1",
			Email:        "alice@vedam.local",
			Name:         "Alice",
			Role:         iam.RoleStudent,
			PasswordHash: "hash1",
			Profile:      json.RawMessage(`{"headline":"AI"}`),
		}
		if err := h.UsersWrite.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		if err := h.UsersWrite.Create(ctx, &iam.User{
			ID: "user-iam-2", Email: "ALICE@VEDAM.LOCAL", Name: "Alice2", Role: iam.RoleStudent, PasswordHash: "hash2",
		}); !errors.Is(err, errs.ErrConflict) {
			t.Fatalf("expected ErrConflict on duplicate case-insensitive email, got %v", err)
		}

		v1 := int64(1)
		newVer, err := h.UsersWrite.UpdateProfile(ctx, "user-iam-1", "Alice Updated", json.RawMessage(`{"headline":"Systems"}`), &v1)
		if err != nil || newVer != 2 {
			t.Fatalf("expected version=2, got %d err=%v", newVer, err)
		}
		if _, err := h.UsersWrite.UpdateProfile(ctx, "user-iam-1", "Stale", json.RawMessage(`{}`), &v1); !errors.Is(err, errs.ErrConflict) {
			t.Fatalf("expected ErrConflict on stale version, got %v", err)
		}
	})

	t.Run("Schedule_OverlapDetection", func(t *testing.T) {
		h := newHarness(t)
		ctx := context.Background()
		h.EnsureSem(ctx, 3)
		if err := h.CatWrite.CreateCourse(ctx, &catalog.Course{
			Code: "CS301", Semester: 3, Name: "Systems", Credits: 4, Att: "90%", Skips: "+2",
		}); err != nil {
			t.Fatalf("create course: %v", err)
		}

		slot1 := catalog.ScheduleClass{
			ID: "slot-1", Week: "2026-10-19", SlotDate: "2026-10-20",
			From: "10:00", To: "12:00", Code: "CS301", Room: "Room B1", Topic: "Locks", Type: "Lecture",
		}
		if err := h.Schedule.AddSlot(ctx, slot1); err != nil {
			t.Fatalf("add slot1: %v", err)
		}
		overlap := catalog.ScheduleClass{
			ID: "slot-2", Week: "2026-10-19", SlotDate: "2026-10-20",
			From: "11:00", To: "13:00", Code: "CS301", Room: "Room B2", Topic: "Deadlocks", Type: "Lecture",
		}
		if err := h.Schedule.AddSlot(ctx, overlap); !errors.Is(err, errs.ErrScheduleOverlap) {
			t.Fatalf("expected ErrScheduleOverlap, got %v", err)
		}
		adjacent := catalog.ScheduleClass{
			ID: "slot-3", Week: "2026-10-19", SlotDate: "2026-10-20",
			From: "12:00", To: "13:00", Code: "CS301", Room: "Room B1", Topic: "Channels", Type: "Lecture",
		}
		if err := h.Schedule.AddSlot(ctx, adjacent); err != nil {
			t.Fatalf("adjacent boundary [12:00,13:00) should not overlap [10:00,12:00): %v", err)
		}
	})

	t.Run("Assessment_StudentPortStripsHiddenTests", func(t *testing.T) {
		h := newHarness(t)
		ctx := context.Background()
		h.EnsureSem(ctx, 3)
		_ = h.CatWrite.CreateCourse(ctx, &catalog.Course{
			Code: "ALG301", Semester: 3, Name: "Algorithms", Credits: 4, Att: "90%", Skips: "+2",
		})
		lid, err := h.CatWrite.CreateLecture(ctx, &catalog.Lecture{
			CourseCode: "ALG301", Code: "LEC 1", Date: "2026-10-19", Title: "DP", RawTitle: "DP", Attendance: "Present",
		})
		if err != nil {
			t.Fatalf("create lecture: %v", err)
		}
		err = h.AssignAuthor.CreateCoding(ctx, "c-prob-1", lid, "ALG301", "Two Sum", "2026-12-15", assessment.CodingProblemSpec{
			Entrypoint:   "twoSum",
			Description:  "Find indices",
			InputFormat:  "n target",
			OutputFormat: "i j",
			Constraints:  "n<=1e4",
			Difficulty:   "Easy",
			StarterCode:  map[string]string{"python": "pass", "java": "class Solution {}"},
			TestCases: []assessment.TestCase{
				{Input: "4 9\n2 7 11 15", Expected: "0 1", Hidden: false, Ordinal: 1},
				{Input: "2 6\n3 3", Expected: "0 1", Hidden: true, Ordinal: 2},
			},
		})
		if err != nil {
			t.Fatalf("create coding assignment: %v", err)
		}

		pub, err := h.AssignRead.GetPublicByID(ctx, "c-prob-1")
		if err != nil {
			t.Fatalf("get public assignment: %v", err)
		}
		if pub.TotalTests != 2 || len(pub.Examples) != 1 {
			t.Fatalf("expected 1 public example and totalTests=2, got examples=%d total=%d", len(pub.Examples), pub.TotalTests)
		}
		staffView, err := h.AssignAuthor.GetWithHiddenTests(ctx, "c-prob-1")
		if err != nil {
			t.Fatalf("get staff assignment: %v", err)
		}
		if len(staffView.AllTestCases) != 2 {
			t.Fatalf("expected staff view to contain 2 full cases, got %d", len(staffView.AllTestCases))
		}
	})

	t.Run("Finance_AtomicRollbackAndForeignOwnership", func(t *testing.T) {
		h := newHarness(t)
		ctx := context.Background()
		if err := h.UsersWrite.Create(ctx, &iam.User{
			ID: "stu-1", Email: "stu1@vedam.local", Name: "Student One", Role: iam.RoleStudent, PasswordHash: "x",
		}); err != nil {
			t.Fatalf("seed user: %v", err)
		}
		if err := h.InvoiceWrite.Create(ctx, &finance.FeeInvoice{
			ID: "fee-1", UserID: "stu-1", InvoiceNo: "INV-1", Description: "Tuition", Semester: "S3",
			Amount: 50000, DueDate: "2026-11-01", Status: finance.StatusPending,
		}); err != nil {
			t.Fatalf("seed invoice: %v", err)
		}

		if _, err := h.InvoiceRead.GetOwned(ctx, "fee-1", "other-user"); !errors.Is(err, errs.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for non-owner, got %v", err)
		}

		boom := errors.New("simulated mid-tx crash")
		err := h.Tx.WithinTx(ctx, func(txCtx context.Context) error {
			if err := h.PaymentRec.Record(txCtx, &finance.PaymentRecord{
				ID: "pay-1", UserID: "stu-1", FeeID: "fee-1", IdempotencyKey: "idem-1", CreatedAt: time.Now().UTC(),
			}); err != nil {
				return err
			}
			if err := h.InvoiceWrite.MarkPaid(txCtx, "fee-1", "2026-11-25"); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("expected injected boom error, got %v", err)
		}

		inv, err := h.InvoiceRead.GetOwned(ctx, "fee-1", "stu-1")
		if err != nil {
			t.Fatalf("re-fetch invoice: %v", err)
		}
		if inv.Status != finance.StatusPending {
			t.Fatalf("expected rollback to leave invoice Pending, got %s", inv.Status)
		}
	})

	t.Run("Sync_ChangelogOrderingAndIdempotency", func(t *testing.T) {
		h := newHarness(t)
		ctx := context.Background()
		_ = h.UsersWrite.Create(ctx, &iam.User{
			ID: "stu-sync", Email: "sync@vedam.local", Name: "Sync", Role: iam.RoleStudent, PasswordHash: "x",
		})
		_ = h.UsersWrite.Create(ctx, &iam.User{
			ID: "stu-other", Email: "other@vedam.local", Name: "Other", Role: iam.RoleStudent, PasswordHash: "x",
		})
		u1 := "stu-sync"
		u2 := "stu-other"
		seq1, err := h.Outbox.AppendChangelog(ctx, "global", nil, "notice", "n-1", "upsert", json.RawMessage(`{"title":"Global"}`))
		if err != nil {
			t.Fatalf("append global: %v", err)
		}
		_, err = h.Outbox.AppendChangelog(ctx, "user:stu-other", &u2, "fee_invoice", "f-2", "upsert", json.RawMessage(`{}`))
		if err != nil {
			t.Fatalf("append other user: %v", err)
		}
		seq3, err := h.Outbox.AppendChangelog(ctx, "user:stu-sync", &u1, "profile", "stu-sync", "upsert", json.RawMessage(`{"name":"Sync2"}`))
		if err != nil {
			t.Fatalf("append user1: %v", err)
		}

		entries, latest, err := h.SyncReader.ReadSince(ctx, "stu-sync", 0, 50)
		if err != nil {
			t.Fatalf("read since: %v", err)
		}
		if len(entries) != 2 || entries[0].Seq != seq1 || entries[1].Seq != seq3 || latest != seq3 {
			t.Fatalf("expected global + stu-sync entries (%d, %d), got %+v latest=%d", seq1, seq3, entries, latest)
		}

		if err := h.Idempotency.SaveMutation(ctx, "mut-1", "stu-sync", json.RawMessage(`{"ok":true}`)); err != nil {
			t.Fatalf("save mutation: %v", err)
		}
		resp, found, err := h.Idempotency.LookupMutation(ctx, "mut-1")
		if err != nil || !found {
			t.Fatalf("expected cached idempotency hit, got found=%v resp=%s err=%v", found, resp, err)
		}
		var parsed map[string]bool
		if err := json.Unmarshal(resp, &parsed); err != nil || !parsed["ok"] {
			t.Fatalf("expected {ok:true}, got %s (err=%v)", resp, err)
		}
	})

	t.Run("AcademicHierarchy_OfferingsSemestersSessionsAndBiometrics", func(t *testing.T) {
		h := newHarness(t)
		ctx := context.Background()

		if err := h.UsersWrite.Create(ctx, &iam.User{
			ID: "u-vedam-1", Email: "student@vedam.local", Name: "Sirwagya Shekhar", Role: iam.RoleStudent, PasswordHash: "x",
		}); err != nil {
			t.Fatalf("create user: %v", err)
		}
		_ = h.AcadWriter.UpsertCampus(ctx, academic.Campus{ID: 1, Name: "Vedam Campus"})
		_ = h.AcadWriter.UpsertProgram(ctx, academic.Program{ID: 1, CampusID: 1, Title: "BTech CSE AI 2025", Label: "BTECH_CSE_AI_25"})
		_ = h.AcadWriter.UpsertCampusPersonRole(ctx, academic.CampusPersonRole{
			CPRID: 125, UserID: "u-vedam-1", CampusID: 1, PersonName: "Sirwagya Shekhar", Email: "student@vedam.local",
		})
		_ = h.AcadWriter.UpsertProgramEnrolment(ctx, 122, 1, 125, 1753313163)

		_ = h.AcadWriter.UpsertSemester(ctx, academic.SemesterRecord{
			ID: 6, CampusID: 1, ProgramID: 1, Title: "Bootcamp Summer 2026", Label: "BTC_SUM_26",
			StartDate: 1779062400, EndDate: 1782777600, ProgramTitle: "BTech CSE AI 2025", ProgramLabel: "BTECH_CSE_AI_25", RequiredAttendance: 75,
		})
		_ = h.AcadWriter.UpsertSemester(ctx, academic.SemesterRecord{
			ID: 3, CampusID: 1, ProgramID: 1, Title: "Semester 3 2026", Label: "SEM3_26",
			StartDate: 1785715200, EndDate: 1796860800, ProgramTitle: "BTech CSE AI 2025", ProgramLabel: "BTECH_CSE_AI_25", RequiredAttendance: 75,
		})
		_ = h.CatWrite.CreateCourse(ctx, &catalog.Course{
			Code: "E0460A", Semester: 3, Name: "Analysis and Design of Algorithms", Credits: 4, Att: "90%", Skips: "+3",
		})
		if err := h.AcadWriter.UpsertCourseOffering(ctx, academic.CourseOfferingSpec{
			ID: 22, CourseID: 15, CourseCode: "E0460A", SemesterID: 3, CampusID: 1, ProgramID: 1,
			Label: "E0460A", Title: "Analysis and Design of Algorithms B1", Batch: "B1", Credits: 4,
			EnrolmentCount: 140, ClassSessionCount: 44, CourseRequiredAttendance: 75,
		}); err != nil {
			t.Fatalf("upsert offering: %v", err)
		}
		if err := h.AcadWriter.UpsertCourseOfferingEnrolment(ctx, 2440, 22, 125, 1785024000, 29, 4, 0, 0, 11, 0, 44); err != nil {
			t.Fatalf("upsert coe: %v", err)
		}
		room := "Room B1"
		_ = h.AcadWriter.UpsertClassSession(ctx, academic.ClassSessionItem{
			ID: 546, CourseOfferingID: 22, StartTime: 1785123000, EndTime: 1785130200,
			Title: "Arrays 1 (prefix and suffix)", Room: &room, ProgramID: 1, SemesterID: 3,
			CSCreatedAt: 1785076288, CSUpdatedAt: 1787464556, Label: "LEC 0", Topic: "Arrays 1 (prefix and suffix)",
		})
		_ = h.Biometrics.UpsertFaceBiometrics(ctx, academic.FaceBiometricRecord{
			PersonID: 125, Email: "student@vedam.local", HasFaceData: true, FaceJSON: `{"embedding":[-0.1,0.2]}`, Embedding: []float64{-0.1, 0.2},
		})

		pe, err := h.EnrolRead.ListProgramEnrolments(ctx, 125)
		if err != nil || len(pe) != 1 || pe[0].ProgramLabel != "BTECH_CSE_AI_25" {
			t.Fatalf("unexpected program enrolments: %+v err=%v", pe, err)
		}
		sems, err := h.SessionRead.ListSemestersByProgram(ctx, 1)
		if err != nil || len(sems) != 2 || sems[0].ID != 6 {
			t.Fatalf("expected Bootcamp sem 6 ordered before sem 3, got %+v err=%v", sems, err)
		}
		coes, err := h.EnrolRead.ListCourseOfferingEnrolments(ctx, 125, nil)
		if err != nil || len(coes) != 1 || coes[0].PresentCount != 29 || coes[0].ExcusedCount != 11 {
			t.Fatalf("unexpected course offering enrolments: %+v err=%v", coes, err)
		}
		bio, err := h.Biometrics.GetFaceBiometrics(ctx, 125)
		if err != nil || !bio.HasFaceData || len(bio.Embedding) != 2 {
			t.Fatalf("unexpected biometrics: %+v err=%v", bio, err)
		}
		semFilter := 3
		sessions, total, err := h.SessionRead.ListClassSessions(ctx, academic.ClassSessionFilter{SemesterID: &semFilter, Page: 0, Size: 10})
		if err != nil || total != 1 || len(sessions) != 1 {
			t.Fatalf("unexpected class sessions: total=%d len=%d err=%v", total, len(sessions), err)
		}
		page := envelope.NewPage(sessions, 0, 10, total)
		if page.TotalPages != 1 || !page.First || !page.Last {
			t.Fatalf("unexpected page metadata: %+v", page)
		}
	})
}
