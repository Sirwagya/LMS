package contest

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"vedam/lms/internal/domain/errs"
)

type Answer struct {
	Choice *int   `json:"choice,omitempty"`
	Source string `json:"source,omitempty"`
	Lang   string `json:"language,omitempty"`
}

type AttemptCommand struct {
	Action   string
	DeviceID string
	Lease    int64
	Revision int64
	Answers  map[int]Answer
}

type Example struct {
	Input    string `json:"input"`
	Expected string `json:"expected"`
}

type StudentQuestion struct {
	Kind     string    `json:"kind"`
	Prompt   string    `json:"prompt"`
	Points   int       `json:"points"`
	Choices  []string  `json:"choices,omitempty"`
	Examples []Example `json:"examples,omitempty"`
}

type AttemptView struct {
	Summary
	Instructions string            `json:"instructions"`
	State        string            `json:"state"`
	ServerTime   time.Time         `json:"serverTime"`
	AttemptID    string            `json:"attemptId,omitempty"`
	StartedAt    *time.Time        `json:"startedAt,omitempty"`
	DeadlineAt   *time.Time        `json:"deadlineAt,omitempty"`
	SubmittedAt  *time.Time        `json:"submittedAt,omitempty"`
	FinishReason string            `json:"finishReason,omitempty"`
	Lease        int64             `json:"lease"`
	Revision     int64             `json:"revision"`
	Answers      map[int]Answer    `json:"answers"`
	Questions    []StudentQuestion `json:"questions,omitempty"`
}

type AttemptRepository interface {
	Execute(context.Context, string, string, AttemptCommand) (AttemptView, error)
}

func AttemptConflict() error {
	return errs.New(errs.ErrConflict, "ATTEMPT_CHANGED", "Your saved answers changed. Reopen the contest to sync them.")
}

// ChoiceOrder maps displayed positions to the corresponding published choice indexes.
func ChoiceOrder(attemptID string, questionIndex, size int) []int {
	type choice struct {
		index int
		rank  string
	}
	ranked := make([]choice, size)
	for i := range ranked {
		digest := hmac.New(sha256.New, []byte(attemptID))
		fmt.Fprintf(digest, "contest-mcq/v1/%d/%d", questionIndex, i)
		ranked[i] = choice{i, string(digest.Sum(nil))}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].rank < ranked[j].rank })
	order := make([]int, size)
	for i, item := range ranked {
		order[i] = item.index
	}
	return order
}

func StudentQuestions(attemptID string, questions []Question) []StudentQuestion {
	out := make([]StudentQuestion, len(questions))
	for index, q := range questions {
		item := StudentQuestion{Kind: q.Kind, Prompt: q.Prompt, Points: q.Points}
		for _, choice := range ChoiceOrder(attemptID, index, len(q.Choices)) {
			item.Choices = append(item.Choices, q.Choices[choice])
		}
		for _, tc := range q.TestCases {
			if !tc.Hidden {
				item.Examples = append(item.Examples, Example{*tc.Input, *tc.Expected})
			}
		}
		out[index] = item
	}
	return out
}

func ValidateAnswers(questions []Question, answers map[int]Answer) error {
	if len(answers) > len(questions) {
		return Invalid("Provide answers only for questions in this contest.")
	}
	total := 0
	for index, answer := range answers {
		if index < 0 || index >= len(questions) {
			return Invalid("Provide answers only for questions in this contest.")
		}
		q := questions[index]
		switch q.Kind {
		case "mcq":
			if answer.Choice == nil || *answer.Choice < 0 || *answer.Choice >= len(q.Choices) || answer.Lang != "" || answer.Source != "" {
				return Invalid("Select a valid choice for each answered MCQ.")
			}
		case "coding":
			if answer.Choice != nil || (answer.Lang != "python" && answer.Lang != "java") ||
				len(answer.Source) > 32<<10 || strings.ContainsRune(answer.Source, 0) {
				return Invalid("Use Python or Java with at most 32 KiB of source per question.")
			}
			total += len(answer.Source)
		}
	}
	if total > 128<<10 {
		return Invalid("Keep the combined source within 128 KiB.")
	}
	return nil
}

func EqualAnswers(left, right map[int]Answer) bool {
	return maps.EqualFunc(left, right, func(a, b Answer) bool {
		if a.Source != b.Source || a.Lang != b.Lang {
			return false
		}
		if a.Choice == nil || b.Choice == nil {
			return a.Choice == b.Choice
		}
		return *a.Choice == *b.Choice
	})
}
