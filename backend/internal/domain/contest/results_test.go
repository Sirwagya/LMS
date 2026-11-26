package contest

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"vedam/lms/internal/domain/errs"
)

func TestRankLeaderboard(t *testing.T) {
	t.Run("handles empty slice", func(t *testing.T) {
		got := RankLeaderboard(nil)
		if len(got) != 0 {
			t.Fatalf("expected 0 entries, got %d", len(got))
		}
	})

	t.Run("standard competition ranking with exact ties and elapsed tiebreaks", func(t *testing.T) {
		input := []RankCandidate{
			{AttemptID: "d", StudentID: "s4", DisplayName: "Delta", Points: 80, PossiblePoints: 100, ElapsedMs: 5000},
			{AttemptID: "b", StudentID: "s2", DisplayName: "Bravo", Points: 90, PossiblePoints: 100, ElapsedMs: 12000},
			{AttemptID: "a", StudentID: "s1", DisplayName: "Alpha", Points: 100, PossiblePoints: 100, ElapsedMs: 15000},
			{AttemptID: "c", StudentID: "s3", DisplayName: "Charlie", Points: 90, PossiblePoints: 100, ElapsedMs: 12000},
			{AttemptID: "e", StudentID: "s5", DisplayName: "Echo", Points: 90, PossiblePoints: 100, ElapsedMs: 14000},
			{AttemptID: "f", StudentID: "s6", DisplayName: "Foxtrot", Points: 80, PossiblePoints: 100, ElapsedMs: 5000},
		}
		origFirst := input[0].AttemptID

		ranked := RankLeaderboard(input)
		if input[0].AttemptID != origFirst {
			t.Fatalf("expected input slice not to be mutated")
		}
		if len(ranked) != 6 {
			t.Fatalf("expected 6 ranked entries, got %d", len(ranked))
		}

		expected := []struct {
			attemptID string
			rank      int
		}{
			{attemptID: "a", rank: 1},
			{attemptID: "b", rank: 2},
			{attemptID: "c", rank: 2},
			{attemptID: "e", rank: 4},
			{attemptID: "d", rank: 5},
			{attemptID: "f", rank: 5},
		}
		for i, exp := range expected {
			if ranked[i].AttemptID != exp.attemptID || ranked[i].Rank != exp.rank {
				t.Fatalf("index %d: expected (%s, rank %d), got (%s, rank %d)",
					i, exp.attemptID, exp.rank, ranked[i].AttemptID, ranked[i].Rank)
			}
		}
	})
}

func TestValidateAlias(t *testing.T) {
	t.Run("trims valid alias and allows clearing", func(t *testing.T) {
		got, err := ValidateAlias("  CodeNinja_01  ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "CodeNinja_01" {
			t.Fatalf("expected trimmed alias, got %q", got)
		}

		cleared, err := ValidateAlias("   ")
		if err != nil {
			t.Fatalf("unexpected error clearing alias: %v", err)
		}
		if cleared != "" {
			t.Fatalf("expected empty alias, got %q", cleared)
		}
	})

	t.Run("enforces 64 rune limit", func(t *testing.T) {
		exact := strings.Repeat("é", MaxAliasRunes)
		if _, err := ValidateAlias(exact); err != nil {
			t.Fatalf("expected 64 UTF-8 runes to pass, got %v", err)
		}

		tooLong := strings.Repeat("a", MaxAliasRunes+1)
		if _, err := ValidateAlias(tooLong); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("expected ErrValidation for >64 runes, got %v", err)
		}
	})

	t.Run("rejects control characters and invalid utf8", func(t *testing.T) {
		for _, bad := range []string{"bad\nalias", "bad\talias", "bad\x00alias", "bad\x7falias", string([]byte{0xff, 0xfe})} {
			if _, err := ValidateAlias(bad); !errors.Is(err, errs.ErrValidation) {
				t.Fatalf("expected ErrValidation for %q, got %v", bad, err)
			}
		}
	})
}

