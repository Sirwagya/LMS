package httpadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"vedam/lms/internal/domain/assessment"
	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/errs"
	"vedam/lms/internal/domain/finance"
	"vedam/lms/internal/domain/iam"
)

func (a *api) staffWorkspace(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	users, err := a.Users.ListAll(ctx)
	if err != nil {
		return err
	}
	public := make([]iam.PublicUser, 0, len(users))
	for _, u := range users {
		if actor(r).Role == iam.RoleAdmin || u.Role == iam.RoleStudent {
			data := u.Public()
			data.Profile = nil
			public = append(public, data)
		}
	}
	subs, err := a.Submissions.ListAll(ctx)
	if err != nil {
		return err
	}
	attendance, err := a.Attendance.ListAll(ctx)
	if err != nil {
		return err
	}
	var fees []finance.FeeInvoice
	if actor(r).Role == iam.RoleAdmin {
		fees, err = a.Invoices.ListAll(ctx)
		if err != nil {
			return err
		}
	}
	writeJSON(w, 200, map[string]any{"users": public, "submissions": items(subs), "attendance": items(attendance), "fees": items(fees)})
	return nil
}
func (a *api) createUser(w http.ResponseWriter, r *http.Request) error {
	f := readBody(w, r, 4096)
	name, email, password := f.text("name", "name", 100), strings.ToLower(f.text("email", "email", 254)), f.text("password", "password", 128)
	role := iam.Role(f.optional("role"))
	if f.err != nil {
		return f.err
	}
	if (role != iam.RoleStudent && role != iam.RoleTeacher) || !emailPattern.MatchString(email) || textLength(password) < 10 {
		return invalid("Valid email, role and a 10-character password required.")
	}
	release, err := a.passwordSlot(w)
	if err != nil {
		return err
	}
	defer release()
	_, err = a.newUser(r.Context(), name, email, password, role)
	return ok(w, 201, err)
}
func (a *api) createInvoice(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	userID := f.id("userId")
	if f.err != nil {
		return f.err
	}
	if _, err := a.Users.FindByRole(r.Context(), userID, iam.RoleStudent); err != nil {
		return missing(err, "Student not found.")
	}
	amount, due := f.integer("amount", "amount", 1, 10000000), f.date("dueDate", "Use a valid date.")
	desc, semester := f.text("description", "description", 200), f.integer("semester", "semester", 1, 3)
	if f.err != nil {
		return f.err
	}
	now := time.Now()
	status := finance.StatusPending
	if due < now.Format(time.DateOnly) {
		status = finance.StatusOverdue
	}
	id := uuid.NewString()
	inv := &finance.FeeInvoice{ID: id, UserID: userID, InvoiceNo: "LOCAL-" + id, Description: desc, Semester: fmt.Sprintf("Semester %d", semester), Amount: int64(amount), DueDate: due, Status: status, Simulated: true}
	err := a.mutate(r.Context(), userID, func(ctx context.Context) error {
		if err := a.Invoices.Create(ctx, inv); err != nil {
			return err
		}
		return a.changed(ctx, userID, "fee_invoice", id, "sync.available", inv)
	})
	return ok(w, 201, err)
}
func (a *api) attendance(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	user, course := f.id("userId"), f.id("course")
	if f.err != nil {
		return f.err
	}
	if _, err := a.Users.FindByRole(r.Context(), user, iam.RoleStudent); err != nil {
		return missing(err, "Student not found.")
	}
	if _, err := a.Catalog.GetCourse(r.Context(), course); err != nil {
		return missing(err, "Course not found.")
	}
	total, attended := f.integer("total", "total classes", 0, 1000), 0
	if f.err == nil {
		attended = f.integer("attended", "attended classes", 0, total)
	}
	if f.err != nil {
		return f.err
	}
	err := a.mutate(r.Context(), user, func(ctx context.Context) error {
		if err := a.Staff.UpdateAttendance(ctx, actor(r).Role, user, course, attended, total); err != nil {
			return err
		}
		return a.changed(ctx, user, "attendance", course, "sync.available", map[string]any{"course": course, "attended": attended, "total": total})
	})
	return ok(w, 200, err)
}
func (a *api) grade(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	if f.err != nil {
		return f.err
	}
	sub, err := a.Submissions.GetByID(r.Context(), f.id("id"))
	if err != nil {
		return missing(err, "Submission not found.")
	}
	score, feedback := f.text("score", "score", 30), f.optional("feedback")
	if f.err != nil {
		return f.err
	}
	err = a.mutate(r.Context(), sub.UserID, func(ctx context.Context) error {
		if err := a.Staff.GradeSubmission(ctx, actor(r).Role, sub.ID, score, feedback); err != nil {
			return err
		}
		updated, err := a.Submissions.GetByID(ctx, sub.ID)
		if err != nil {
			return err
		}
		return a.changed(ctx, sub.UserID, "submission", sub.ID, "grade.updated", updated)
	})
	return ok(w, 200, err)
}
func (a *api) schedule(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	if f.err != nil {
		return f.err
	}
	course := f.id("course")
	if _, err := a.Catalog.GetCourse(r.Context(), course); err != nil {
		return missing(err, "Course not found.")
	}
	date, from, to := f.text("date", "date", 10), f.text("from", "start time", 5), f.text("to", "end time", 5)
	room, topic := f.text("room", "room", 150), f.text("topic", "topic", 200)
	if f.err != nil {
		return f.err
	}
	err := a.mutate(r.Context(), "", func(ctx context.Context) error {
		if err := a.Staff.AddScheduleSlot(ctx, actor(r).Role, date, from, to, course, room, topic); err != nil {
			return err
		}
		return a.changed(ctx, "", "schedule", date+"T"+from, "sync.available", map[string]any{"date": date, "from": from, "to": to, "course": course, "room": room, "topic": topic})
	})
	return ok(w, 201, err)
}
func (a *api) editCatalog(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	kind, id := f.text("kind", "record type", 30), f.id("id")
	if f.err != nil {
		return f.err
	}
	var apply func(context.Context) error
	var missingMessage string
	var title, desc string
	switch kind {
	case "course":
		title, desc = f.text("title", "name", 200), f.text("description", "instructor", 100)
		apply = func(ctx context.Context) error { return a.Catalog.UpdateCourseMeta(ctx, id, title, desc) }
		missingMessage = "Course not found."
	case "lecture":
		title, desc = f.text("title", "title", 200), f.text("description", "notes", 20000)
		apply = func(ctx context.Context) error { return a.Catalog.UpdateLectureMeta(ctx, intID(id), title, desc) }
		missingMessage = "Lecture not found."
	case "notice":
		title, desc = f.text("title", "title", 200), f.text("description", "message", 5000)
		apply = func(ctx context.Context) error { return a.Notices.UpdateNotice(ctx, id, title, desc) }
		missingMessage = "Notice not found."
	default:
		return invalid("Unsupported record type.")
	}
	if f.err != nil {
		return f.err
	}
	err := a.mutate(r.Context(), "", func(ctx context.Context) error {
		if err := apply(ctx); err != nil {
			return missing(err, missingMessage)
		}
		return a.changed(ctx, "", kind, id, "sync.available", map[string]string{"id": id, "title": title, "description": desc})
	})
	return ok(w, 200, err)
}
func (a *api) createCourse(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	semester, code := f.integer("semester", "semester", 1, 3), f.text("code", "course identifier", 30)
	name, instructor, credits := f.text("name", "course name", 200), f.text("instructor", "instructor", 100), f.integer("credits", "credits", 1, 10)
	if f.err != nil {
		return f.err
	}
	err := a.mutate(r.Context(), "", func(ctx context.Context) error {
		if err := a.Staff.CreateCourse(ctx, actor(r).Role, semester, code, name, instructor, credits); err != nil {
			if errors.Is(err, errs.ErrConflict) {
				return problem(409, "Course identifier already exists.")
			}
			return err
		}
		value, err := a.Catalog.GetCourse(ctx, code)
		if err != nil {
			return err
		}
		return a.changed(ctx, "", "course", code, "sync.available", value)
	})
	return ok(w, 201, err)
}
func (a *api) createLecture(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	if f.err != nil {
		return f.err
	}
	course := f.id("course")
	if _, err := a.Catalog.GetCourse(r.Context(), course); err != nil {
		return missing(err, "Course not found.")
	}
	title, summary := f.text("title", "title", 200), f.text("summary", "lecture notes", 20000)
	if f.err != nil {
		return f.err
	}
	lecture := &catalog.Lecture{CourseCode: course, Date: time.Now().UTC().Format(time.DateOnly), Unit: 1, Title: title, RawTitle: f.optional("title"), Summary: summary, Attendance: "Awaiting", Resources: []catalog.Resource{}, Assignments: []catalog.LectureAssignmentRef{}}
	err := a.mutate(r.Context(), "", func(ctx context.Context) error {
		id, err := a.Catalog.CreateLecture(ctx, lecture)
		if err != nil {
			return err
		}
		lecture.ID = id
		return a.changed(ctx, "", "lecture", strconv.FormatInt(id, 10), "sync.available", lecture)
	})
	if err != nil {
		return err
	}
	writeJSON(w, 201, map[string]int64{"id": lecture.ID})
	return nil
}
func (a *api) createAssignment(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	if f.err != nil {
		return f.err
	}
	lecture, err := a.Catalog.GetLecture(r.Context(), intID(f.id("lectureId")))
	if err != nil {
		return missing(err, "Lecture not found.")
	}
	due, title := f.date("dueDate", "Use a valid date."), f.text("title", "assignment title", 250)
	if f.err != nil {
		return f.err
	}
	id := uuid.NewString()
	err = a.mutate(r.Context(), "", func(ctx context.Context) error {
		if err := a.Assignments.CreateWritten(ctx, id, lecture.ID, lecture.CourseCode, title, due); err != nil {
			return err
		}
		return a.assignmentChanged(ctx, id, lecture.ID)
	})
	return ok(w, 201, err)
}

