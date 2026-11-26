package httpadapter

import (
	"net/http"
	"strings"

	"vedam/lms/internal/domain/contest"
	"vedam/lms/internal/domain/iam"
)

func (a *api) contestStudentResultsSummary(w http.ResponseWriter, r *http.Request) error {
	items, err := a.ContestResults.ListStudentResultsSummary(r.Context(), actor(r).UserID)
	if err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"items": items})
	return nil
}

func (a *api) contestSetAlias(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	var body struct {
		Alias string `json:"alias"`
	}
	if err := decodeContestJSON(w, r, 4096, &body); err != nil {
		return err
	}
	if err := a.ContestResults.SetAlias(r.Context(), id, actor(r).UserID, strings.TrimSpace(body.Alias)); err != nil {
		return missing(err, "Contest not found.")
	}
	writeJSON(w, 200, map[string]any{"alias": strings.TrimSpace(body.Alias)})
	return nil
}

func (a *api) contestStudentReport(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	report, err := a.ContestResults.GetStudentReport(r.Context(), id, actor(r).UserID)
	if err != nil {
		return missing(err, "Contest results are not published.")
	}
	writeJSON(w, 200, report)
	return nil
}

func (a *api) contestLeaderboard(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	page, err := queryNumber(r, "page", 0, 10000, 0)
	if err != nil {
		return err
	}
	session := actor(r)
	isStaff := session.Role == iam.RoleTeacher || session.Role == iam.RoleAdmin
	board, err := a.ContestResults.GetLeaderboard(r.Context(), id, session.UserID, isStaff, int(page))
	if err != nil {
		return missing(err, "Contest leaderboard is not available.")
	}
	writeJSON(w, 200, board)
	return nil
}

func (a *api) contestStaffResults(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	page, err := queryNumber(r, "page", 0, 10000, 0)
	if err != nil {
		return err
	}
	view, err := a.ContestResults.GetStaffResults(r.Context(), contestActor(r), id, int(page))
	if err != nil {
		return missing(err, "Contest not found.")
	}
	writeJSON(w, 200, view)
	return nil
}

func (a *api) contestPublishResults(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	var cmd contest.PublishResultsInput
	if err := decodeContestJSON(w, r, 64<<10, &cmd); err != nil {
		return err
	}
	rel, err := a.ContestResults.PublishOrRegrade(r.Context(), contestActor(r), id, cmd)
	if err != nil {
		return missing(err, "Contest not found.")
	}
	writeJSON(w, 200, rel)
	return nil
}
