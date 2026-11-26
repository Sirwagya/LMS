//go:build integration

package runtime

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"vedam/lms/internal/adapters/grader"
	"vedam/lms/internal/adapters/postgres"
	"vedam/lms/internal/domain/contest"
)

func TestContestGradingWorkflow(t *testing.T) {
	const hidden = "PRIVATE_JUDGE_CASE"
	judgeToken := strings.Repeat("test-judge-secret", 3)
	called := make(chan string, 10)
	judge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var grade grader.Request
		if req.Method != "POST" || req.Header.Get("Authorization") != "Bearer "+judgeToken ||
			json.NewDecoder(req.Body).Decode(&grade) != nil || grade.Entrypoint != "stdin" ||
			len(grade.Cases) != 2 || grade.Cases[0].Hidden || !grade.Cases[1].Hidden ||
			grade.Cases[1].Expected == nil || *grade.Cases[1].Expected != hidden {
			http.Error(w, "bad request", http.StatusBadRequest)
			t.Error("judge received a malformed request")
			return
		}
		called <- req.Header.Get("Idempotency-Key")
		if grade.Source == "# judge unavailable" {
			http.Error(w, hidden, http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"verdict": "Accepted", "passed": 2, "total": 2, "extra": hidden})
	}))
	t.Cleanup(judge.Close)
	api := startConfiguredAPI(t, func(cfg *Config) {
		cfg.ContestAuthoring, cfg.ContestAttempts, cfg.ContestGrading = true, true, true
		cfg.ContestJudgeURL, cfg.ContestJudgeToken = judge.URL, judgeToken
	})
	teacher := loginAs(t, api, "teacher")
	student := loginAs(t, api, "student")
	admin := loginAs(t, api, "admin")
	other := contestTestUser(t, api, admin.Token, "student", "outside-gradework")
	device := uuid.NewString()
	chosen := 1
	example, hiddenText := "sample", hidden
	now := time.Now().UTC()
	draft := contest.Draft{
		Title: "Grading contest", OpensAt: now.Add(time.Hour), ClosesAt: now.Add(3 * time.Hour), DurationMinutes: 20,
		StudentIDs: []string{student.User.ID},
		Questions: []contest.Question{
			{Kind: "mcq", Prompt: "Choose", Choices: []string{"a", "b"}, CorrectChoice: &chosen, Points: 9},
			{Kind: "coding", Prompt: "Read stdin", Points: 11, TestCases: []contest.TestCase{
				{Input: &example, Expected: &example}, {Input: &hiddenText, Expected: &hiddenText, Hidden: true},
			}},
		},
	}
	repo := postgres.NewContestAttemptRepo(api.Tx)
	db := func(query string, args ...any) {
		t.Helper()
		if _, err := api.DB.Exec(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	create := func() string {
		t.Helper()
		const path = "/api/staff/contests"
		record := decode[contest.Record](t, requireStatus(t, api, 201, "POST", path, teacher.Token, "", map[string]any{"draft": draft}))
		requireStatus(t, api, 200, "POST", path+"/"+record.ID+"/publish", teacher.Token, "", map[string]int{"revision": 1})
		db("UPDATE assessments SET opens_at=$2,closes_at=$3 WHERE id=$1", record.ID, now.Add(-time.Hour), now.Add(time.Hour))
		return record.ID
	}
	call := func(id, token string, status int, body map[string]any) contest.AttemptView {
		t.Helper()
		body["deviceId"] = device
		res, err := api.callWithHeaders(t.Context(), "POST", "/api/v1/contests/"+id+"/attempt", token, "", body,
			map[string]string{"X-LMS-Contest-Client": "macos"})
		if err != nil || res.status != status {
			t.Fatalf("%v: HTTP %d (%v) %s, want %d", body["action"], res.status, err, res.raw, status)
		}
		for _, secret := range []string{hidden, "questionScores", "possiblePoints", "correctChoice", "print(input())"} {
			if bytes.Contains(res.raw, []byte(secret)) {
				t.Fatalf("official submission or result unexpectedly disclosed %q", secret)
			}
		}
		if status != 200 {
			return contest.AttemptView{}
		}
		return decode[contest.AttemptView](t, res)
	}
	submitCode := func(id, key, source string) contest.AttemptView {
		t.Helper()
		return call(id, student.Token, 200, map[string]any{"action": "code", "requestId": key, "questionIndex": 1,
			"language": "python", "source": source, "lease": 1})
	}
	gradeOnce := func(expectID string) {
		t.Helper()
		found, err := api.processContestSubmission(t.Context(), repo)
		if !found || err != nil {
			t.Fatalf("judge worker did not process %s (%v)", expectID, err)
		}
		select {
		case id := <-called:
			if id != expectID {
				t.Fatalf("idempotency key = %s, want %s", id, expectID)
			}
		default:
			t.Fatal("judge call not observed")
		}
	}
	compute := func(expected bool) {
		t.Helper()
		if found, err := repo.ComputeScore(t.Context()); found != expected || err != nil {
			t.Fatalf("computed=%v (%v), want %v", found, err, expected)
		}
	}

	id := create()
	start := call(id, student.Token, 200, map[string]any{"action": "start"})
	if !start.OfficialCoding || start.State != "active" {
		t.Fatalf("grading unavailable: %#v", start)
	}
	call(id, other.Token, 404, map[string]any{"action": "grading"})
	call(id, student.Token, 400, map[string]any{"action": "code", "requestId": uuid.NewString(), "questionIndex": 0,
		"language": "python", "source": "example", "lease": 1})
	var selected int
	for i, choice := range start.Questions[0].Choices {
		if choice == "b" {
			selected = i
		}
	}
	answers := map[int]contest.Answer{0: {Choice: &selected}}
	saved := call(id, student.Token, 200, map[string]any{"action": "save", "lease": 1, "revision": 0, "answers": answers})
	key := uuid.NewString()
	first := submitCode(id, key, "print(input())")
	repeated := submitCode(id, key, "print(input())")
	if first.CodingSubmissionID == "" || first.CodingSubmissionID != repeated.CodingSubmissionID || len(repeated.CodingSubmissions) != 1 {
		t.Fatalf("duplicate admission: %#v", repeated)
	}
	call(id, student.Token, 409, map[string]any{"action": "code", "requestId": key, "questionIndex": 1,
		"language": "python", "source": "some other code", "lease": 1})
	second := submitCode(id, uuid.NewString(), "print(input().strip())")
	receipt := call(id, student.Token, 200, map[string]any{"action": "submit", "lease": 1, "revision": saved.Revision, "answers": answers})
	if receipt.GradingStatus != "grading" {
		t.Fatalf("expected pending grading: %#v", receipt)
	}
	late := submitCode(id, uuid.NewString(), "print(0)")
	if late.CodingSubmissionID != "" || len(late.CodingSubmissions) != 2 {
		t.Fatal("the API admitted code after finalization")
	}
	compute(false)

	// Simulate a worker process dying after claiming a job. Its old response
	// must be fenced out when another worker claims the same submission.
	old, found, err := repo.ClaimCode(t.Context())
	if !found || err != nil || old.ID != first.CodingSubmissionID {
		t.Fatalf("claim: %#v, %v", old, err)
	}
	db("UPDATE assessment_coding_submissions SET claim_after=statement_timestamp()-interval '1 second' WHERE id=$1", old.ID)
	gradeOnce(first.CodingSubmissionID)
	if err := repo.FinishCode(t.Context(), old, "Wrong Answer"); err != nil {
		t.Fatal(err)
	}
	compute(false)
	gradeOnce(second.CodingSubmissionID)
	compute(true)
	compute(false)
	finished := call(id, student.Token, 200, map[string]any{"action": "grading"})
	if finished.GradingStatus != "awaiting_release" || len(finished.CodingSubmissions) != 2 ||
		finished.CodingSubmissions[0].Status != "accepted" || finished.CodingSubmissions[1].Status != "accepted" {
		t.Fatalf("result status: %#v", finished)
	}
	var points, possible int
	var elapsed int64
	var details []byte
	if err := api.DB.QueryRow(t.Context(), `
		SELECT points,possible_points,elapsed_ms,question_scores
		FROM assessment_attempt_results WHERE attempt_id=$1 AND version=1
	`, start.AttemptID).Scan(&points, &possible, &elapsed, &details); err != nil {
		t.Fatal(err)
	}
	var breakdown []contest.QuestionScore
	if err := json.Unmarshal(details, &breakdown); err != nil {
		t.Fatal(err)
	}
	expectedElapsed := first.CodingSubmissions[0].SubmittedAt.Sub(*start.StartedAt).Milliseconds()
	if points != 20 || possible != 20 || elapsed != expectedElapsed || len(breakdown) != 2 ||
		breakdown[0].Points != 9 || breakdown[1].SubmissionID != first.CodingSubmissionID {
		t.Fatalf("score = %d of %d, elapsed %d (want %d), breakdown %+v", points, possible, elapsed, expectedElapsed, breakdown)
	}

	failing := create()
	failedStart := call(failing, student.Token, 200, map[string]any{"action": "start"})
	failed := submitCode(failing, uuid.NewString(), "# judge unavailable")
	db("UPDATE assessment_attempts SET deadline_at=clock_timestamp() WHERE id=$1", failedStart.AttemptID)
	expired := submitCode(failing, uuid.NewString(), "print(0)")
	if expired.FinishReason != "expired" || expired.CodingSubmissionID != "" || len(expired.CodingSubmissions) != 1 {
		t.Fatalf("deadline did not freeze admitted jobs: %#v", expired)
	}
	for round := range postgres.ContestJudgeRetries {
		gradeOnce(failed.CodingSubmissionID)
		if round < postgres.ContestJudgeRetries-1 {
			compute(false)
			db("UPDATE assessment_coding_submissions SET claim_after=statement_timestamp() WHERE id=$1", failed.CodingSubmissionID)
		}
	}
	compute(true)
	failure := call(failing, student.Token, 200, map[string]any{"action": "grading"})
	if failure.GradingStatus != "needs_review" || failure.CodingSubmissions[0].Status != "failed" {
		t.Fatalf("judge failure silently scored: %#v", failure)
	}
	var withheld *int
	if err := api.DB.QueryRow(t.Context(), "SELECT points FROM assessment_attempt_results WHERE attempt_id=$1 AND version=1", failedStart.AttemptID).Scan(&withheld); err != nil || withheld != nil {
		t.Fatalf("failed points = %v (%v)", withheld, err)
	}
}
