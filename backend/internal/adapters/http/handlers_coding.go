package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"vedam/lms/internal/adapters/grader"
	redisadapter "vedam/lms/internal/adapters/redis"
	"vedam/lms/internal/domain/assessment"
	"vedam/lms/internal/domain/errs"
)

type CodingService interface {
	Run(context.Context, redisadapter.GradingJob, *string, bool) (grader.Result, error)
	History(context.Context, string, string) ([]assessment.CodingAttempt, error)
}

var ErrEvaluationBusy = errors.New("Wait for your current execution to finish.")
var ErrRunnerBusy = errors.New("The local runner is busy. Please try again shortly.")

func (a *api) evaluate(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	if f.err != nil {
		return f.err
	}
	id := f.id("assignmentId")
	assignment, err := a.Assignments.GetPublicByID(r.Context(), id)
	if err != nil {
		return missing(err, "Coding assignment not found.")
	}
	if assignment.Kind != assessment.KindCoding {
		return problem(404, "Coding assignment not found.")
	}
	source := f.text("source", "source code", 20000)
	if f.err != nil {
		return f.err
	}
	language, action := f.optional("language"), f.optional("action")
	if language != "python" && language != "java" {
		return invalid("Choose Java or Python.")
	}
	if action != "run" && action != "submit" {
		return invalid("Choose Run or Submit.")
	}
	var custom *string
	if value, exists := f.values["customInput"]; exists && action == "run" {
		if json.Unmarshal(value, &custom) != nil || custom == nil || textLength(*custom) > 10000 {
			return invalid("Custom input must be under 10000 characters.")
		}
	}
	job := redisadapter.GradingJob{ID: uuid.NewString(), UserID: actor(r).UserID, AssignmentID: id, Language: language, Source: source}
	if action == "submit" && strings.HasPrefix(r.URL.Path, "/api/v1/") && r.Header.Get("Prefer") == "respond-async" {
		err := a.Queue.Enqueue(r.Context(), job)
		if errors.Is(err, redisadapter.ErrQueueFull) || errors.Is(err, redisadapter.ErrJobPending) {
			return problem(429, err.Error())
		}
		if err != nil {
			return err
		}
		w.Header().Set("Location", "/api/v1/coding/jobs/"+job.ID)
		w.Header().Set("Preference-Applied", "respond-async")
		writeJSON(w, 202, map[string]string{"id": job.ID, "status": "queued"})
		return nil
	}
	result, err := a.Coding.Run(r.Context(), job, custom, action == "submit")
	var unavailable grader.Unavailable
	switch {
	case errors.Is(err, ErrEvaluationBusy), errors.Is(err, ErrRunnerBusy):
		return problem(429, err.Error())
	case errors.Is(err, errs.ErrValidation):
		return invalid(err.Error())
	case errors.As(err, &unavailable):
		return problem(503, unavailable.Error())
	case err != nil:
		return err
	}
	writeJSON(w, 200, result)
	return nil
}
func (a *api) history(w http.ResponseWriter, r *http.Request) error {
	list, err := a.Coding.History(r.Context(), actor(r).UserID, r.URL.Query().Get("assignmentId"))
	if err != nil {
		return missing(err, "Assignment not found.")
	}
	writeJSON(w, 200, items(list))
	return nil
}
func (a *api) gradingJob(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		return problem(404, "Grading job not found.")
	}
	state, err := a.Queue.Status(r.Context(), actor(r).UserID, id)
	if errors.Is(err, redis.Nil) {
		return problem(404, "Grading job not found.")
	}
	if err != nil {
		return err
	}
	writeJSON(w, 200, state)
	return nil
}
