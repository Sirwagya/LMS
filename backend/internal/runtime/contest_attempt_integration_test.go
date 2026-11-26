//go:build integration

package runtime

import (
	"bytes"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"vedam/lms/internal/adapters/postgres"
	"vedam/lms/internal/domain/contest"
)

func TestContestAttempts(t *testing.T) {
	api := startAPIWithContests(t, true, true)
	staff := loginAs(t, api, "teacher")
	student := loginAs(t, api, "student")
	admin := loginAs(t, api, "admin")
	outsider := contestTestUser(t, api, admin.Token, "student", "outside-attempt")
	a, b := uuid.NewString(), uuid.NewString()
	first := 0
	input, output, hidden := "example-in", "example-out", "hidden-output"
	now := time.Now().UTC().Truncate(time.Second)
	draft := contest.Draft{
		Title: "Timed contest", Instructions: "Closed book",
		OpensAt: now.Add(time.Hour), ClosesAt: now.Add(2 * time.Hour), DurationMinutes: 30,
		StudentIDs: []string{student.User.ID},
		Questions: []contest.Question{
			{Kind: "mcq", Prompt: "Select one", Points: 5, Choices: []string{"One", "Two"}, CorrectChoice: &first, Explanation: "private explanation"},
			{Kind: "coding", Prompt: "Write a solution", Points: 10, TestCases: []contest.TestCase{
				{Input: &input, Expected: &output}, {Input: &input, Expected: &hidden, Hidden: true},
			}},
		},
	}

	create := func() (string, string) {
		const staffPath = "/api/staff/contests"
		created := decode[contest.Record](t, requireStatus(t, api, 201, "POST", staffPath, staff.Token, "", map[string]any{"draft": draft}))
		return created.ID, staffPath + "/" + created.ID
	}
	publish := func(path string) {
		requireStatus(t, api, 200, "POST", path+"/publish", staff.Token, "", map[string]int{"revision": 1})
	}
	openWindow := func(id string) {
		if _, err := api.DB.Exec(t.Context(), "UPDATE assessments SET opens_at=$2,closes_at=$3 WHERE id=$1", id, now.Add(-40*time.Minute), now.Add(10*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	raw := func(id, token, action, device string, lease, revision int64, answers map[int]contest.Answer, headers map[string]string) (reply, error) {
		return api.callWithHeaders(t.Context(), "POST", "/api/v1/contests/"+id+"/attempt", token, "",
			map[string]any{"action": action, "deviceId": device, "lease": lease, "revision": revision, "answers": answers}, headers)
	}
	native := map[string]string{"X-LMS-Contest-Client": "macos"}
	check := func(id, token, action, device string, lease, revision int64, answers map[int]contest.Answer, status int) reply {
		t.Helper()
		res, err := raw(id, token, action, device, lease, revision, answers, native)
		if err != nil || res.status != status {
			t.Fatalf("%s: want HTTP %d, got %d %s (%v)", action, status, res.status, res.raw, err)
		}
		return res
	}
	state := func(id, action, device string, lease, revision int64, answers map[int]contest.Answer) contest.AttemptView {
		t.Helper()
		return decode[contest.AttemptView](t, check(id, student.Token, action, device, lease, revision, answers, 200))
	}

	id, path := create()
	check(id, student.Token, "state", a, 0, 0, nil, 404)
	publish(path)
	for _, headers := range []map[string]string{nil, {"X-LMS-Contest-Client": "macos", "Origin": api.URL}, {"X-LMS-Contest-Client": "macos", "Sec-Fetch-Site": "same-origin"}} {
		if res, err := raw(id, student.Token, "state", a, 0, 0, nil, headers); err != nil || res.status != http.StatusForbidden {
			t.Fatalf("browser admission = %d (%v)", res.status, err)
		}
	}
	check(id, staff.Token, "state", a, 0, 0, nil, 403)
	check(id, outsider.Token, "start", a, 0, 0, nil, 404)
	if before := state(id, "start", a, 0, 0, nil); before.State != "upcoming" || before.AttemptID != "" || before.Questions != nil {
		t.Fatalf("early start leaked attempt: %#v", before)
	}
	openWindow(id)

	var starts [2]reply
	var failures [2]error
	var wg sync.WaitGroup
	for i := range starts {
		wg.Go(func() { starts[i], failures[i] = raw(id, student.Token, "start", a, 0, 0, nil, native) })
	}
	wg.Wait()
	for i, res := range starts {
		if failures[i] != nil || res.status != 200 || bytes.Contains(res.raw, []byte("hidden-output")) ||
			bytes.Contains(res.raw, []byte("private explanation")) || bytes.Contains(res.raw, []byte("correctChoice")) {
			t.Fatalf("bad start: %d %s (%v)", res.status, res.raw, failures[i])
		}
	}
	start := decode[contest.AttemptView](t, starts[0])
	repeated := decode[contest.AttemptView](t, starts[1])
	if start.State != "active" || start.Lease != 1 || start.AttemptID != repeated.AttemptID ||
		!start.DeadlineAt.Equal(now.Add(10*time.Minute)) || !reflect.DeepEqual(start.Questions, repeated.Questions) ||
		len(start.Questions[1].Examples) != 1 || start.Questions[1].Examples[0].Expected != output {
		t.Fatalf("invalid resumed start: %#v", start)
	}
	if occupied := state(id, "start", b, 0, 0, nil); occupied.State != "occupied" || occupied.Questions != nil || occupied.Answers != nil {
		t.Fatalf("second device saw attempt content: %#v", occupied)
	}
	answers := map[int]contest.Answer{0: {Choice: &first}}
	saved := state(id, "save", a, 1, 0, answers)
	if saved.Revision != 1 || state(id, "save", a, 1, 0, answers).Revision != 1 {
		t.Fatal("save retry changed the revision")
	}
	check(id, student.Token, "save", a, 1, 0, map[int]contest.Answer{}, 409)
	next := state(id, "takeover", b, 1, 0, nil)
	if next.State != "active" || next.Lease != 2 || next.Revision != 1 || !reflect.DeepEqual(next.Questions, start.Questions) {
		t.Fatalf("takeover lost state: %#v", next)
	}
	if state(id, "save", a, 1, 1, map[int]contest.Answer{}).State != "occupied" ||
		state(id, "takeover", a, 1, 0, nil).State != "occupied" ||
		state(id, "takeover", b, 1, 0, nil).Lease != 2 {
		t.Fatal("stale lease or takeover retry changed the session")
	}
	var audits int
	if err := api.DB.QueryRow(t.Context(), "SELECT count(*) FROM assessment_attempt_takeovers WHERE attempt_id=$1", start.AttemptID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("takeover audit count = %d (%v)", audits, err)
	}
	check(id, student.Token, "save", b, 2, 1, map[int]contest.Answer{0: {Lang: "python"}}, 400)
	answers[1] = contest.Answer{Lang: "python", Source: "print(input())"}
	receipt := state(id, "submit", b, 2, 1, answers)
	duplicate := state(id, "submit", b, 2, 1, map[int]contest.Answer{})
	if receipt.FinishReason != "manual" || receipt.Revision != 2 || !contest.EqualAnswers(receipt.Answers, answers) ||
		!receipt.SubmittedAt.Equal(*duplicate.SubmittedAt) || !contest.EqualAnswers(receipt.Answers, duplicate.Answers) {
		t.Fatalf("submission was not immutable: %#v, %#v", receipt, duplicate)
	}

	for _, sweep := range []bool{false, true} {
		expiring, staffPath := create()
		publish(staffPath)
		openWindow(expiring)
		initial := state(expiring, "start", a, 0, 0, nil)
		state(expiring, "save", a, 1, 0, answers)
		if _, err := api.DB.Exec(t.Context(), `
			UPDATE assessment_attempts SET started_at=clock_timestamp()-interval '2 minutes',
				deadline_at=clock_timestamp()-interval '1 minute' WHERE id=$1
		`, initial.AttemptID); err != nil {
			t.Fatal(err)
		}
		if sweep {
			if count, err := postgres.NewContestAttemptRepo(api.Tx).Expire(t.Context(), 50); err != nil || count != 1 {
				t.Fatalf("worker expiry = %d (%v)", count, err)
			}
		}
		expired := state(expiring, "submit", a, 1, 1, map[int]contest.Answer{})
		if expired.FinishReason != "expired" || !expired.SubmittedAt.Equal(*expired.DeadlineAt) || !contest.EqualAnswers(expired.Answers, answers) {
			t.Fatalf("expiry did not freeze last acknowledged work: %#v", expired)
		}
	}

	// A write queued before the deadline must be rejected if it acquires the lease lock afterward.
	racing, racingPath := create()
	publish(racingPath)
	openWindow(racing)
	racingAttempt := state(racing, "start", a, 0, 0, nil)
	state(racing, "save", a, 1, 0, answers)
	if _, err := api.DB.Exec(t.Context(), "UPDATE assessment_attempts SET deadline_at=clock_timestamp()+interval '500 ms' WHERE id=$1", racingAttempt.AttemptID); err != nil {
		t.Fatal(err)
	}
	tx, err := api.DB.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	var racingDeadline time.Time
	if err := tx.QueryRow(t.Context(), "SELECT deadline_at FROM assessment_attempts WHERE id=$1 FOR UPDATE", racingAttempt.AttemptID).Scan(&racingDeadline); err != nil {
		t.Fatal(err)
	}
	var lateReply reply
	var lateError error
	wg.Go(func() {
		lateReply, lateError = raw(racing, student.Token, "save", a, 1, 1, map[int]contest.Answer{}, native)
	})
	if _, err := tx.Exec(t.Context(), "SELECT pg_sleep(GREATEST(0,extract(epoch FROM ($1::timestamptz-clock_timestamp()))))", racingDeadline); err != nil {
		t.Fatal(err)
	}
	if count, err := postgres.NewContestAttemptRepo(api.Tx).Expire(t.Context(), 50); err != nil || count != 0 {
		t.Fatalf("worker did not skip locked attempt: %d (%v)", count, err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if lateError != nil || lateReply.status != 200 {
		t.Fatalf("deadline race: HTTP %d %s (%v)", lateReply.status, lateReply.raw, lateError)
	}
	frozen := decode[contest.AttemptView](t, lateReply)
	if frozen.FinishReason != "expired" || !contest.EqualAnswers(frozen.Answers, answers) {
		t.Fatalf("late write changed frozen answers: %#v", frozen)
	}

	cancelled, staffPath := create()
	publish(staffPath)
	openWindow(cancelled)
	state(cancelled, "start", a, 0, 0, nil)
	requireStatus(t, api, 200, "POST", staffPath+"/cancel", staff.Token, "", map[string]any{"revision": 2, "reason": "Reschedule"})
	if stopped := state(cancelled, "submit", a, 1, 0, answers); stopped.State != "cancelled" || stopped.Answers != nil || stopped.Questions != nil {
		t.Fatalf("cancelled attempt kept accepting answers: %#v", stopped)
	}
}
