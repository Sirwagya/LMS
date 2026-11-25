package workspace_test

import (
	"context"
	"testing"

	"vedam/lms/internal/adapters/memory"
	"vedam/lms/internal/application/workspace"
	"vedam/lms/internal/domain/assessment"
	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/iam"
	"vedam/lms/internal/domain/learning"
)

func TestBootstrapService_IsolatesHiddenTestsAndAttendance(t *testing.T) {
	st := memory.NewStore()
	ctx := context.Background()

	st.Catalog().EnsureSemester(ctx, catalog.Semester{Number: 3, Label: "Semester 3", IsCurrent: true})
	_ = st.Catalog().CreateCourse(ctx, &catalog.Course{
		Code: "E0460A", Semester: 3, Name: "ADA", Credits: 4, Att: "90%", Skips: "+3 skips",
	})
	lid, _ := st.Catalog().CreateLecture(ctx, &catalog.Lecture{
		CourseCode: "E0460A", Code: "LEC 1", Date: "2026-10-19", Title: "DP", RawTitle: "DP", Attendance: "Present",
	})
	_ = st.Assignments().CreateCoding(ctx, "coding-two-sum", lid, "E0460A", "Two Sum", "2026-12-15", assessment.CodingProblemSpec{
		Entrypoint:   "twoSum",
		Description:  "Sum",
		InputFormat:  "n target",
		OutputFormat: "i j",
		Constraints:  "n<=100",
		Difficulty:   "Easy",
		StarterCode:  map[string]string{"python": "pass"},
		TestCases: []assessment.TestCase{
			{Input: "1", Expected: "2", Hidden: false, Ordinal: 1},
			{Input: "3", Expected: "4", Hidden: true, Ordinal: 2},
		},
	})

	demoStudent := &iam.User{ID: "u-demo", Email: "student@vedam.local", Name: "Demo", Role: iam.RoleStudent}
	newStudent := &iam.User{ID: "u-new", Email: "new@vedam.local", Name: "New", Role: iam.RoleStudent}
	teacher := &iam.User{ID: "u-teach", Email: "teacher@vedam.local", Name: "Teacher", Role: iam.RoleTeacher}
	_ = st.Users().Create(ctx, demoStudent)
	_ = st.Users().Create(ctx, newStudent)
	_ = st.Users().Create(ctx, teacher)
	_ = st.Attendance().Upsert(ctx, learning.AttendanceSummary{UserID: demoStudent.ID, CourseCode: "E0460A", Attended: 16, Total: 18})

	svc := workspace.NewBootstrapService(
		st.Catalog(), st.Schedule(), st.Notices(), st.Attendance(), st.Progress(),
		st.Submissions(), st.Assignments(), st.Assignments(), st.Contests(), st.Invoices(),
	)

	// New student should NOT inherit demo student attendance or Present status, and must NOT see hidden test cases
	newResp, err := svc.Build(ctx, newStudent)
	if err != nil {
		t.Fatalf("build for new student: %v", err)
	}
	if newResp.SemesterSubjects[3][0].TotalClasses != 0 {
		t.Fatalf("expected 0 total classes for new student, got %d", newResp.SemesterSubjects[3][0].TotalClasses)
	}
	lec := newResp.LecturesBySubject["E0460A"][0]
	if lec.Attendance != "Awaiting" {
		t.Fatalf("expected Awaiting attendance for non-demo student, got %s", lec.Attendance)
	}
	probMap := lec.Assignments[0].Problem.(map[string]any)
	if _, hasHidden := probMap["testCases"]; hasHidden {
		t.Fatalf("student bootstrap leaked hidden testCases key")
	}

	// Teacher MUST receive full testCases
	teachResp, err := svc.Build(ctx, teacher)
	if err != nil {
		t.Fatalf("build for teacher: %v", err)
	}
	teachProb := teachResp.LecturesBySubject["E0460A"][0].Assignments[0].Problem.(map[string]any)
	if _, hasFull := teachProb["testCases"]; !hasFull {
		t.Fatalf("teacher bootstrap missing full testCases")
	}
}
