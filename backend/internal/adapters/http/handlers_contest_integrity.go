package httpadapter

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"vedam/lms/internal/domain/contest"
	"vedam/lms/internal/domain/errs"
)

func contestSubID(r *http.Request, key, msg string) (string, error) {
	id, err := uuid.Parse(r.PathValue(key))
	if err != nil {
		return "", problem(404, msg)
	}
	return id.String(), nil
}

func decodeContestJSON(w http.ResponseWriter, r *http.Request, limit int64, dst any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			return problem(413, "Request body is too large.")
		}
		return invalid("Invalid JSON payload.")
	}
	var extra any
	if d.Decode(&extra) == nil {
		return invalid("Invalid JSON payload.")
	}
	return nil
}

func (a *api) contestIngestEvents(w http.ResponseWriter, r *http.Request) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Authorization")), "bearer ") ||
		r.Header.Get("X-LMS-Contest-Client") != "macos" ||
		!nativeRequest(r) {
		return problem(403, "Open this contest in the LMS macOS app.")
	}
	id, err := contestID(r)
	if err != nil {
		return err
	}
	var batch contest.EventBatchInput
	if err := decodeContestJSON(w, r, 256<<10, &batch); err != nil {
		return err
	}
	result, err := a.ContestIntegrity.IngestEvents(r.Context(), id, actor(r).UserID, batch)
	if err != nil {
		return missing(err, "Contest attempt not found.")
	}
	writeJSON(w, 200, result)
	return nil
}

func (a *api) contestListAppeals(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	status, err := a.ContestIntegrity.ListStudentAppeals(r.Context(), id, actor(r).UserID)
	if err != nil {
		return missing(err, "Contest attempt not found.")
	}
	writeJSON(w, 200, status)
	return nil
}

func (a *api) contestSubmitAppeal(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	var cmd contest.AppealInput
	if err := decodeContestJSON(w, r, 32<<10, &cmd); err != nil {
		return err
	}
	appeal, err := a.ContestIntegrity.SubmitAppeal(r.Context(), id, actor(r).UserID, cmd)
	if err != nil {
		return missing(err, "Contest attempt not found.")
	}
	writeJSON(w, 201, appeal)
	return nil
}

func (a *api) contestReviewQueue(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	view, err := a.ContestIntegrity.ListReviewQueue(r.Context(), contestActor(r), id)
	if err != nil {
		return missing(err, "Contest not found.")
	}
	writeJSON(w, 200, view)
	return nil
}

func (a *api) contestAttemptReplay(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	attemptID, err := contestSubID(r, "attemptId", "Attempt not found.")
	if err != nil {
		return err
	}
	replay, err := a.ContestIntegrity.GetAttemptReplay(r.Context(), contestActor(r), id, attemptID)
	if err != nil {
		return missing(err, "Attempt not found.")
	}
	writeJSON(w, 200, replay)
	return nil
}

func (a *api) contestAssignReviewer(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	var body struct {
		ReviewerID string `json:"reviewerId"`
	}
	if err := decodeContestJSON(w, r, 4096, &body); err != nil {
		return err
	}
	if strings.TrimSpace(body.ReviewerID) == "" {
		return invalid("Provide a reviewer user ID.")
	}
	if err := a.ContestIntegrity.AssignReviewer(r.Context(), contestActor(r), id, strings.TrimSpace(body.ReviewerID)); err != nil {
		return missing(err, "Contest or reviewer not found.")
	}
	writeJSON(w, 200, map[string]bool{"assigned": true})
	return nil
}

func (a *api) contestDecideReview(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	attemptID, err := contestSubID(r, "attemptId", "Attempt not found.")
	if err != nil {
		return err
	}
	var cmd contest.ReviewDecisionInput
	if err := decodeContestJSON(w, r, 16<<10, &cmd); err != nil {
		return err
	}
	summary, err := a.ContestIntegrity.DecideReview(r.Context(), contestActor(r), id, attemptID, cmd)
	if err != nil {
		return missing(err, "Attempt not found.")
	}
	writeJSON(w, 200, summary)
	return nil
}

func (a *api) contestResolveAppeal(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	appealID, err := contestSubID(r, "appealId", "Appeal not found.")
	if err != nil {
		return err
	}
	var cmd contest.ResolveAppealInput
	if err := decodeContestJSON(w, r, 16<<10, &cmd); err != nil {
		return err
	}
	appeal, err := a.ContestIntegrity.ResolveAppeal(r.Context(), contestActor(r), id, appealID, cmd)
	if err != nil {
		return missing(err, "Appeal not found.")
	}
	writeJSON(w, 200, appeal)
	return nil
}

