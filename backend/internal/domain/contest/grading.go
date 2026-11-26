package contest

import (
	"time"

	"vedam/lms/internal/domain/errs"
)

func JudgeUnavailable() error {
	return errs.New(errs.ErrUnavailable, "CONTEST_JUDGE_UNAVAILABLE", "The official judge is unavailable. Try again shortly.")
}

type CodingCommand struct {
	RequestID string
	Question  int
	Language  string
	Source    string
}

type CodingSubmission struct {
	ID          string    `json:"id"`
	Question    int       `json:"questionIndex"`
	Language    string    `json:"language"`
	SubmittedAt time.Time `json:"submittedAt"`
	Status      string    `json:"status"`
}

type QuestionScore struct {
	Points           int    `json:"points"`
	SubmissionID     string `json:"submissionId,omitempty"`
	NeedsJudgeReview bool   `json:"needsJudgeReview,omitempty"`
}

type Score struct {
	Status   string
	Points   *int
	Possible int
	Elapsed  *int64
	Details  []QuestionScore
}

// Scoring uses the last saved MCQ choice and the earliest accepted official
// submission per coding question. The latest scoring event sets tie-break time.
func ScoreAttempt(id string, started, submitted time.Time, questions []Question, answers map[int]Answer, saved map[int]time.Time, submissions []CodingSubmission) Score {
	best := make(map[int]CodingSubmission)
	failed := make(map[int]time.Time)
	for _, submission := range submissions {
		switch submission.Status {
		case "accepted":
			previous, ok := best[submission.Question]
			if !ok || submission.SubmittedAt.Before(previous.SubmittedAt) ||
				(submission.SubmittedAt.Equal(previous.SubmittedAt) && submission.ID < previous.ID) {
				best[submission.Question] = submission
			}
		case "failed":
			previous, ok := failed[submission.Question]
			if !ok || submission.SubmittedAt.Before(previous) {
				failed[submission.Question] = submission.SubmittedAt
			}
		}
	}

	score := Score{Status: "ready", Details: make([]QuestionScore, len(questions))}
	total := 0
	reached := started
	for index, q := range questions {
		score.Possible += q.Points
		detail := &score.Details[index]
		switch q.Kind {
		case "mcq":
			choice := answers[index].Choice
			if choice != nil && *choice >= 0 && *choice < len(q.Choices) &&
				q.CorrectChoice != nil && ChoiceOrder(id, index, len(q.Choices))[*choice] == *q.CorrectChoice {
				detail.Points = q.Points
				when := saved[index]
				if when.IsZero() {
					when = submitted
				}
				if when.After(reached) {
					reached = when
				}
			}
		case "coding":
			if submission, ok := best[index]; ok {
				detail.Points = q.Points
				detail.SubmissionID = submission.ID
				if submission.SubmittedAt.After(reached) {
					reached = submission.SubmittedAt
				}
			}
			// A failed earlier job might have scored sooner, even when a later submission passed.
			earliest, hasFailure := failed[index]
			accepted, hasAccepted := best[index]
			if hasFailure && (!hasAccepted || earliest.Before(accepted.SubmittedAt)) {
				detail.NeedsJudgeReview = true
				score.Status = "needs_review"
			}
		}
		total += detail.Points
	}
	if score.Status == "ready" {
		elapsed := reached.Sub(started).Milliseconds()
		score.Points, score.Elapsed = &total, &elapsed
	}
	return score
}
