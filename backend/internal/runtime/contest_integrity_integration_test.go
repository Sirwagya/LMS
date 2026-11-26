//go:build integration

package runtime

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"vedam/lms/internal/adapters/postgres"
	"vedam/lms/internal/domain/contest"
)

func TestContestIntegrityWorkflow(t *testing.T) {
	api := startConfiguredAPI(t, func(cfg *Config) {
		cfg.ContestAuthoring = true
		cfg.ContestAttempts = true
		cfg.ContestIntegrity = true
	})

	teacher := loginAs(t, api, "teacher")
	student := loginAs(t, api, "student")
	admin := loginAs(t, api, "admin")
	studentB := contestTestUser(t, api, admin.Token, "student", "integrity-b")
	reviewer2 := contestTestUser(t, api, admin.Token, "teacher", "integrity-reviewer2")
	unrelatedTeacher := contestTestUser(t, api, admin.Token, "teacher", "integrity-unrelated")

	// Status endpoint advertises integrityEnabled.
	statusRes := requireStatus(t, api, 200, "GET", "/api/contests/status", student.Token, "", nil)
	statusBody := decode[map[string]bool](t, statusRes)
	if !statusBody["integrityEnabled"] {
		t.Fatalf("expected integrityEnabled=true, got %#v", statusBody)
	}

	now := time.Now().UTC()
	chosen := 0
	exampleIn, exampleOut := "5", "25"
	hiddenIn, hiddenOut := "6", "36"
	draft := contest.Draft{
		Title:           "Integrity & Replay Contest",
		Instructions:    "Follow contest integrity rules.",
		OpensAt:         now.Add(time.Hour),
		ClosesAt:        now.Add(3 * time.Hour),
		DurationMinutes: 30,
		StudentIDs:      []string{student.User.ID, studentB.User.ID},
		Questions: []contest.Question{
			{Kind: "mcq", Prompt: "Pick one", Choices: []string{"A", "B"}, CorrectChoice: &chosen, Points: 5},
			{
				Kind:      "coding",
				Prompt:    "Square input",
				Points:    15,
				TestCases: []contest.TestCase{{Input: &exampleIn, Expected: &exampleOut}, {Input: &hiddenIn, Expected: &hiddenOut, Hidden: true}},
			},
		},
	}

	created := decode[contest.Record](t, requireStatus(t, api, 201, "POST", "/api/staff/contests", teacher.Token, "", map[string]any{"draft": draft}))
	requireStatus(t, api, 200, "POST", "/api/staff/contests/"+created.ID+"/publish", teacher.Token, "", map[string]int{"revision": 1})
	if _, err := api.DB.Exec(t.Context(), "UPDATE assessments SET opens_at=$2, closes_at=$3 WHERE id=$1", created.ID, now.Add(-time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	startAttempt := func(tok, dev string) contest.AttemptView {
		t.Helper()
		res, err := api.callWithHeaders(t.Context(), "POST", "/api/v1/contests/"+created.ID+"/attempt", tok, "", map[string]any{
			"action": "start", "deviceId": dev,
		}, map[string]string{"X-LMS-Contest-Client": "macos"})
		if err != nil || res.status != 200 {
			t.Fatalf("start attempt failed: %v status=%d raw=%s", err, res.status, res.raw)
		}
		return decode[contest.AttemptView](t, res)
	}

	devA, devB := uuid.NewString(), uuid.NewString()
	viewA := startAttempt(student.Token, devA)
	viewB := startAttempt(studentB.Token, devB)

	// Non-macos / web requests cannot ingest telemetry events.
	webRes, err := api.callWithHeaders(t.Context(), "POST", "/api/v1/contests/"+created.ID+"/events", student.Token, "", map[string]any{
		"attemptId": viewA.AttemptID, "deviceId": devA, "batchId": uuid.NewString(),
		"events": []map[string]any{{"seq": 1, "eventType": "nav.question", "questionIndex": 0, "payload": map[string]any{}}},
	}, nil)
	if err != nil || webRes.status != http.StatusForbidden {
		t.Fatalf("expected 403 without macos header, got %d (%v)", webRes.status, err)
	}

	postEvents := func(tok string, wantStatus int, body map[string]any) contest.EventBatchResult {
		t.Helper()
		res, err := api.callWithHeaders(t.Context(), "POST", "/api/v1/contests/"+created.ID+"/events", tok, "", body,
			map[string]string{"X-LMS-Contest-Client": "macos"})
		if err != nil || res.status != wantStatus {
			t.Fatalf("events status=%d want=%d err=%v raw=%s", res.status, wantStatus, err, res.raw)
		}
		if wantStatus != 200 {
			return contest.EventBatchResult{}
		}
		return decode[contest.EventBatchResult](t, res)
	}

	// Privacy guardrails: prohibited telemetry keys and clipboard text must be rejected.
	postEvents(student.Token, 400, map[string]any{
		"attemptId": viewA.AttemptID, "deviceId": devA, "batchId": uuid.NewString(),
		"events": []map[string]any{{
			"seq": 1, "eventType": "window.blur", "payload": map[string]any{"clipboardText": "secret"},
		}},
	})
	postEvents(student.Token, 400, map[string]any{
		"attemptId": viewA.AttemptID, "deviceId": devA, "batchId": uuid.NewString(),
		"events": []map[string]any{{
			"seq": 1, "eventType": "editor.paste", "questionIndex": 1, "payload": map[string]any{"charCount": 120, "text": "pasted body"},
		}},
	})

	// Retry-safe batchId deduplication & dropped sequence detection (send seq 1, skip seq 2, send seq 3).
	batch1 := uuid.NewString()
	first := postEvents(student.Token, 200, map[string]any{
		"attemptId": viewA.AttemptID, "deviceId": devA, "batchId": batch1,
		"events": []map[string]any{
			{"seq": 1, "eventType": "nav.question", "questionIndex": 1, "payload": map[string]any{}},
			{"seq": 3, "eventType": "editor.paste", "questionIndex": 1, "payload": map[string]any{"charCount": 460}},
			{"seq": 4, "eventType": "window.blur", "payload": map[string]any{"source": "accessibility_voiceover"}},
		},
	})
	if first.Accepted != 3 || first.Duplicate || len(first.MissingSeqs) != 1 || first.MissingSeqs[0] != 2 {
		t.Fatalf("unexpected first batch result: %#v", first)
	}
	retry := postEvents(student.Token, 200, map[string]any{
		"attemptId": viewA.AttemptID, "deviceId": devA, "batchId": batch1,
		"events": []map[string]any{
			{"seq": 1, "eventType": "nav.question", "questionIndex": 1, "payload": map[string]any{}},
		},
	})
	if !retry.Duplicate || retry.Accepted != 0 {
		t.Fatalf("expected duplicate retry no-op, got %#v", retry)
	}

	// Late arrival fills seq 2 gap.
	late := postEvents(student.Token, 200, map[string]any{
		"attemptId": viewA.AttemptID, "deviceId": devA, "batchId": uuid.NewString(),
		"events": []map[string]any{
			{"seq": 2, "eventType": "code.snapshot", "questionIndex": 1, "payload": map[string]any{"language": "python", "charCount": 60, "source": "def solve(n):\n    return n * n\n"}},
		},
	})
	if len(late.MissingSeqs) != 0 {
		t.Fatalf("expected gap closed after late seq 2, got %#v", late)
	}

	// Also drop seq 5 and send seq 6..7 so replay still reports missing interval 5..5.
	postEvents(student.Token, 200, map[string]any{
		"attemptId": viewA.AttemptID, "deviceId": devA, "batchId": uuid.NewString(),
		"events": []map[string]any{
			{"seq": 6, "eventType": "client.reconnect", "payload": map[string]any{"reason": "network"}},
		},
	})

	// Both students save similar Python solutions after starter code, then submit.
	sharedBody := "def solve(n):\n    total = n * n\n    if total < 0:\n        return 0\n    return total\n"
	for _, pair := range []struct {
		tok, dev string
		view     contest.AttemptView
		code     string
	}{
		{student.Token, devA, viewA, sharedBody},
		{studentB.Token, devB, viewB, strings.ReplaceAll(sharedBody, "total", "value")},
	} {
		ans := map[int]contest.Answer{
			0: {Choice: &chosen},
			1: {Lang: "python", Source: pair.code},
		}
		res, err := api.callWithHeaders(t.Context(), "POST", "/api/v1/contests/"+created.ID+"/attempt", pair.tok, "", map[string]any{
			"action": "submit", "deviceId": pair.dev, "lease": pair.view.Lease, "revision": pair.view.Revision, "answers": ans,
		}, map[string]string{"X-LMS-Contest-Client": "macos"})
		if err != nil || res.status != 200 {
			t.Fatalf("submit failed: %v %d %s", err, res.status, res.raw)
		}
	}

	// Unrelated teacher cannot view integrity queue or replay before assignment; owner and assigned reviewer can.
	requireStatus(t, api, 404, "GET", "/api/staff/contests/"+created.ID+"/integrity", unrelatedTeacher.Token, "", nil)
	requireStatus(t, api, 404, "GET", "/api/staff/contests/"+created.ID+"/attempts/"+viewA.AttemptID+"/replay", unrelatedTeacher.Token, "", nil)

	requireStatus(t, api, 200, "POST", "/api/staff/contests/"+created.ID+"/reviewers", teacher.Token, "", map[string]string{"reviewerId": reviewer2.User.ID})

	simRes := requireStatus(t, api, 200, "POST", "/api/staff/contests/"+created.ID+"/similarity", teacher.Token, "", nil)
	simBody := decode[struct {
		Pairs []contest.SimilarityPair `json:"pairs"`
	}](t, simRes)
	if len(simBody.Pairs) != 1 || simBody.Pairs[0].Similarity < 0.70 {
		t.Fatalf("expected 1 high similarity pair across normalized variable names, got %#v", simBody.Pairs)
	}

	queue := decode[contest.ReviewQueueView](t, requireStatus(t, api, 200, "GET", "/api/staff/contests/"+created.ID+"/integrity", reviewer2.Token, "", nil))
	if len(queue.Reviews) != 2 || len(queue.AccessLog) == 0 {
		t.Fatalf("unexpected review queue: %#v", queue)
	}
	var revA contest.ReviewSummary
	for _, rv := range queue.Reviews {
		if rv.AttemptID == viewA.AttemptID {
			revA = rv
		}
	}
	// Automated paste/similarity signals flag for human review without auto-disqualifying.
	if revA.Status != "flagged" {
		t.Fatalf("expected auto-status 'flagged' (not confirmed_violation), got %q", revA.Status)
	}

	replay := decode[contest.AttemptReplayView](t, requireStatus(t, api, 200, "GET", "/api/staff/contests/"+created.ID+"/attempts/"+viewA.AttemptID+"/replay", teacher.Token, "", nil))
	if len(replay.MissingIntervals) != 1 || replay.MissingIntervals[0].FromSeq != 5 || replay.MissingIntervals[0].ToSeq != 5 {
		t.Fatalf("expected missing sequence interval 5..5 in replay, got %#v", replay.MissingIntervals)
	}
	hasServerEvent := false
	for _, ev := range replay.Events {
		if ev.SourceKind == "server" && ev.EventType == "attempt.started" {
			hasServerEvent = true
		}
	}
	if !hasServerEvent {
		t.Fatalf("expected authoritative server events in replay, got %#v", replay.Events)
	}

	// Teacher records under_review decision with reason.
	requireStatus(t, api, 200, "POST", "/api/staff/contests/"+created.ID+"/attempts/"+viewA.AttemptID+"/review", teacher.Token, "", map[string]string{
		"status": "under_review", "reason": "Large paste and normalized similarity match require appeal confirmation.",
	})

	// Student sees private review status and files an appeal, while studentB has no access to A's appeals.
	stStatus := decode[contest.StudentIntegrityStatus](t, requireStatus(t, api, 200, "GET", "/api/contests/"+created.ID+"/appeal", student.Token, "", nil))
	if stStatus.ReviewStatus != "under_review" {
		t.Fatalf("expected student to see under_review status, got %q", stStatus.ReviewStatus)
	}
	appeal := decode[contest.Appeal](t, requireStatus(t, api, 201, "POST", "/api/contests/"+created.ID+"/appeal", student.Token, "", map[string]string{
		"kind": "appeal", "message": "I wrote the helper in my IDE earlier; VoiceOver caused window focus changes.",
	}))

	// Distinct reviewer policy: original reviewer (teacher) cannot self-resolve appeal while reviewer2 is assigned.
	requireStatus(t, api, 400, "POST", "/api/staff/contests/"+created.ID+"/appeals/"+appeal.ID+"/resolve", teacher.Token, "", map[string]string{
		"status": "resolved", "resolution": "Cannot self-resolve",
	})
	resolved := decode[contest.Appeal](t, requireStatus(t, api, 200, "POST", "/api/staff/contests/"+created.ID+"/appeals/"+appeal.ID+"/resolve", reviewer2.Token, "", map[string]string{
		"status": "resolved", "resolution": "Reviewed accessibility interruption and explanation.",
	}))
	if resolved.Status != "resolved" || resolved.ReviewerID != reviewer2.User.ID {
		t.Fatalf("unexpected appeal resolution: %#v", resolved)
	}

	// Retention sweep: backdate both reviews' retain_until; keep attempt A protected while open appeal exists, purge expired attempt B.
	requireStatus(t, api, 201, "POST", "/api/contests/"+created.ID+"/appeal", student.Token, "", map[string]string{
		"kind": "concern", "message": "Requesting secondary log retention check.",
	})
	if _, err := api.DB.Exec(t.Context(), "UPDATE assessment_reviews SET retain_until = clock_timestamp() - interval '1 day'"); err != nil {
		t.Fatal(err)
	}
	intRepo := postgres.NewContestIntegrityRepo(api.Tx)
	deleted, err := intRepo.PurgeExpiredEvidence(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 expired review purged (studentB only, studentA protected by open appeal), got %d", deleted)
	}
}
