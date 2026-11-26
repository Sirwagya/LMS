package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"vedam/lms/internal/domain/learning"

	redisadapter "vedam/lms/internal/adapters/redis"
	learningapp "vedam/lms/internal/application/learning"
	"vedam/lms/internal/domain/errs"
	"vedam/lms/internal/domain/iam"
)

func (a *api) bootstrap(w http.ResponseWriter, r *http.Request) error {
	session := actor(r)
	key := redisadapter.BootstrapKey(session.UserID)
	data, err := a.Cache.Load(r.Context(), key, []string{redisadapter.CatalogKey, key}, 5*time.Minute, func(ctx context.Context) (json.RawMessage, error) {
		user, err := a.Users.GetByID(ctx, session.UserID)
		if err != nil {
			return nil, err
		}
		result, err := a.Bootstrap.Build(ctx, user)
		if err != nil {
			return nil, err
		}
		return result.MarshalJSONBytes()
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, data)
	return nil
}
func (a *api) toggleProgress(ctx context.Context, user string, id int64, complete bool) error {
	return missing(a.Learning.ToggleProgress(ctx, user, id, complete), "Lecture not found.")
}
func (a *api) progress(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	if f.err != nil {
		return f.err
	}
	id := intID(f.id("lectureId"))
	user := actor(r).UserID
	return ok(w, 200, a.mutate(r.Context(), user, func(ctx context.Context) error {
		return a.toggleProgress(ctx, user, id, f.flag("complete"))
	}))
}
func (a *api) submit(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	if f.err != nil {
		return f.err
	}
	session := actor(r)
	id := f.id("assignmentId")
	assignment, err := a.Assignments.GetPublicByID(r.Context(), id)
	if err != nil {
		return missing(err, "Assignment not found.")
	}
	if assignment.Kind == "coding" {
		return invalid("Use the coding workspace to submit this assignment.")
	}
	in := learningapp.SubmitWrittenInput{UserID: session.UserID, Role: string(session.Role), AssignmentID: id, Answer: f.text("answer", "submission answer", 20000)}
	if f.optional("file") != "" {
		in.Filename = f.text("filename", "filename", 200)
		in.FileBytes = f.bytes("file", "Maximum attachment size is 10 MB.")
	}
	if f.err != nil {
		return f.err
	}
	if len(in.FileBytes) > 0 {
		previous, err := a.Submissions.GetByUserAndAssignment(r.Context(), session.UserID, id)
		if err != nil && !errors.Is(err, errs.ErrNotFound) {
			return err
		}
		if previous != nil && previous.Score != nil && *previous.Score != "" {
			return errs.New(errs.ErrAlreadyGraded, "ALREADY_GRADED", "Graded submissions cannot be changed.")
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := a.Learning.StageWrittenAttachment(ctx, &in); err != nil {
		return err
	}
	var subID string
	err = a.mutate(ctx, session.UserID, func(ctx context.Context) error {
		if err := a.Learning.SubmitWrittenAssignment(ctx, in); err != nil {
			return err
		}
		sub, err := a.Submissions.GetByUserAndAssignment(ctx, session.UserID, id)
		if err != nil {
			return err
		}
		subID = sub.ID
		return a.changed(ctx, session.UserID, "submission", sub.ID, "sync.available", sub)
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"ok": true, "id": subID})
	return nil
}
func (a *api) submissionFile(w http.ResponseWriter, r *http.Request) error {
	sub, err := a.Submissions.GetByID(r.Context(), r.PathValue("id"))
	if err != nil {
		return missing(err, "Attachment not found.")
	}
	if sub.AssetID == nil {
		return problem(404, "Attachment not found.")
	}
	session := actor(r)
	if session.Role == iam.RoleStudent && sub.UserID != session.UserID {
		return problem(403, "Access denied.")
	}
	return a.serveAsset(w, r, *sub.AssetID, true)
}
func (a *api) discussions(w http.ResponseWriter, r *http.Request) error {
	list, err := a.Learning.ListDiscussions(r.Context(), intID(r.URL.Query().Get("lecture")))
	if err != nil {
		return err
	}
	writeJSON(w, 200, items(list))
	return nil
}
func (a *api) postDiscussion(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	if f.err != nil {
		return f.err
	}
	ctx := r.Context()
	lectureID := intID(f.id("lectureId"))
	if _, err := a.Catalog.GetLecture(ctx, lectureID); err != nil {
		return missing(err, "Lecture not found.")
	}
	message := f.text("body", "message", 2000)
	if f.err != nil {
		return f.err
	}
	var discussion learning.Discussion
	err := a.mutate(ctx, "", func(ctx context.Context) error {
		var err error
		discussion, err = a.addDiscussion(ctx, actor(r).UserID, lectureID, message)
		return err
	})
	if err != nil {
		return missing(err, "Lecture not found.")
	}
	writeJSON(w, 201, map[string]any{"ok": true, "id": discussion.ID})
	return nil
}

func (a *api) addDiscussion(ctx context.Context, user string, lectureID int64, body string) (learning.Discussion, error) {
	id, err := a.Learning.PostDiscussion(ctx, user, lectureID, body)
	if err != nil {
		return learning.Discussion{}, err
	}
	d, err := a.Discussions.GetByID(ctx, id)
	if err != nil {
		return learning.Discussion{}, err
	}
	return d, a.changed(ctx, "", "discussion", id, "sync.available", d)
}

var unsafeFilename = regexp.MustCompile(`[^a-zA-Z0-9._ -]`)

func fileHeaders(w http.ResponseWriter, name, mime string, download bool) {
	disposition := "inline"
	if download {
		disposition = "attachment"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, unsafeFilename.ReplaceAllString(name, "_")))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
}
func textFile(w http.ResponseWriter, r *http.Request, name, content string) {
	fileHeaders(w, name, "text/plain; charset=utf-8", true)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader([]byte(content)))
}
func (a *api) notes(w http.ResponseWriter, r *http.Request) error {
	l, err := a.Catalog.GetLecture(r.Context(), intID(r.PathValue("id")))
	if err != nil {
		return missing(err, "Lecture not found.")
	}
	const footer = "Sample local course notes. Replace these with instructor materials in the staff portal."
	textFile(w, r, l.Title+".txt", fmt.Sprintf("%s\n%s · %s\n\n%s\n\n%s", l.Title, l.Code, l.Date, l.Summary, footer))
	return nil
}
func (a *api) asset(w http.ResponseWriter, r *http.Request) error {
	return a.serveAsset(w, r, r.PathValue("id"), false)
}
func (a *api) serveAsset(w http.ResponseWriter, r *http.Request, id string, attachment bool) error {
	asset, err := a.AssetMeta.GetByID(r.Context(), id)
	if err != nil {
		return missing(err, "File not found.")
	}
	// Submission attachments can only be downloaded through the ownership-checked route.
	if strings.HasPrefix(asset.StorageKey, "submissions/") && !attachment {
		return problem(404, "File not found.")
	}
	file, err := a.Assets.Open(r.Context(), asset.StorageKey)
	if errors.Is(err, errs.ErrNotFound) {
		return problem(404, "File not found.")
	}
	if err != nil {
		return err
	}
	defer file.Close()
	download := attachment || (asset.MimeType != "video/mp4" && asset.MimeType != "video/webm")
	fileHeaders(w, asset.Filename, asset.MimeType, download)
	http.ServeContent(w, r, asset.Filename, asset.CreatedAt, file)
	return nil
}
