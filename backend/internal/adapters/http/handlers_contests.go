package httpadapter

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"vedam/lms/internal/domain/contest"
	"vedam/lms/internal/domain/iam"
)

func contestActor(r *http.Request) contest.Actor {
	session := actor(r)
	return contest.Actor{ID: session.UserID, Admin: session.Role == iam.RoleAdmin}
}

func contestID(r *http.Request) (string, error) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return "", problem(404, "Contest not found.")
	}
	return id.String(), nil
}

func (a *api) contestStatus(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, 200, map[string]bool{"enabled": a.Contests != nil, "attemptsEnabled": a.ContestAttempts != nil})
	return nil
}

func (a *api) contestList(w http.ResponseWriter, r *http.Request) error {
	page, err := queryNumber(r, "page", 0, 10000, 0)
	if err != nil {
		return err
	}
	const pageSize = 25
	viewer := contestActor(r)
	var list []contest.Summary
	if actor(r).Role == iam.RoleStudent {
		list, err = a.ContestRepo.ListStudent(r.Context(), viewer.ID, pageSize+1, int(page)*pageSize)
	} else {
		list, err = a.ContestRepo.ListStaff(r.Context(), viewer, pageSize+1, int(page)*pageSize)
	}
	if err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"items": list[:min(len(list), pageSize)], "hasMore": len(list) > pageSize, "page": page})
	return nil
}

func (a *api) contestDetail(w http.ResponseWriter, r *http.Request) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	record, err := a.ContestRepo.GetStaff(r.Context(), contestActor(r), id)
	if err != nil {
		return missing(err, "Contest not found.")
	}
	writeJSON(w, 200, record)
	return nil
}

func (a *api) contestSave(w http.ResponseWriter, r *http.Request) error {
	var id string
	var err error
	if r.Method == http.MethodPut {
		id, err = contestID(r)
		if err != nil {
			return err
		}
	}
	f := readBody(w, r, 4<<20)
	if f.err != nil {
		return f.err
	}
	var d contest.Draft
	if json.Unmarshal(f.values["draft"], &d) != nil {
		return invalid("Provide a contest draft with valid dates and questions.")
	}
	var revision int64
	if id != "" && (json.Unmarshal(f.values["revision"], &revision) != nil || revision < 1) {
		return invalid("Provide the current draft revision.")
	}
	record, err := a.Contests.Save(r.Context(), contestActor(r), id, revision, d)
	if err != nil {
		return missing(err, "Contest not found.")
	}
	status := 200
	if id == "" {
		status = 201
	}
	writeJSON(w, status, record)
	return nil
}

func (a *api) contestPublish(w http.ResponseWriter, r *http.Request) error {
	return a.transitionContest(w, r, false)
}

func (a *api) contestCancel(w http.ResponseWriter, r *http.Request) error {
	return a.transitionContest(w, r, true)
}

func (a *api) transitionContest(w http.ResponseWriter, r *http.Request, cancel bool) error {
	id, err := contestID(r)
	if err != nil {
		return err
	}
	f := readBody(w, r, 4096)
	if f.err != nil {
		return f.err
	}
	var revision int64
	if json.Unmarshal(f.values["revision"], &revision) != nil || revision < 1 {
		return invalid("Provide the current contest revision.")
	}
	if cancel {
		err = a.Contests.Cancel(r.Context(), contestActor(r), id, revision, f.optional("reason"))
	} else {
		err = a.Contests.Publish(r.Context(), contestActor(r), id, revision)
	}
	return ok(w, 200, missing(err, "Contest not found."))
}
