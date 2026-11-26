//go:build integration

package runtime

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"

	"vedam/lms/internal/domain/contest"
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
