//go:build integration

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"vedam/lms/internal/adapters/grader"
	"vedam/lms/internal/adapters/postgres"
	"vedam/lms/internal/domain/contest"
	"vedam/lms/migrations"
)

func TestContestAuthoringAndPrivateRoster(t *testing.T) {
	api := startAPIWithContests(t, true)
	teacher := loginAs(t, api, "teacher")
	student := loginAs(t, api, "student")
	admin := loginAs(t, api, "admin")
	otherTeacher := contestTestUser(t, api, admin.Token, "teacher", "reviewer")
	outsider := contestTestUser(t, api, admin.Token, "student", "outsider")
	const staff = "/api/staff/contests"
	const list = "/api/v1/contests"
	now := time.Now().UTC().Truncate(time.Second)
	choice := 0
	input, example, hidden := "11", "22", "private-test-output"
	draft := contest.Draft{
		Title: "Engineering contest", Instructions: "Closed book", OpensAt: now.Add(time.Hour),
		ClosesAt: now.Add(3 * time.Hour), DurationMinutes: 60, StudentIDs: []string{student.User.ID},
		Questions: []contest.Question{
			{Kind: "mcq", Prompt: "Choose the correct answer", Points: 5, Choices: []string{"One", "Two"}, CorrectChoice: &choice, Explanation: "private answer explanation"},
			{Kind: "coding", Prompt: "Double the number", Points: 10, TestCases: []contest.TestCase{
				{Input: &input, Expected: &example}, {Input: &input, Expected: &hidden, Hidden: true},
			}},
		},
	}
	payload := map[string]any{"draft": draft}
	requireStatus(t, api, 403, "POST", staff, student.Token, "", payload)
	created := decode[contest.Record](t, requireStatus(t, api, 201, "POST", staff, teacher.Token, "", payload))
	path := staff + "/" + created.ID
	if created.Revision != 1 || created.Status != "draft" {
		t.Fatalf("unexpected initial state: %#v", created)
	}
	requireStatus(t, api, 404, "GET", path, otherTeacher.Token, "", nil)
	requireStatus(t, api, 200, "GET", path, admin.Token, "", nil)
	requireStatus(t, api, 403, "GET", path, student.Token, "", nil)
	if got := contestItems(t, api, list, student.Token); len(got) != 0 {
		t.Fatal("student list exposed the draft")
	}

	bad := draft
	bad.StudentIDs = []string{teacher.User.ID}
	requireStatus(t, api, 400, "PUT", path, teacher.Token, "", map[string]any{"revision": 1, "draft": bad})
	unchanged := decode[contest.Record](t, requireStatus(t, api, 200, "GET", path, teacher.Token, "", nil))
	if unchanged.Revision != 1 || len(unchanged.StudentIDs) != 1 || unchanged.StudentIDs[0] != student.User.ID {
		t.Fatal("failed roster update was not rolled back")
	}

	draft.Title = "Engineering contest revised"
	edit := map[string]any{"revision": 1, "draft": draft}
	updated := decode[contest.Record](t, requireStatus(t, api, 200, "PUT", path, teacher.Token, "", edit))
	if updated.Revision != 2 {
		t.Fatalf("revision = %d", updated.Revision)
	}
	requireStatus(t, api, 409, "PUT", path, teacher.Token, "", edit)
	requireStatus(t, api, 409, "POST", path+"/publish", teacher.Token, "", map[string]int{"revision": 1})
	requireStatus(t, api, 200, "POST", path+"/publish", teacher.Token, "", map[string]int{"revision": 2})
	requireStatus(t, api, 409, "PUT", path, teacher.Token, "", map[string]any{"revision": 3, "draft": draft})
	if got := contestItems(t, api, list, outsider.Token); len(got) != 0 {
		t.Fatal("student outside roster saw the contest")
	}
	studentReply := requireStatus(t, api, 200, "GET", list, student.Token, "", nil)
	if bytes.Contains(studentReply.raw, []byte("private")) || bytes.Contains(studentReply.raw, []byte("questions")) ||
		bytes.Contains(studentReply.raw, []byte("studentIds")) || !bytes.Contains(studentReply.raw, []byte(draft.Title)) {
		t.Fatalf("unexpected student response: %s", studentReply.raw)
	}

	requireStatus(t, api, 404, "POST", path+"/cancel", otherTeacher.Token, "", map[string]any{"revision": 3, "reason": "bad"})
	requireStatus(t, api, 200, "POST", path+"/cancel", teacher.Token, "", map[string]any{"revision": 3, "reason": "Course rescheduled"})
	got := contestItems(t, api, list, student.Token)
	if len(got) != 1 || got[0].Status != "cancelled" {
		t.Fatalf("cancelled contest missing from student schedule: %#v", got)
	}
	cancelled := decode[contest.Record](t, requireStatus(t, api, 200, "GET", path, teacher.Token, "", nil))
	if cancelled.CancellationReason != "Course rescheduled" {
		t.Fatal("cancellation reason was not retained")
	}
}

