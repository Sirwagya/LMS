package contest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"vedam/lms/internal/domain/errs"
)

type Actor struct {
	ID    string
	Admin bool
}

type TestCase struct {
	Input    *string `json:"input"`
	Expected *string `json:"expected"`
	Hidden   bool    `json:"hidden"`
}

type Question struct {
	Kind          string     `json:"kind"`
	Prompt        string     `json:"prompt"`
	Points        int        `json:"points"`
	Choices       []string   `json:"choices,omitempty"`
	CorrectChoice *int       `json:"correctChoice,omitempty"`
	Explanation   string     `json:"explanation,omitempty"`
	TestCases     []TestCase `json:"testCases,omitempty"`
}

type Draft struct {
	Title           string     `json:"title"`
	Instructions    string     `json:"instructions"`
	OpensAt         time.Time  `json:"opensAt"`
	ClosesAt        time.Time  `json:"closesAt"`
	DurationMinutes int        `json:"durationMinutes"`
	StudentIDs      []string   `json:"studentIds"`
	Questions       []Question `json:"questions"`
}

// Summary is a student-safe projection. Questions and roster are staff data.
type Summary struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	OpensAt         time.Time `json:"opensAt"`
	ClosesAt        time.Time `json:"closesAt"`
	DurationMinutes int       `json:"durationMinutes"`
	Status          string    `json:"status"`
}

type Record struct {
	Draft
	ID                 string `json:"id"`
	Revision           int64  `json:"revision"`
	Status             string `json:"status"`
	CancellationReason string `json:"cancellationReason,omitempty"`
}

type Repository interface {
	ListStaff(context.Context, Actor, int, int) ([]Summary, error)
	ListStudent(context.Context, string, int, int) ([]Summary, error)
	GetStaff(context.Context, Actor, string) (Record, error)
	Create(context.Context, Actor, string, Draft) error
	Update(context.Context, Actor, string, int64, Draft) error
	Publish(context.Context, Actor, string, int64) error
	Cancel(context.Context, Actor, string, int64, string) error
}

func Invalid(message string) error { return errs.New(errs.ErrValidation, "CONTEST_INVALID", message) }

func (d *Draft) Validate() error {
	d.Title = strings.TrimSpace(d.Title)
	d.Instructions = strings.TrimSpace(d.Instructions)
	if d.Title == "" || len(d.Title) > 240 || len(d.Instructions) > 8000 {
		return Invalid("Provide a title up to 240 bytes and instructions up to 8,000 bytes.")
	}
	if d.OpensAt.IsZero() || d.ClosesAt.IsZero() || d.DurationMinutes < 1 || d.DurationMinutes > 720 ||
		d.OpensAt.Add(time.Duration(d.DurationMinutes)*time.Minute).After(d.ClosesAt) {
		return Invalid("The contest window must accommodate the attempt duration (1–720 minutes).")
	}
	if len(d.StudentIDs) > 1000 || len(d.Questions) > 50 {
		return Invalid("Use at most 1,000 students and 50 questions.")
	}
	seen := make(map[string]bool, len(d.StudentIDs))
	for _, id := range d.StudentIDs {
		if id == "" || len(id) > 128 || seen[id] {
			return Invalid("Select distinct student accounts.")
		}
		seen[id] = true
	}
	for i := range d.Questions {
		if err := d.Questions[i].validate(); err != nil {
			return Invalid(fmt.Sprintf("Question %d. %s", i+1, err))
		}
	}
	return nil
}

func (q *Question) validate() error {
	q.Prompt = strings.TrimSpace(q.Prompt)
	if q.Prompt == "" || len(q.Prompt) > 10000 || q.Points < 1 || q.Points > 1000 {
		return Invalid("Provide a prompt up to 10,000 bytes and points between 1 and 1,000.")
	}
	switch q.Kind {
	case "mcq":
		if len(q.Choices) < 2 || len(q.Choices) > 8 || q.CorrectChoice == nil ||
			*q.CorrectChoice < 0 || *q.CorrectChoice >= len(q.Choices) || len(q.TestCases) > 0 || len(q.Explanation) > 4000 {
			return Invalid("Add 2–8 choices, choose the correct answer, and keep the explanation within 4,000 bytes.")
		}
		seen := map[string]bool{}
		for i, choice := range q.Choices {
			choice = strings.TrimSpace(choice)
			if choice == "" || len(choice) > 1000 || seen[choice] {
				return Invalid("Choices must be distinct and contain 1–1,000 bytes.")
			}
			seen[choice] = true
			q.Choices[i] = choice
		}
	case "coding":
		if len(q.TestCases) < 2 || len(q.TestCases) > 10 || len(q.Choices) > 0 || q.CorrectChoice != nil || q.Explanation != "" {
			return Invalid("Add 2–10 coding test cases.")
		}
		hidden := 0
		for _, tc := range q.TestCases {
			if tc.Input == nil || tc.Expected == nil || len(*tc.Input) > 10000 || len(*tc.Expected) > 10000 {
				return Invalid("Each case needs input and expected output of at most 10,000 bytes each.")
			}
			if tc.Hidden {
				hidden++
			}
		}
		if hidden == 0 || hidden == len(q.TestCases) {
			return Invalid("Add both an example and a hidden case.")
		}
	default:
		return Invalid("Choose MCQ or coding.")
	}
	return nil
}
