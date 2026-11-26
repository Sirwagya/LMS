package httpadapter

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"vedam/lms/internal/domain/contest"
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