func (a *api) assignmentChanged(ctx context.Context, id string, lectureID int64) error {
	assignment, err := a.Assignments.GetPublicByID(ctx, id)
	if err != nil {
		return err
	}
	return a.changed(ctx, "", "assignment", id, "sync.available", struct {
		*assessment.PublicAssignment
		LectureID int64 `json:"lectureId"`
	}{assignment, lectureID})
}
func (a *api) createCoding(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	if f.err != nil {
		return f.err
	}
	lecture, err := a.Catalog.GetLecture(r.Context(), intID(f.id("lectureId")))
	if err != nil {
		return missing(err, "Lecture not found.")
	}
	title := f.text("title", "title", 250)
	spec := assessment.CodingProblemSpec{
		Description: f.text("description", "problem statement", 10000), InputFormat: f.text("inputFormat", "input format", 2000),
		OutputFormat: f.text("outputFormat", "output format", 2000), Constraints: f.text("constraints", "constraints", 2000),
		Difficulty: f.optional("difficulty"), Entrypoint: "solve",
		StarterCode: map[string]string{
			"python": "class Solution:\n    def solve(self, input: str) -> str:\n        # Return the answer as a string.\n        pass\n",
			"java":   "class Solution {\n    public String solve(String input) {\n        return \"\";\n    }\n}\n",
		},
	}
	due := f.date("dueDate", "Use a valid due date.")
	if f.err != nil {
		return f.err
	}
	if spec.Difficulty != "Medium" && spec.Difficulty != "Hard" {
		spec.Difficulty = "Easy"
	}
	var cases []struct {
		Input, Expected *string
		Hidden          bool
	}
	if json.Unmarshal(f.values["testCases"], &cases) != nil || len(cases) < 2 || len(cases) > 10 {
		return invalid("Add 2–10 test cases with at least one example and one hidden test.")
	}
	hidden := 0
	for _, tc := range cases {
		if tc.Input == nil || tc.Expected == nil || textLength(*tc.Input) > 10000 || textLength(*tc.Expected) > 10000 {
			return invalid("Test input/output must be strings under 10000 characters.")
		}
		if tc.Hidden {
			hidden++
		}
		spec.TestCases = append(spec.TestCases, assessment.TestCase{Input: *tc.Input, Expected: *tc.Expected, Hidden: tc.Hidden})
	}
	if hidden == 0 || hidden == len(cases) {
		return invalid("Include both example and hidden test cases.")
	}
	id := uuid.NewString()
	err = a.mutate(r.Context(), "", func(ctx context.Context) error {
		if err := a.Assignments.CreateCoding(ctx, id, lecture.ID, lecture.CourseCode, title, due, spec); err != nil {
			return err
		}
		return a.assignmentChanged(ctx, id, lecture.ID)
	})
	return ok(w, 201, err)
}
func (a *api) createNotice(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	title, desc := f.text("title", "title", 200), f.text("desc", "notice", 5000)
	if f.err != nil {
		return f.err
	}
	tag := "ACADEMIC"
	if f.optional("tag") == "URGENT" {
		tag = "URGENT"
	}
	now := time.Now()
	notice := &catalog.Notice{ID: uuid.NewString(), Title: title, Desc: desc, Tag: tag, Placement: "drawer", Time: now.Format("02/01/2006, 15:04:05"), CreatedAt: now}
	err := a.mutate(r.Context(), "", func(ctx context.Context) error {
		if err := a.Notices.CreateNotice(ctx, notice); err != nil {
			return err
		}
		return a.changed(ctx, "", "notice", notice.ID, "notice.published", notice)
	})
	return ok(w, 201, err)
}
func (a *api) uploadAsset(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	if f.err != nil {
		return f.err
	}
	lecture, err := a.Catalog.GetLecture(r.Context(), intID(f.id("lectureId")))
	if err != nil {
		return missing(err, "Lecture not found.")
	}
	name := f.text("name", "filename", 200)
	f.text("bytes", "file contents", 15<<20)
	data := f.bytes("bytes", "Maximum file size is 10 MB.")
	if f.err != nil {
		return f.err
	}
	mime := "application/octet-stream"
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4":
		mime = "video/mp4"
	case ".webm":
		mime = "video/webm"
	}
	recording := f.flag("recording")
	if recording && mime == "application/octet-stream" {
		return invalid("Use an MP4 or WebM video.")
	}
	id := uuid.NewString()
	key := "lecture-assets/" + id
	if err := a.Assets.Put(r.Context(), key, data); err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	asset := &catalog.Asset{ID: id, Filename: name, MimeType: mime, ByteSize: int64(len(data)), StorageKey: key, SHA256: hex.EncodeToString(digest[:]), CreatedAt: time.Now()}
	url := "/api/assets/" + id
	err = a.mutate(r.Context(), "", func(ctx context.Context) error {
		if err := a.AssetMeta.Create(ctx, asset); err != nil {
			return err
		}
		if recording {
			if err := a.Catalog.SetLectureRecording(ctx, lecture.ID, url, id); err != nil {
				return err
			}
		} else {
			res := catalog.Resource{ID: id, Title: name, Type: "File", Size: fmt.Sprintf("%d KB", (len(data)+1023)/1024), URL: &url}
			if err := a.Catalog.AddLectureResource(ctx, lecture.ID, res, &id); err != nil {
				return err
			}
		}
		return a.changed(ctx, "", "asset", id, "sync.available", map[string]any{"id": id, "lectureId": lecture.ID, "url": url, "recording": recording, "name": name})
	})
	if err != nil {
		// Keep the blob on an ambiguous commit result. The metadata may already exist.
		check, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), time.Second)
		defer cancel()
		if _, lookupErr := a.AssetMeta.GetByID(check, id); errors.Is(lookupErr, errs.ErrNotFound) {
			_ = a.Assets.Remove(key)
		}
	}
	return ok(w, 201, err)
}