func contestItems(t *testing.T, api liveAPI, path, token string) []contest.Summary {
	t.Helper()
	return decode[struct{ Items []contest.Summary }](t, requireStatus(t, api, 200, "GET", path, token, "", nil)).Items
}

func contestTestUser(t *testing.T, api liveAPI, adminToken, role, name string) identity {
	t.Helper()
	email := fmt.Sprintf("%s@vedam.local", name)
	const password = "ContestTest12345!"
	requireStatus(t, api, 201, "POST", "/api/staff/users", adminToken, "", map[string]string{"name": name, "email": email, "role": role, "password": password})
	res := requireStatus(t, api, 200, "POST", "/api/login", "", "", map[string]string{"email": email, "password": password})
	user := decode[identity](t, res)
	response := http.Response{Header: res.header}
	for _, cookie := range response.Cookies() {
		if cookie.Name == "lms_session" {
			user.Token = cookie.Value
		}
	}
	if user.Token == "" {
		t.Fatal("test login did not issue a session")
	}
	return user
}

func contestDatabaseURL() string {
	if source := os.Getenv("TEST_DATABASE_URL"); source != "" {
		return source
	}
	return "postgres://lms:lms_local_dev@127.0.0.1:5432/lms_test?sslmode=disable"
}

func startContestWorker(t *testing.T, api liveAPI) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		api.Work(ctx)
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
	t.Cleanup(stop)
	return stop
}

func assertNoSecret(t *testing.T, raw []byte, secret, contextLabel string) {
	t.Helper()
	if bytes.Contains(raw, []byte(secret)) {
		t.Fatalf("%s leaked hidden secret %q", contextLabel, secret)
	}
}

func enableAllContestFlags(judgeURL, judgeToken string) func(*Config) {
	return func(cfg *Config) {
		cfg.ContestAuthoring = true
		cfg.ContestAttempts = true
		cfg.ContestGrading = true
		cfg.ContestIntegrity = true
		cfg.ContestResults = true
		cfg.ContestJudgeURL = judgeURL
		cfg.ContestJudgeToken = judgeToken
	}
}

func TestContestMigrationRollbackOnDisposableDB(t *testing.T) {
	ctx := t.Context()
	source := contestDatabaseURL()

	control, err := pgxpool.New(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(control.Close)

	schema := "lms_mig_rollback_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := control.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := control.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	})

	dbURL, err := url.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	query := dbURL.Query()
	query.Set("search_path", schema+",public")
	dbURL.RawQuery = query.Encode()

	pool, err := pgxpool.New(ctx, dbURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	contestTables := []string{
		"assessments",
		"assessment_roster",
		"assessment_attempts",
		"assessment_attempt_takeovers",
		"assessment_coding_submissions",
		"assessment_attempt_results",
		"assessment_reviewers",
		"assessment_event_batches",
		"assessment_attempt_events",
		"assessment_reviews",
		"assessment_appeals",
		"assessment_similarity_pairs",
		"assessment_evidence_access_log",
		"assessment_result_releases",
		"assessment_leaderboard_entries",
		"assessment_student_reports",
	}

	assertTables := func(wantExist bool) {
		t.Helper()
		for _, table := range contestTables {
			var reg *string
			if err := pool.QueryRow(ctx, `SELECT to_regclass($1)::text`, schema+"."+table).Scan(&reg); err != nil {
				t.Fatalf("check table %s: %v", table, err)
			}
			if wantExist && reg == nil {
				t.Fatalf("expected table %s.%s to exist", schema, table)
			}
			if !wantExist && reg != nil {
				t.Fatalf("expected table %s.%s to be dropped, got %q", schema, table, *reg)
			}
		}
	}

	assertMigrationCount := func(want int) {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil {
			t.Fatalf("count schema_migrations: %v", err)
		}
		if count != want {
			t.Fatalf("schema_migrations count = %d, want %d", count, want)
		}
	}

	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("initial Up failed: %v", err)
	}
	assertTables(true)
	assertMigrationCount(8)

	if err := migrations.Down(ctx, pool); err != nil {
		t.Fatalf("Down failed: %v", err)
	}
	assertMigrationCount(0)
	assertTables(false)

	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("re-apply Up failed: %v", err)
	}
	assertMigrationCount(8)
	assertTables(true)
}