func TestValidatePublishInput(t *testing.T) {
	id1 := "11111111-1111-4111-8111-111111111111"
	id2 := "22222222-2222-4222-8222-222222222222"

	t.Run("initial publish allows empty reason", func(t *testing.T) {
		cmd := PublishResultsInput{
			Reason:              "   ",
			ReleaseExplanations: true,
			WithholdAttemptIDs:  []string{strings.ToUpper(id2), id1, id2},
		}
		if err := ValidatePublishInput(&cmd, 0); err != nil {
			t.Fatalf("expected initial publish without reason to succeed, got %v", err)
		}
		if cmd.Reason != "" {
			t.Fatalf("expected trimmed empty reason, got %q", cmd.Reason)
		}
		if len(cmd.WithholdAttemptIDs) != 2 || cmd.WithholdAttemptIDs[0] != id1 || cmd.WithholdAttemptIDs[1] != id2 {
			t.Fatalf("expected normalized sorted withheld IDs, got %#v", cmd.WithholdAttemptIDs)
		}
	})

	t.Run("regrade requires non-empty reason", func(t *testing.T) {
		cmd := PublishResultsInput{Reason: "   "}
		if err := ValidatePublishInput(&cmd, 1); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("expected ErrValidation when regrade reason is empty, got %v", err)
		}

		cmd.Reason = "Re-evaluated coding submission after judge recovery"
		if err := ValidatePublishInput(&cmd, 1); err != nil {
			t.Fatalf("expected regrade with reason to pass, got %v", err)
		}
	})

	t.Run("rejects oversized reason and invalid attempt IDs", func(t *testing.T) {
		tooLong := PublishResultsInput{Reason: strings.Repeat("r", 2001)}
		if err := ValidatePublishInput(&tooLong, 0); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("expected ErrValidation for oversized reason, got %v", err)
		}

		badID := PublishResultsInput{Reason: "ok", WithholdAttemptIDs: []string{"not-a-uuid"}}
		if err := ValidatePublishInput(&badID, 0); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("expected ErrValidation for invalid attempt UUID, got %v", err)
		}
	})
}

