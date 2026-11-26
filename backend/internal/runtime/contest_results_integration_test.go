//go:build integration

package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"vedam/lms/internal/adapters/grader"
	"vedam/lms/internal/domain/contest"
)

func TestContestResultsWorkflow(t *testing.T) {
	const hiddenSecret = "HIDDEN_RESULTS_CASE"
	judgeToken := strings.Repeat("results-judge-secret", 2)
	judge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var grade grader.Request
		if json.NewDecoder(req.Body).Decode(&grade) != nil {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		verdict := "Accepted"
		passed := 2
		if strings.Contains(grade.Source, "wrong") {
			verdict = "Wrong Answer"
			passed = 1
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"verdict": verdict, "passed": passed, "total": 2})
	}))
	t.Cleanup(judge.Close)

	api := startConfiguredAPI(t, func(cfg *Config) {
		cfg.ContestAuthoring = true
		cfg.ContestAttempts = true
		cfg.ContestGrading = true
		cfg.ContestIntegrity = true
		cfg.ContestResults = true
		cfg.ContestJudgeURL = judge.URL
		cfg.ContestJudgeToken = judgeToken
	})

	teacher := loginAs(t, api, "teacher")
	student := loginAs(t, api, "student")
	admin := loginAs(t, api, "admin")
	outsideStudent := contestTestUser(t, api, admin.Token, "student", "results-outside")

	// Status endpoint advertises resultsEnabled.
	stRes := requireStatus(t, api, 200, "GET", "/api/contests/status", student.Token, "", nil)
	stBody := decode[map[string]bool](t, stRes)
	if !stBody["resultsEnabled"] {
		t.Fatalf("expected resultsEnabled=true, got %#v", stBody)
	}

	// Create 22 students total (seeded student + 21 extra cohort students) to test >20 leaderboard pagination & ties.
	type cohortUser struct {
		id, token, dev string
	}
	cohort := []cohortUser{{id: student.User.ID, token: student.Token, dev: uuid.NewString()}}
	rosterIDs := []string{student.User.ID}
	for i := range 21 {
		u := contestTestUser(t, api, admin.Token, "student", fmt.Sprintf("res-%02d", i))
		cohort = append(cohort, cohortUser{id: u.User.ID, token: u.Token, dev: uuid.NewString()})
		rosterIDs = append(rosterIDs, u.User.ID)
	}

	now := time.Now().UTC()
	chosen := 0
	visIn, visOut := "1", "1"
	hidIn, hidOut := "2", hiddenSecret
	draft := contest.Draft{
		Title:           "Final Results Contest",
		Instructions:    "Scored contest",
		OpensAt:         now.Add(time.Hour),
		ClosesAt:        now.Add(4 * time.Hour),
		DurationMinutes: 30,
		StudentIDs:      rosterIDs,
		Questions: []contest.Question{
			{
				Kind:          "mcq",
				Prompt:        "2+2?",
				Points:        5,
				Choices:       []string{"4", "5"},
				CorrectChoice: &chosen,
				Explanation:   "Basic arithmetic shows 2+2=4.",
			},
			{
				Kind:   "coding",
				Prompt: "Solve",
				Points: 15,
				TestCases: []contest.TestCase{
					{Input: &visIn, Expected: &visOut},
					{Input: &hidIn, Expected: &hidOut, Hidden: true},
				},
			},
		},
	}

	created := decode[contest.Record](t, requireStatus(t, api, 201, "POST", "/api/staff/contests", teacher.Token, "", map[string]any{"draft": draft}))
	requireStatus(t, api, 200, "POST", "/api/staff/contests/"+created.ID+"/publish", teacher.Token, "", map[string]int{"revision": 1})
	if _, err := api.DB.Exec(t.Context(), "UPDATE assessments SET opens_at=$2, closes_at=$3 WHERE id=$1", created.ID, now.Add(-time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	// Have cohort[0] and cohort[1] tie with identical elapsed_ms and points (20 pts);
	// cohort[2] withheld via under_review; cohort[3..21] score 5 pts (MCQ only) with varying elapsed_ms.
	baseStart := now.Add(-20 * time.Minute)
	attemptIDs := make([]string, len(cohort))
	for i, cu := range cohort {
		res, err := api.callWithHeaders(t.Context(), "POST", "/api/v1/contests/"+created.ID+"/attempt", cu.token, "", map[string]any{
			"action": "start", "deviceId": cu.dev,
		}, map[string]string{"X-LMS-Contest-Client": "macos"})
		if err != nil || res.status != 200 {
			t.Fatalf("start[%d] failed: %v %d %s", i, err, res.status, res.raw)
		}
		v := decode[contest.AttemptView](t, res)
		attemptIDs[i] = v.AttemptID

		// Set deterministic started_at / choice_saved_at / submitted_at to verify tie-breaking.
		elapsed := time.Duration(10+i) * time.Second
		if i == 0 || i == 1 {
			elapsed = 10 * time.Second // Exact tie between student 0 and student 1 at 20 pts, 10s!
		}
		saveAt := baseStart.Add(elapsed)
		// Find original choice 0 ("4")'s mapped display index.
		mappedCorrect := 0
		for dispIdx, label := range v.Questions[0].Choices {
			if label == "4" {
				mappedCorrect = dispIdx
			}
		}

		if i <= 2 {
			// Official coding submission accepted for 0, 1, 2
			codeRes, err := api.callWithHeaders(t.Context(), "POST", "/api/v1/contests/"+created.ID+"/attempt", cu.token, "", map[string]any{
				"action": "code", "deviceId": cu.dev, "lease": v.Lease, "requestId": uuid.NewString(),
				"questionIndex": 1, "language": "python", "source": fmt.Sprintf("print('sol-%d')", i),
			}, map[string]string{"X-LMS-Contest-Client": "macos"})
			if err != nil || codeRes.status != 200 {
				t.Fatalf("code submit[%d] failed: %v %d %s", i, err, codeRes.status, codeRes.raw)
			}
			cView := decode[contest.AttemptView](t, codeRes)
			if _, err := api.DB.Exec(t.Context(),
				"UPDATE assessment_coding_submissions SET status='accepted', verdict='Accepted', submitted_at=$2 WHERE id=$1",
				cView.CodingSubmissionID, saveAt); err != nil {
				t.Fatal(err)
			}
		}

		subRes, err := api.callWithHeaders(t.Context(), "POST", "/api/v1/contests/"+created.ID+"/attempt", cu.token, "", map[string]any{
			"action": "submit", "deviceId": cu.dev, "lease": v.Lease, "revision": v.Revision,
			"answers": map[int]contest.Answer{0: {Choice: &mappedCorrect}},
		}, map[string]string{"X-LMS-Contest-Client": "macos"})
		if err != nil || subRes.status != 200 {
			t.Fatalf("submit[%d] failed: %v %d %s", i, err, subRes.status, subRes.raw)
		}
		choiceSavedJSON := fmt.Sprintf(`{"0":%q}`, saveAt.Format(time.RFC3339Nano))
		if _, err := api.DB.Exec(t.Context(),
			"UPDATE assessment_attempts SET started_at=$2, choice_saved_at=$3::jsonb, submitted_at=$4 WHERE id=$1",
			attemptIDs[i], baseStart, choiceSavedJSON, saveAt.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	// Mark student 2 as under_review so their row is automatically withheld!
	requireStatus(t, api, 200, "POST", "/api/staff/contests/"+created.ID+"/attempts/"+attemptIDs[2]+"/review", teacher.Token, "", map[string]string{
		"status": "under_review", "reason": "Manual verification pending for attempt 2.",
	})

	// Set contest alias for last student (cohort[21], outside page 0!).
	requireStatus(t, api, 200, "PUT", "/api/contests/"+created.ID+"/alias", cohort[21].token, "", map[string]string{
		"alias": "MidnightCoder",
	})

	// Prior to release, report and leaderboard return 404.
	requireStatus(t, api, 404, "GET", "/api/contests/"+created.ID+"/report", student.Token, "", nil)
	requireStatus(t, api, 404, "GET", "/api/contests/"+created.ID+"/leaderboard", student.Token, "", nil)

	// First release: releaseExplanations = false.
	rel1 := decode[contest.ReleaseRecord](t, requireStatus(t, api, 200, "POST", "/api/staff/contests/"+created.ID+"/results/publish", teacher.Token, "", map[string]any{
		"reason": "Initial results release", "releaseExplanations": false,
	}))
	if rel1.Version != 1 {
		t.Fatalf("expected version 1, got %d", rel1.Version)
	}

	// Outside student cannot read report or leaderboard.
	requireStatus(t, api, 404, "GET", "/api/contests/"+created.ID+"/report", outsideStudent.Token, "", nil)
	requireStatus(t, api, 404, "GET", "/api/contests/"+created.ID+"/leaderboard", outsideStudent.Token, "", nil)

	// Leaderboard page 0 has 20 items, totalRanked = 21 (22 - 1 withheld), ties on ranks 1, 1, 3.
	lb0Res := requireStatus(t, api, 200, "GET", "/api/contests/"+created.ID+"/leaderboard?page=0", student.Token, "", nil)
	if bytes.Contains(lb0Res.raw, []byte(hiddenSecret)) || bytes.Contains(lb0Res.raw, []byte("@vedam.local")) || bytes.Contains(lb0Res.raw, []byte("Manual verification pending")) || bytes.Contains(lb0Res.raw, []byte("sol-1")) {
		t.Fatalf("leaderboard leaked private data: %s", lb0Res.raw)
	}
	lb0 := decode[contest.LeaderboardPage](t, lb0Res)
	if !lb0.HasMore || lb0.TotalRanked != 21 || len(lb0.Items) != 20 {
		t.Fatalf("unexpected page 0: totalRanked=%d len=%d hasMore=%v", lb0.TotalRanked, len(lb0.Items), lb0.HasMore)
	}
	if lb0.Items[0].Rank != 1 || lb0.Items[1].Rank != 1 || lb0.Items[2].Rank != 3 {
		t.Fatalf("expected competition ranking ties 1,1,3, got %d,%d,%d", lb0.Items[0].Rank, lb0.Items[1].Rank, lb0.Items[2].Rank)
	}

	// Cohort[21] falls outside page 0, yet still receives SelfRow with their contest alias!
	lbLastFromPage0 := decode[contest.LeaderboardPage](t, requireStatus(t, api, 200, "GET", "/api/contests/"+created.ID+"/leaderboard?page=0", cohort[21].token, "", nil))
	if lbLastFromPage0.SelfRow == nil || lbLastFromPage0.SelfRow.Rank != 21 || lbLastFromPage0.SelfRow.DisplayName != "MidnightCoder" {
		t.Fatalf("expected SelfRow rank 21 MidnightCoder on page 0, got %#v", lbLastFromPage0.SelfRow)
	}
	// Page 1 shows remaining item cleanly.
	lb1 := decode[contest.LeaderboardPage](t, requireStatus(t, api, 200, "GET", "/api/contests/"+created.ID+"/leaderboard?page=1", cohort[21].token, "", nil))
	if lb1.HasMore || len(lb1.Items) != 1 || lb1.Items[0].DisplayName != "MidnightCoder" {
		t.Fatalf("unexpected page 1: %#v", lb1)
	}

	// Withheld student 2 sees private under_review report with review reason, no rank/points, and is absent from leaderboard.
	rep2 := decode[contest.StudentReport](t, requireStatus(t, api, 200, "GET", "/api/contests/"+created.ID+"/report", cohort[2].token, "", nil))
	if rep2.Status != "withheld" || rep2.Rank != nil || rep2.Points != nil || !strings.Contains(rep2.ReviewNote, "Manual verification pending") {
		t.Fatalf("unexpected withheld report: %#v", rep2)
	}

	// Student 0 report has points=20, rank=1, coding code, and hidden explanation (releaseExplanations=false).
	rep0Res := requireStatus(t, api, 200, "GET", "/api/contests/"+created.ID+"/report", student.Token, "", nil)
	if bytes.Contains(rep0Res.raw, []byte(hiddenSecret)) {
		t.Fatalf("student report leaked hidden test")
	}
	rep0 := decode[contest.StudentReport](t, rep0Res)
	if rep0.Status != "published" || rep0.Rank == nil || *rep0.Rank != 1 || rep0.Points == nil || *rep0.Points != 20 || rep0.McqPoints != 5 || rep0.CodingPoints != 15 {
		t.Fatalf("unexpected published report: %#v", rep0)
	}
	if rep0.Breakdown[0].Explanation != "" || rep0.Breakdown[0].CorrectChoice != nil {
		t.Fatalf("expected MCQ explanation & key hidden when releaseExplanations=false, got %#v", rep0.Breakdown[0])
	}

	// Dedicated dashboard summary endpoint includes published & withheld summaries.
	dashSum := decode[struct {
		Items []contest.StudentResultSummary `json:"items"`
	}](t, requireStatus(t, api, 200, "GET", "/api/contests/results", student.Token, "", nil))
	if len(dashSum.Items) != 1 || dashSum.Items[0].Version != 1 || *dashSum.Items[0].Points != 20 {
		t.Fatalf("unexpected dashboard summary: %#v", dashSum.Items)
	}

	// Regrade requires a reason when version >= 1, then publishes version 2 retaining version 1 audit trail & revealing explanations.
	requireStatus(t, api, 400, "POST", "/api/staff/contests/"+created.ID+"/results/publish", teacher.Token, "", map[string]any{
		"reason": "", "releaseExplanations": true,
	})
	rel2 := decode[contest.ReleaseRecord](t, requireStatus(t, api, 200, "POST", "/api/staff/contests/"+created.ID+"/results/publish", teacher.Token, "", map[string]any{
		"reason": "Publish MCQ explanations", "releaseExplanations": true,
	}))
	if rel2.Version != 2 {
		t.Fatalf("expected version 2 on regrade, got %d", rel2.Version)
	}

	rep0v2 := decode[contest.StudentReport](t, requireStatus(t, api, 200, "GET", "/api/contests/"+created.ID+"/report", student.Token, "", nil))
	if rep0v2.Version != 2 || rep0v2.Breakdown[0].Explanation != "Basic arithmetic shows 2+2=4." || rep0v2.Breakdown[0].CorrectChoice == nil {
		t.Fatalf("expected explanations released in v2, got %#v", rep0v2.Breakdown[0])
	}

	staffRes := decode[contest.StaffResultsView](t, requireStatus(t, api, 200, "GET", "/api/staff/contests/"+created.ID+"/results?page=0", teacher.Token, "", nil))
	if staffRes.LatestVersion != 2 || len(staffRes.Releases) != 2 {
		t.Fatalf("expected 2 preserved release audit records, got %#v", staffRes)
	}
}