func TestContestWorkerRestartAndClaimRecovery(t *testing.T) {
	const hiddenSecret = "HIDDEN_SECRET"
	judgeToken := strings.Repeat("restart-judge-secret", 2)

	var calls atomic.Int32
	firstStarted := make(chan struct{})
	var firstStartedOnce sync.Once
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFirst) }) })

	var expectedSubID atomic.Value
	expectedSubID.Store("")

	judge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var grade grader.Request
		if req.Method != http.MethodPost ||
			req.Header.Get("Authorization") != "Bearer "+judgeToken ||
			json.NewDecoder(req.Body).Decode(&grade) != nil ||
			grade.Entrypoint != "stdin" ||
			len(grade.Cases) != 2 ||
			grade.Cases[0].Hidden ||
			!grade.Cases[1].Hidden ||
			grade.Cases[1].Expected == nil ||
			*grade.Cases[1].Expected != hiddenSecret {
			http.Error(w, "invalid judge request", http.StatusBadRequest)
			return
		}
		if wantID, _ := expectedSubID.Load().(string); wantID != "" && req.Header.Get("Idempotency-Key") != wantID {
			http.Error(w, "unexpected idempotency key", http.StatusBadRequest)
			return
		}

		if calls.Add(1) == 1 {
			firstStartedOnce.Do(func() { close(firstStarted) })
			select {
			case <-req.Context().Done():
				return
			case <-releaseFirst:
				_ = json.NewEncoder(w).Encode(map[string]any{"verdict": "Wrong Answer", "passed": 0, "total": 2})
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"verdict": "Accepted", "passed": 2, "total": 2, "debug": hiddenSecret})
	}))
	t.Cleanup(judge.Close)

	api := startConfiguredAPI(t, enableAllContestFlags(judge.URL, judgeToken))
	teacher := loginAs(t, api, "teacher")
	student := loginAs(t, api, "student")
	device := uuid.NewString()
	nativeHeaders := map[string]string{"X-LMS-Contest-Client": "macos"}

	now := time.Now().UTC()
	chosen := 0
	visIn, visOut := "4", "4"
	hidIn, hidOut := "99", hiddenSecret
	draft := contest.Draft{
		Title:           "Worker Recovery Contest",
		Instructions:    "Submit code and wait for grading.",
		OpensAt:         now.Add(time.Hour),
		ClosesAt:        now.Add(2 * time.Hour),
		DurationMinutes: 30,
		StudentIDs:      []string{student.User.ID},
		Questions: []contest.Question{
			{Kind: "mcq", Prompt: "2+2?", Points: 5, Choices: []string{"4", "5"}, CorrectChoice: &chosen},
			{
				Kind:   "coding",
				Prompt: "Echo stdin",
				Points: 15,
				TestCases: []contest.TestCase{
					{Input: &visIn, Expected: &visOut},
					{Input: &hidIn, Expected: &hidOut, Hidden: true},
				},
			},
		},
	}

	created := decode[contest.Record](t, requireStatus(t, api, http.StatusCreated, http.MethodPost, "/api/staff/contests", teacher.Token, "", map[string]any{"draft": draft}))
	requireStatus(t, api, http.StatusOK, http.MethodPost, "/api/staff/contests/"+created.ID+"/publish", teacher.Token, "", map[string]int{"revision": 1})
	if _, err := api.DB.Exec(t.Context(), `UPDATE assessments SET opens_at=$2, closes_at=$3 WHERE id=$1`, created.ID, now.Add(-5*time.Minute), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	callAttempt := func(body map[string]any) contest.AttemptView {
		t.Helper()
		body["deviceId"] = device
		res, err := api.callWithHeaders(t.Context(), http.MethodPost, "/api/v1/contests/"+created.ID+"/attempt", student.Token, "", body, nativeHeaders)
		if err != nil || res.status != http.StatusOK {
			t.Fatalf("attempt %v: status=%d err=%v raw=%s", body["action"], res.status, err, res.raw)
		}
		assertNoSecret(t, res.raw, hiddenSecret, fmt.Sprintf("attempt %v", body["action"]))
		return decode[contest.AttemptView](t, res)
	}

	started := callAttempt(map[string]any{"action": "start"})
	mappedCorrect := 0
	for i, c := range started.Questions[0].Choices {
		if c == "4" {
			mappedCorrect = i
		}
	}
	subID := uuid.NewString()
	codeView := callAttempt(map[string]any{
		"action":        "code",
		"requestId":     subID,
		"questionIndex": 1,
		"language":      "python",
		"source":        "print(input())",
		"lease":         started.Lease,
	})
	if codeView.CodingSubmissionID == "" {
		t.Fatalf("missing coding submission ID: %#v", codeView)
	}
	expectedSubID.Store(codeView.CodingSubmissionID)

	callAttempt(map[string]any{
		"action":   "submit",
		"lease":    started.Lease,
		"revision": started.Revision,
		"answers":  map[int]contest.Answer{0: {Choice: &mappedCorrect}},
	})

	// Worker 1 claims the job, starts judging, and is cancelled mid-flight.
	stopWorker1 := startContestWorker(t, api)
	select {
	case <-firstStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for worker 1 to invoke judge")
	}
	stopWorker1()

	var status string
	var generation int
	if err := api.DB.QueryRow(t.Context(), `
		SELECT status, generation FROM assessment_coding_submissions WHERE id=$1
	`, codeView.CodingSubmissionID).Scan(&status, &generation); err != nil {
		t.Fatal(err)
	}
	if status != "judging" || generation != 1 {
		t.Fatalf("expected interrupted in-flight row (judging, gen 1), got status=%q generation=%d", status, generation)
	}

	// Expire the lease and launch Worker 2 to recover the interrupted claim.
	if _, err := api.DB.Exec(t.Context(), `
		UPDATE assessment_coding_submissions
		SET claim_after = statement_timestamp() - interval '1 second'
		WHERE id = $1
	`, codeView.CodingSubmissionID); err != nil {
		t.Fatal(err)
	}

	stopWorker2 := startContestWorker(t, api)
	deadline := time.Now().Add(15 * time.Second)
	var gradingView contest.AttemptView
	for {
		gradingView = callAttempt(map[string]any{"action": "grading"})
		if gradingView.GradingStatus == "awaiting_release" &&
			len(gradingView.CodingSubmissions) == 1 &&
			gradingView.CodingSubmissions[0].Status == "accepted" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for recovered grading: %#v", gradingView)
		}
		time.Sleep(50 * time.Millisecond)
	}
	stopWorker2()

	// Stale generation-1 completion must be fenced out and must not overwrite generation-2 acceptance.
	repo := postgres.NewContestAttemptRepo(api.Tx)
	if err := repo.FinishCode(t.Context(), postgres.ContestJudgeJob{
		ID:         codeView.CodingSubmissionID,
		Generation: 1,
	}, "Wrong Answer"); err != nil {
		t.Fatal(err)
	}

	var verdict string
	if err := api.DB.QueryRow(t.Context(), `
		SELECT status, COALESCE(verdict, ''), generation
		FROM assessment_coding_submissions WHERE id=$1
	`, codeView.CodingSubmissionID).Scan(&status, &verdict, &generation); err != nil {
		t.Fatal(err)
	}
	if status != "accepted" || verdict != "Accepted" || generation != 2 {
		t.Fatalf("unexpected submission state after recovery & stale fencing: status=%q verdict=%q gen=%d", status, verdict, generation)
	}

	var resultStatus string
	var points int
	if err := api.DB.QueryRow(t.Context(), `
		SELECT status, points FROM assessment_attempt_results WHERE attempt_id=$1 AND version=1
	`, started.AttemptID).Scan(&resultStatus, &points); err != nil {
		t.Fatal(err)
	}
	if resultStatus != "ready" || points != 20 {
		t.Fatalf("unexpected score result: status=%q points=%d", resultStatus, points)
	}
	if calls.Load() < 2 {
		t.Fatalf("expected at least 2 judge calls across restart, got %d", calls.Load())
	}
}