func TestBuildStudentReport(t *testing.T) {
	attemptID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	assessmentID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	subID := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	now := time.Date(2026, 11, 26, 18, 0, 0, 0, time.UTC)
	started := now.Add(-20 * time.Minute)
	submitted := now.Add(-5 * time.Minute)

	correctOrig := 1
	exIn, exOut := "1 2\n", "3\n"
	hidIn, hidOut := "SECRET_HIDDEN_INPUT", "SECRET_HIDDEN_OUTPUT"
	questions := []Question{
		{
			Kind:          "mcq",
			Prompt:        "What is 2+2?",
			Points:        10,
			Choices:       []string{"3", "4", "5"},
			CorrectChoice: &correctOrig,
			Explanation:   "2 plus 2 equals 4.",
		},
		{
			Kind:   "coding",
			Prompt: "Add two numbers",
			Points: 20,
			TestCases: []TestCase{
				{Input: &exIn, Expected: &exOut, Hidden: false},
				{Input: &hidIn, Expected: &hidOut, Hidden: true},
			},
		},
	}

	// Determine the shuffled displayed choice that corresponds to correctOrig=1.
	order := ChoiceOrder(attemptID, 0, len(questions[0].Choices))
	displayedCorrect := -1
	for d, orig := range order {
		if orig == correctOrig {
			displayedCorrect = d
			break
		}
	}
	if displayedCorrect < 0 {
		t.Fatalf("failed to locate correct choice in ChoiceOrder")
	}

	answers := map[int]Answer{
		0: {Choice: &displayedCorrect},
	}
	saved := map[int]time.Time{
		0: started.Add(2 * time.Minute),
	}
	subs := []CodingSubmissionDetail{
		{
			ID:          subID,
			Question:    1,
			Language:    "python",
			Source:      "a, b = map(int, input().split())\nprint(a + b)\n",
			SubmittedAt: started.Add(5 * time.Minute),
			Status:      "accepted",
			Verdict:     "Accepted",
		},
	}
	score := ScoreAttempt(attemptID, started, submitted, questions, answers, saved, []CodingSubmission{
		{ID: subID, Question: 1, Language: "python", SubmittedAt: subs[0].SubmittedAt, Status: "accepted"},
	})
	rank := 1

	t.Run("hides explanations when ReleaseExplanations is false and never leaks hidden tests", func(t *testing.T) {
		report := BuildStudentReport(ReportBuildInput{
			AssessmentID:        assessmentID,
			Title:               "Midterm Contest",
			Version:             1,
			AttemptID:           attemptID,
			PublishedAt:         now,
			ReleaseExplanations: false,
			Withheld:            false,
			Rank:                &rank,
			TotalRanked:         10,
			ReviewStatus:        "clear",
			Questions:           questions,
			Answers:             answers,
			Score:               score,
			Submissions:         subs,
		})

		if report.Status != "published" || report.Points == nil || *report.Points != 30 {
			t.Fatalf("expected published 30-point report, got %+v", report)
		}
		if report.McqPoints != 10 || report.CodingPoints != 20 {
			t.Fatalf("expected mcq=10 coding=20, got %d / %d", report.McqPoints, report.CodingPoints)
		}
		if len(report.Breakdown) != 2 {
			t.Fatalf("expected 2 breakdown items, got %d", len(report.Breakdown))
		}
		mcq := report.Breakdown[0]
		if mcq.CorrectChoice != nil || mcq.Explanation != "" {
			t.Fatalf("expected explanations and correct choice withheld, got %+v", mcq)
		}
		if mcq.Verdict != "correct" || mcq.Choices[*mcq.StudentChoice] != "4" {
			t.Fatalf("expected displayed choice to match answer '4', got %+v", mcq)
		}
		coding := report.Breakdown[1]
		if coding.OfficialSubmissionID != subID || coding.Verdict != "Accepted" || coding.Language != "python" {
			t.Fatalf("expected accepted coding breakdown, got %+v", coding)
		}

		raw, err := json.Marshal(report)
		if err != nil {
			t.Fatalf("marshal report: %v", err)
		}
		if strings.Contains(string(raw), "SECRET_HIDDEN") {
			t.Fatalf("hidden test case leaked into student report JSON: %s", raw)
		}
	})

	t.Run("includes displayed CorrectChoice and Explanation when ReleaseExplanations is true", func(t *testing.T) {
		report := BuildStudentReport(ReportBuildInput{
			AssessmentID:        assessmentID,
			Title:               "Midterm Contest",
			Version:             1,
			AttemptID:           attemptID,
			PublishedAt:         now,
			ReleaseExplanations: true,
			Withheld:            false,
			Rank:                &rank,
			TotalRanked:         10,
			Questions:           questions,
			Answers:             answers,
			Score:               score,
			Submissions:         subs,
		})

		mcq := report.Breakdown[0]
		if mcq.CorrectChoice == nil || *mcq.CorrectChoice != displayedCorrect {
			t.Fatalf("expected displayed CorrectChoice=%d, got %v", displayedCorrect, mcq.CorrectChoice)
		}
		if mcq.Explanation != "2 plus 2 equals 4." {
			t.Fatalf("expected explanation to be released, got %q", mcq.Explanation)
		}
	})

	t.Run("withheld report omits scores, rank, and breakdown while keeping review note", func(t *testing.T) {
		report := BuildStudentReport(ReportBuildInput{
			AssessmentID:        assessmentID,
			Title:               "Midterm Contest",
			Version:             1,
			AttemptID:           attemptID,
			PublishedAt:         now,
			ReleaseExplanations: true,
			Withheld:            true,
			Rank:                &rank,
			TotalRanked:         10,
			ReviewStatus:        "under_review",
			ReviewNote:          "Manual verification pending for coding question 2.",
			Questions:           questions,
			Answers:             answers,
			Score:               score,
			Submissions:         subs,
		})

		if report.Status != "withheld" {
			t.Fatalf("expected withheld status, got %q", report.Status)
		}
		if report.Rank != nil || report.Points != nil || report.ElapsedMs != nil {
			t.Fatalf("expected rank, points, elapsedMs nil when withheld, got %+v", report)
		}
		if report.McqPoints != 0 || report.CodingPoints != 0 || len(report.Breakdown) != 0 {
			t.Fatalf("expected zero subscores and empty breakdown when withheld, got %+v", report)
		}
		if report.ReviewStatus != "under_review" || report.ReviewNote == "" {
			t.Fatalf("expected private review status and note preserved, got %+v", report)
		}
	})
}