func (a *api) contestComputeSimilarity(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	pairs, err := a.ContestIntegrity.ComputeSimilarity(r.Context(), contestActor(r), id)
	if err != nil {
		return missing(err, "Contest not found.")
	}
	writeJSON(w, 200, map[string]any{"pairs": pairs})
	return nil
}

func (a *api) contestUploadProctorMedia(w http.ResponseWriter, r *http.Request) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Authorization")), "bearer ") ||
		r.Header.Get("X-LMS-Contest-Client") != "macos" ||
		!nativeRequest(r) {
		return problem(403, "Open this contest in the LMS macOS app.")
	}
	if a.Assets == nil {
		return problem(503, "Media storage unavailable.")
	}
	id, err := contestID(r)
	if err != nil {
		return err
	}
	var body struct {
		AttemptID   string    `json:"attemptId"`
		DeviceID    string    `json:"deviceId"`
		Kind        string    `json:"kind"`
		Seq         int       `json:"seq"`
		ContentType string    `json:"contentType"`
		DurationMS  int       `json:"durationMs"`
		CapturedAt  time.Time `json:"capturedAt"`
		DataBase64  string    `json:"dataBase64"`
	}
	if err := decodeContestJSON(w, r, 7_500_000, &body); err != nil {
		return err
	}
	rawBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(body.DataBase64))
	if err != nil {
		return invalid("Provide valid base64-encoded media data.")
	}
	if len(rawBytes) == 0 {
		return invalid("Media payload cannot be empty.")
	}
	if len(rawBytes) > contest.MaxProctorMediaBytes {
		return problem(413, "Media chunk exceeds 5 MB limit.")
	}

	in := contest.ProctorMediaInput{
		AttemptID:   body.AttemptID,
		DeviceID:    body.DeviceID,
		Kind:        body.Kind,
		Seq:         body.Seq,
		ContentType: body.ContentType,
		DurationMS:  body.DurationMS,
		CapturedAt:  body.CapturedAt,
		ByteSize:    len(rawBytes),
	}
	ext, err := contest.ValidateProctorMediaInput(&in)
	if err != nil {
		return err
	}
	attemptUUID, _ := uuid.Parse(strings.TrimSpace(in.AttemptID))
	key := filepath.Join("contests", id, attemptUUID.String(), fmt.Sprintf("%s-%d-%s.%s", in.Kind, in.Seq, uuid.NewString(), ext))
	if !filepath.IsLocal(key) {
		return invalid("Invalid media storage key.")
	}
	in.StorageKey = key

	if err := a.Assets.Put(r.Context(), key, rawBytes); err != nil {
		return err
	}
	item, err := a.ContestIntegrity.RecordProctorMedia(r.Context(), id, actor(r).UserID, in)
	if err != nil {
		_ = a.Assets.Remove(key)
		return missing(err, "Contest attempt not found.")
	}
	writeJSON(w, 201, item)
	return nil
}

func (a *api) contestListProctorMedia(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	attemptID, err := contestSubID(r, "attemptId", "Attempt not found.")
	if err != nil {
		return err
	}
	list, err := a.ContestIntegrity.ListProctorMedia(r.Context(), contestActor(r), id, attemptID)
	if err != nil {
		return missing(err, "Attempt not found.")
	}
	writeJSON(w, 200, map[string]any{"items": items(list)})
	return nil
}

func (a *api) contestGetProctorMedia(w http.ResponseWriter, r *http.Request) error {
	if a.Assets == nil {
		return problem(503, "Media storage unavailable.")
	}
	id, err := contestID(r)
	if err != nil {
		return err
	}
	mediaID, err := contestSubID(r, "mediaId", "Proctoring media not found.")
	if err != nil {
		return err
	}
	item, err := a.ContestIntegrity.GetProctorMedia(r.Context(), contestActor(r), id, mediaID)
	if err != nil {
		return missing(err, "Proctoring media not found.")
	}
	f, err := a.Assets.Open(r.Context(), item.StorageKey)
	if errors.Is(err, errs.ErrNotFound) {
		return problem(404, "Proctoring media not found.")
	}
	if err != nil {
		return err
	}
	defer f.Close()

	filename := filepath.Base(item.StorageKey)
	fileHeaders(w, filename, item.ContentType, false)
	http.ServeContent(w, r, filename, item.CreatedAt, f)
	return nil
}