func TestContestEndToEndPilotBurst(t *testing.T) {
	const hiddenSecret = "HIDDEN_PILOT_SECRET"
	judgeToken := strings.Repeat("pilot-judge-secret", 2)

	judge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var grade grader.Request
		if req.Method != http.MethodPost ||
			req.Header.Get("Authorization") != "Bearer "+judgeToken ||
			json.NewDecoder(req.Body).Decode(&grade) != nil ||
			len(grade.Cases) != 2 ||
			grade.Cases[0].Hidden ||
			!grade.Cases[1].Hidden ||
			grade.Cases[1].Expected == nil ||
			*grade.Cases[1].Expected != hiddenSecret {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"verdict": "Accepted", "passed": 2, "total": 2, "trace": hiddenSecret})
	}))
	t.Cleanup(judge.Close)

	api := startConfiguredAPI(t, enableAllContestFlags(judge.URL, judgeToken))
	stopWorker := startContestWorker(t, api)
	defer stopWorker()

	teacher := loginAs(t, api, "teacher")
	admin := loginAs(t, api, "admin")
	seededStudent := loginAs(t, api, "student")
	reviewer2 := contestTestUser(t, api, admin.Token, "teacher", "pilot-reviewer2")
	outsider := contestTestUser(t, api, admin.Token, "student", "pilot-outsider")

	type cohortMember struct {
		id    string
		token string
		dev   string
	}
	// Seed 22 enrolled students (seeded student + 21 cohort users) so even with 1 withheld
	// attempt there are 21 ranked rows spanning leaderboard page 0 (20 rows) and page 1 (1 row).
	cohort := make([]cohortMember, 0, 22)
	cohort = append(cohort, cohortMember{id: seededStudent.User.ID, token: seededStudent.Token, dev: uuid.NewString()})
	rosterIDs := make([]string, 0, 22)
	rosterIDs = append(rosterIDs, seededStudent.User.ID)
	for i := range 21 {
		u := contestTestUser(t, api, admin.Token, "student", fmt.Sprintf("pilot-s%02d", i+1))
		cohort = append(cohort, cohortMember{id: u.User.ID, token: u.Token, dev: uuid.NewString()})
		rosterIDs = append(rosterIDs, u.User.ID)
	}

	now := time.Now().UTC()
	correct := 1
	visIn, visOut := "2", "4"
	hidIn, hidOut := "9", hiddenSecret
	draft := contest.Draft{
		Title:           "Pilot Burst Contest",
		Instructions:    "Complete the MCQ and coding question inside the macOS app.",
		OpensAt:         now.Add(time.Hour),
		ClosesAt:        now.Add(2 * time.Hour),
		DurationMinutes: 45,
		StudentIDs:      rosterIDs,
		Questions: []contest.Question{
			{
				Kind:          "mcq",
				Prompt:        "What is 2 + 2?",
				Points:        5,
				Choices:       []string{"3", "4"},
				CorrectChoice: &correct,
				Explanation:   "2 + 2 equals 4.",
			},
			{
				Kind:   "coding",
				Prompt: "Square the input integer.",
				Points: 15,
				TestCases: []contest.TestCase{
					{Input: &visIn, Expected: &visOut},
					{Input: &hidIn, Expected: &hidOut, Hidden: true},
				},
			},
		},
	}

	created := decode[contest.Record](t, requireStatus(t, api, http.StatusCreated, http.MethodPost, "/api/staff/contests", teacher.Token, "", map[string]any{"draft": draft}))
	requireStatus(t, api, http.StatusOK, http.MethodPost, "/api/staff/contests/"+created.ID+"/publish", teacher.Token, "", map[string]int{"revision": 1})
	if _, err := api.DB.Exec(t.Context(), `UPDATE assessments SET opens_at=$2, closes_at=$3 WHERE id=$1`, created.ID, now.Add(-5*time.Minute), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	nativeHeaders := map[string]string{"X-LMS-Contest-Client": "macos"}
	sharedSource := "def solve(n):\n    total = n * n\n    if total < 0:\n        return 0\n    return total\n"

	attemptIDs := make([]string, len(cohort))
	burstErrs := make([]error, len(cohort))
	var wg sync.WaitGroup
	for i, member := range cohort {
		wg.Go(func() {
			postAttempt := func(body map[string]any) (contest.AttemptView, error) {
				body["deviceId"] = member.dev
				res, err := api.callWithHeaders(t.Context(), http.MethodPost, "/api/v1/contests/"+created.ID+"/attempt", member.token, "", body, nativeHeaders)
				if err != nil {
					return contest.AttemptView{}, err
				}
				if res.status != http.StatusOK {
					return contest.AttemptView{}, fmt.Errorf("student %d action %v: status %d: %s", i, body["action"], res.status, res.raw)
				}
				if bytes.Contains(res.raw, []byte(hiddenSecret)) {
					return contest.AttemptView{}, fmt.Errorf("student %d action %v leaked hidden secret", i, body["action"])
				}
				var view contest.AttemptView
				if err := json.Unmarshal(res.raw, &view); err != nil {
					return contest.AttemptView{}, err
				}
				return view, nil
			}

			postEvents := func(batchID string, events []map[string]any) (contest.EventBatchResult, error) {
				res, err := api.callWithHeaders(t.Context(), http.MethodPost, "/api/v1/contests/"+created.ID+"/events", member.token, "", map[string]any{
					"attemptId": attemptIDs[i],
					"deviceId":  member.dev,
					"batchId":   batchID,
					"events":    events,
				}, nativeHeaders)
				if err != nil {
					return contest.EventBatchResult{}, err
				}
				if res.status != http.StatusOK {
					return contest.EventBatchResult{}, fmt.Errorf("student %d events: status %d: %s", i, res.status, res.raw)
				}
				if bytes.Contains(res.raw, []byte(hiddenSecret)) {
					return contest.EventBatchResult{}, fmt.Errorf("student %d events leaked hidden secret", i)
				}
				var out contest.EventBatchResult
				if err := json.Unmarshal(res.raw, &out); err != nil {
					return contest.EventBatchResult{}, err
				}
				return out, nil
			}

			startView, err := postAttempt(map[string]any{"action": "start"})
			if err != nil {
				burstErrs[i] = err
				return
			}
			attemptIDs[i] = startView.AttemptID

			mcqChoice := -1
			for idx, choice := range startView.Questions[0].Choices {
				if choice == "4" {
					mcqChoice = idx
				}
			}
			if mcqChoice < 0 {
				burstErrs[i] = fmt.Errorf("student %d could not find expected MCQ choice", i)
				return
			}
			answers := map[int]contest.Answer{0: {Choice: &mcqChoice}}

			saveView, err := postAttempt(map[string]any{
				"action":   "save",
				"lease":    startView.Lease,
				"revision": startView.Revision,
				"answers":  answers,
			})
			if err != nil {
				burstErrs[i] = err
				return
			}

			codeSrc := fmt.Sprintf("def solve(n):\n    res_%d = n * n\n    return res_%d\n", i, i)
			if i == 0 {
				codeSrc = sharedSource
			} else if i == 1 {
				codeSrc = strings.ReplaceAll(sharedSource, "total", "value")
			}

			if _, err := postAttempt(map[string]any{
				"action":        "code",
				"requestId":     uuid.NewString(),
				"questionIndex": 1,
				"language":      "python",
				"source":        codeSrc,
				"lease":         saveView.Lease,
			}); err != nil {
				burstErrs[i] = err
				return
			}

			cleanBatch := []map[string]any{
				{"seq": 1, "eventType": "nav.question", "questionIndex": 1, "payload": map[string]any{}},
				{"seq": 2, "eventType": "code.snapshot", "questionIndex": 1, "payload": map[string]any{"language": "python", "charCount": len(codeSrc), "source": codeSrc}},
			}
			if _, err := postEvents(uuid.NewString(), cleanBatch); err != nil {
				burstErrs[i] = err
				return
			}

			if i == 0 {
				flaggedBatchID := uuid.NewString()
				flaggedEvents := []map[string]any{
					{"seq": 3, "eventType": "editor.paste", "questionIndex": 1, "payload": map[string]any{"charCount": 460}},
					{"seq": 4, "eventType": "window.blur", "payload": map[string]any{"source": "app_switch"}},
				}
				firstIngest, err := postEvents(flaggedBatchID, flaggedEvents)
				if err != nil {
					burstErrs[i] = err
					return
				}
				if firstIngest.Duplicate || firstIngest.Accepted != 2 {
					burstErrs[i] = fmt.Errorf("unexpected flagged ingest result: %#v", firstIngest)
					return
				}
				retryIngest, err := postEvents(flaggedBatchID, flaggedEvents)
				if err != nil {
					burstErrs[i] = err
					return
				}
				if !retryIngest.Duplicate || retryIngest.Accepted != 0 {
					burstErrs[i] = fmt.Errorf("expected idempotent duplicate batch retry, got %#v", retryIngest)
					return
				}
			}

			if _, err := postAttempt(map[string]any{
				"action":   "submit",
				"lease":    saveView.Lease,
				"revision": saveView.Revision,
				"answers":  answers,
			}); err != nil {
				burstErrs[i] = err
				return
			}
		})
	}
	wg.Wait()
	for _, err := range burstErrs {
		if err != nil {
			t.Fatal(err)
		}
	}

	// Wait for background worker to judge all 22 official submissions and materialize scores.
	settleDeadline := time.Now().Add(20 * time.Second)
	for {
		var acceptedCount, scoredCount int
		if err := api.DB.QueryRow(t.Context(), `
			SELECT
				(SELECT count(*) FROM assessment_coding_submissions s
				 JOIN assessment_attempts p ON p.id = s.attempt_id
				 WHERE p.assessment_id = $1::uuid AND s.status = 'accepted'),
				(SELECT count(*) FROM assessment_attempts
				 WHERE assessment_id = $1::uuid AND score_computed_at IS NOT NULL)
		`, created.ID).Scan(&acceptedCount, &scoredCount); err != nil {
			t.Fatal(err)
		}
		if acceptedCount == len(cohort) && scoredCount == len(cohort) {
			break
		}
		if time.Now().After(settleDeadline) {
			t.Fatalf("timed out waiting for pilot burst grading/scoring: accepted=%d scored=%d want=%d", acceptedCount, scoredCount, len(cohort))
		}
		time.Sleep(50 * time.Millisecond)
	}

	gradingRes, err := api.callWithHeaders(t.Context(), http.MethodPost, "/api/v1/contests/"+created.ID+"/attempt", cohort[0].token, "", map[string]any{
		"action":   "grading",
		"deviceId": cohort[0].dev,
	}, nativeHeaders)
	if err != nil || gradingRes.status != http.StatusOK {
		t.Fatalf("grading status check failed: status=%d err=%v raw=%s", gradingRes.status, err, gradingRes.raw)
	}
	assertNoSecret(t, gradingRes.raw, hiddenSecret, "grading status")
	gradingView := decode[contest.AttemptView](t, gradingRes)
	if gradingView.GradingStatus != "awaiting_release" {
		t.Fatalf("expected awaiting_release after worker settled, got %q", gradingView.GradingStatus)
	}

	// Staff assigns secondary reviewer, runs similarity, and withholds flagged student[0] in Results v1.
	requireStatus(t, api, http.StatusOK, http.MethodPost, "/api/staff/contests/"+created.ID+"/reviewers", teacher.Token, "", map[string]string{"reviewerId": reviewer2.User.ID})

	simRes := requireStatus(t, api, http.StatusOK, http.MethodPost, "/api/staff/contests/"+created.ID+"/similarity", teacher.Token, "", nil)
	assertNoSecret(t, simRes.raw, hiddenSecret, "similarity response")
	simBody := decode[struct {
		Pairs []contest.SimilarityPair `json:"pairs"`
	}](t, simRes)
	if len(simBody.Pairs) < 1 || simBody.Pairs[0].Similarity < 0.70 {
		t.Fatalf("expected normalized similarity match, got %#v", simBody.Pairs)
	}

	queueRes := requireStatus(t, api, http.StatusOK, http.MethodGet, "/api/staff/contests/"+created.ID+"/integrity", teacher.Token, "", nil)
	assertNoSecret(t, queueRes.raw, hiddenSecret, "integrity queue")
	queue := decode[contest.ReviewQueueView](t, queueRes)
	var flaggedFound bool
	for _, rv := range queue.Reviews {
		if rv.AttemptID == attemptIDs[0] {
			flaggedFound = rv.Status == "flagged"
		}
	}
	if !flaggedFound {
		t.Fatalf("expected student 0 to be flagged in review queue: %#v", queue.Reviews)
	}

	requireStatus(t, api, http.StatusOK, http.MethodPost, "/api/staff/contests/"+created.ID+"/attempts/"+attemptIDs[0]+"/review", teacher.Token, "", map[string]string{
		"status": "under_review",
		"reason": "Large paste and focus loss signals require confirmation.",
	})

	// Publish Results v1 with student[0] withheld.
	rel1 := decode[contest.ReleaseRecord](t, requireStatus(t, api, http.StatusOK, http.MethodPost, "/api/staff/contests/"+created.ID+"/results/publish", teacher.Token, "", map[string]any{
		"reason":              "Initial pilot results release",
		"releaseExplanations": false,
	}))
	if rel1.Version != 1 {
		t.Fatalf("expected release version 1, got %d", rel1.Version)
	}

	withheldRepRes := requireStatus(t, api, http.StatusOK, http.MethodGet, "/api/contests/"+created.ID+"/report", cohort[0].token, "", nil)
	assertNoSecret(t, withheldRepRes.raw, hiddenSecret, "withheld report")
	withheldRep := decode[contest.StudentReport](t, withheldRepRes)
	if withheldRep.Status != "withheld" || withheldRep.Rank != nil || withheldRep.Points != nil {
		t.Fatalf("expected withheld report for flagged student, got %#v", withheldRep)
	}

	// Verify cohort leaderboard paginates cleanly across page 0 (20 rows) & page 1 (1 row) when 21 of 22 are ranked.
	lbPage0Res := requireStatus(t, api, http.StatusOK, http.MethodGet, "/api/contests/"+created.ID+"/leaderboard?page=0", cohort[1].token, "", nil)
	assertNoSecret(t, lbPage0Res.raw, hiddenSecret, "leaderboard page 0")
	lbPage0 := decode[contest.LeaderboardPage](t, lbPage0Res)
	if !lbPage0.HasMore || lbPage0.TotalRanked != 21 || len(lbPage0.Items) != 20 {
		t.Fatalf("unexpected leaderboard page 0: totalRanked=%d len=%d hasMore=%v", lbPage0.TotalRanked, len(lbPage0.Items), lbPage0.HasMore)
	}

	lbPage1Res := requireStatus(t, api, http.StatusOK, http.MethodGet, "/api/contests/"+created.ID+"/leaderboard?page=1", cohort[1].token, "", nil)
	assertNoSecret(t, lbPage1Res.raw, hiddenSecret, "leaderboard page 1")
	lbPage1 := decode[contest.LeaderboardPage](t, lbPage1Res)
	if lbPage1.HasMore || lbPage1.TotalRanked != 21 || len(lbPage1.Items) != 1 {
		t.Fatalf("unexpected leaderboard page 1: totalRanked=%d len=%d hasMore=%v", lbPage1.TotalRanked, len(lbPage1.Items), lbPage1.HasMore)
	}

	// Unenrolled student gets 404 on private student leaderboard/report and 403 on staff results.
	requireStatus(t, api, http.StatusNotFound, http.MethodGet, "/api/contests/"+created.ID+"/leaderboard?page=0", outsider.Token, "", nil)
	requireStatus(t, api, http.StatusNotFound, http.MethodGet, "/api/contests/"+created.ID+"/report", outsider.Token, "", nil)
	requireStatus(t, api, http.StatusForbidden, http.MethodGet, "/api/staff/contests/"+created.ID+"/results?page=0", outsider.Token, "", nil)

	// Student[0] files an appeal, reviewer2 resolves it, teacher clears student[0], and publishes Results v2.
	appeal := decode[contest.Appeal](t, requireStatus(t, api, http.StatusCreated, http.MethodPost, "/api/contests/"+created.ID+"/appeal", cohort[0].token, "", map[string]string{
		"kind":    "appeal",
		"message": "I pasted my own helper from local notes and switched windows briefly.",
	}))
	resolved := decode[contest.Appeal](t, requireStatus(t, api, http.StatusOK, http.MethodPost, "/api/staff/contests/"+created.ID+"/appeals/"+appeal.ID+"/resolve", reviewer2.Token, "", map[string]string{
		"status":     "resolved",
		"resolution": "Verified explanation with secondary reviewer.",
	}))
	if resolved.Status != "resolved" || resolved.ReviewerID != reviewer2.User.ID {
		t.Fatalf("unexpected resolved appeal: %#v", resolved)
	}

	requireStatus(t, api, http.StatusOK, http.MethodPost, "/api/staff/contests/"+created.ID+"/attempts/"+attemptIDs[0]+"/review", teacher.Token, "", map[string]string{
		"status": "cleared",
		"reason": "Cleared following appeal resolution.",
	})

	rel2 := decode[contest.ReleaseRecord](t, requireStatus(t, api, http.StatusOK, http.MethodPost, "/api/staff/contests/"+created.ID+"/results/publish", teacher.Token, "", map[string]any{
		"reason":              "Cleared appeal and released MCQ explanations",
		"releaseExplanations": true,
	}))
	if rel2.Version != 2 || rel2.Reason != "Cleared appeal and released MCQ explanations" {
		t.Fatalf("unexpected release v2: %#v", rel2)
	}

	// Student[0] sets alias and reads personal v2 report and updated leaderboard.
	requireStatus(t, api, http.StatusOK, http.MethodPut, "/api/contests/"+created.ID+"/alias", cohort[0].token, "", map[string]string{
		"alias": "PilotAce",
	})
	rep2Res := requireStatus(t, api, http.StatusOK, http.MethodGet, "/api/contests/"+created.ID+"/report", cohort[0].token, "", nil)
	assertNoSecret(t, rep2Res.raw, hiddenSecret, "student v2 report")
	rep2 := decode[contest.StudentReport](t, rep2Res)
	if rep2.Version != 2 || rep2.Status != "published" || rep2.Points == nil || *rep2.Points != 20 ||
		rep2.Breakdown[0].Explanation != "2 + 2 equals 4." {
		t.Fatalf("unexpected v2 personal report: %#v", rep2)
	}

	lbAfterAlias := decode[contest.LeaderboardPage](t, requireStatus(t, api, http.StatusOK, http.MethodGet, "/api/contests/"+created.ID+"/leaderboard?page=0", cohort[0].token, "", nil))
	if lbAfterAlias.TotalRanked != 22 || lbAfterAlias.SelfRow == nil || lbAfterAlias.SelfRow.DisplayName != "PilotAce" {
		t.Fatalf("expected SelfRow with PilotAce alias after v2 publish, got %#v", lbAfterAlias)
	}
	lbV2Page1 := decode[contest.LeaderboardPage](t, requireStatus(t, api, http.StatusOK, http.MethodGet, "/api/contests/"+created.ID+"/leaderboard?page=1", cohort[0].token, "", nil))
	if len(lbV2Page1.Items) != 2 || lbV2Page1.HasMore {
		t.Fatalf("expected 2 items on page 1 after clearing withheld student, got %#v", lbV2Page1)
	}
}
