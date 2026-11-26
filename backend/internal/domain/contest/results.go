package contest

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	LeaderboardPageSize = 20
	MaxAliasRunes       = 64
)

type PublishResultsInput struct {
	Reason              string   `json:"reason"`
	ReleaseExplanations bool     `json:"releaseExplanations"`
	WithholdAttemptIDs  []string `json:"withholdAttemptIds,omitempty"`
}

type QuestionBreakdown struct {
	QuestionIndex        int        `json:"questionIndex"`
	Kind                 string     `json:"kind"`
	Prompt               string     `json:"prompt"`
	Points               int        `json:"points"`
	PossiblePoints       int        `json:"possiblePoints"`
	Verdict              string     `json:"verdict"`
	StudentChoice        *int       `json:"studentChoice,omitempty"`
	CorrectChoice        *int       `json:"correctChoice,omitempty"`
	Choices              []string   `json:"choices,omitempty"`
	Explanation          string     `json:"explanation,omitempty"`
	Language             string     `json:"language,omitempty"`
	Source               string     `json:"source,omitempty"`
	OfficialSubmissionID string     `json:"officialSubmissionId,omitempty"`
	SubmittedAt          *time.Time `json:"submittedAt,omitempty"`
}

type StudentReport struct {
	AssessmentID   string              `json:"assessmentId"`
	Title          string              `json:"title"`
	Version        int                 `json:"version"`
	Status         string              `json:"status"`
	AttemptID      string              `json:"attemptId"`
	Rank           *int                `json:"rank,omitempty"`
	TotalRanked    int                 `json:"totalRanked"`
	Points         *int                `json:"points,omitempty"`
	PossiblePoints int                 `json:"possiblePoints"`
	McqPoints      int                 `json:"mcqPoints"`
	CodingPoints   int                 `json:"codingPoints"`
	ElapsedMs      *int64              `json:"elapsedMs,omitempty"`
	PublishedAt    time.Time           `json:"publishedAt"`
	ReviewStatus   string              `json:"reviewStatus"`
	ReviewNote     string              `json:"reviewNote,omitempty"`
	Breakdown      []QuestionBreakdown `json:"breakdown"`
}

type LeaderboardRow struct {
	Rank           int    `json:"rank"`
	StudentID      string `json:"studentId,omitempty"`
	DisplayName    string `json:"displayName"`
	Points         int    `json:"points"`
	PossiblePoints int    `json:"possiblePoints"`
	ElapsedMs      int64  `json:"elapsedMs"`
	IsSelf         bool   `json:"isSelf"`
}

type LeaderboardPage struct {
	AssessmentID string           `json:"assessmentId"`
	Title        string           `json:"title"`
	Version      int              `json:"version"`
	PublishedAt  time.Time        `json:"publishedAt"`
	Page         int              `json:"page"`
	PageSize     int              `json:"pageSize"`
	TotalRanked  int              `json:"totalRanked"`
	HasMore      bool             `json:"hasMore"`
	Items        []LeaderboardRow `json:"items"`
	SelfRow      *LeaderboardRow  `json:"selfRow,omitempty"`
}

type ReleaseRecord struct {
	Version             int       `json:"version"`
	PublishedBy         string    `json:"publishedBy"`
	Reason              string    `json:"reason"`
	ReleaseExplanations bool      `json:"releaseExplanations"`
	WithheldAttemptIDs  []string  `json:"withheldAttemptIds"`
	PublishedAt         time.Time `json:"publishedAt"`
}

type StaffResultsView struct {
	AssessmentID        string          `json:"assessmentId"`
	LatestVersion       int             `json:"latestVersion"`
	ReleaseExplanations bool            `json:"releaseExplanations"`
	Releases            []ReleaseRecord `json:"releases"`
	Leaderboard         LeaderboardPage `json:"leaderboard"`
}

type StudentResultSummary struct {
	AssessmentID   string    `json:"assessmentId"`
	Title          string    `json:"title"`
	Version        int       `json:"version"`
	Status         string    `json:"status"`
	Rank           *int      `json:"rank,omitempty"`
	Points         *int      `json:"points,omitempty"`
	PossiblePoints int       `json:"possiblePoints"`
	PublishedAt    time.Time `json:"publishedAt"`
}

type ResultsRepository interface {
	PublishOrRegrade(ctx context.Context, actor Actor, assessmentID string, cmd PublishResultsInput) (ReleaseRecord, error)
	GetStaffResults(ctx context.Context, actor Actor, assessmentID string, page int) (StaffResultsView, error)
	GetStudentReport(ctx context.Context, assessmentID, studentID string) (StudentReport, error)
	GetLeaderboard(ctx context.Context, assessmentID, viewerID string, isStaff bool, page int) (LeaderboardPage, error)
	SetAlias(ctx context.Context, assessmentID, studentID string, alias string) error
	ListStudentResultsSummary(ctx context.Context, studentID string) ([]StudentResultSummary, error)
}

type RankCandidate struct {
	AttemptID      string
	StudentID      string
	DisplayName    string
	Points         int
	PossiblePoints int
	ElapsedMs      int64
}

type RankedEntry struct {
	RankCandidate
	Rank int
}

// RankLeaderboard applies standard competition ranking (1, 2, 2, 4):
// candidates are sorted by Points DESC, ElapsedMs ASC, AttemptID ASC.
// Exact ties on (Points, ElapsedMs) share the same rank and the next rank
// jumps by the number of prior entries (i + 1).
func RankLeaderboard(candidates []RankCandidate) []RankedEntry {
	if len(candidates) == 0 {
		return []RankedEntry{}
	}
	sorted := slices.Clone(candidates)
	slices.SortFunc(sorted, func(a, b RankCandidate) int {
		if c := cmp.Compare(b.Points, a.Points); c != 0 {
			return c
		}
		if c := cmp.Compare(a.ElapsedMs, b.ElapsedMs); c != 0 {
			return c
		}
		return cmp.Compare(a.AttemptID, b.AttemptID)
	})

	ranked := make([]RankedEntry, len(sorted))
	prevRank := 1
	for i, c := range sorted {
		rank := i + 1
		if i > 0 && c.Points == sorted[i-1].Points && c.ElapsedMs == sorted[i-1].ElapsedMs {
			rank = prevRank
		}
		prevRank = rank
		ranked[i] = RankedEntry{
			RankCandidate: c,
			Rank:          rank,
		}
	}
	return ranked
}

// ValidateAlias trims and validates a contest leaderboard display alias.
// An empty string clears the alias so the student's account name is displayed.
func ValidateAlias(alias string) (string, error) {
	if !utf8.ValidString(alias) {
		return "", Invalid("Display alias must be valid UTF-8 text.")
	}
	trimmed := strings.TrimSpace(alias)
	if utf8.RuneCountInString(trimmed) > MaxAliasRunes {
		return "", Invalid("Keep the display alias within 64 characters.")
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return "", Invalid("Display alias cannot contain control characters.")
		}
	}
	return trimmed, nil
}

// ValidatePublishInput validates and normalizes a publish/regrade command.
// Updating an existing published version (prevVersion > 0) requires a non-empty reason.
func ValidatePublishInput(cmd *PublishResultsInput, prevVersion int) error {
	if !utf8.ValidString(cmd.Reason) {
		return Invalid("Release reason must be valid UTF-8 text.")
	}
	cmd.Reason = strings.TrimSpace(cmd.Reason)
	if utf8.RuneCountInString(cmd.Reason) > maxReasonRunes {
		return Invalid("Keep the release reason within 2,000 characters.")
	}
	if prevVersion > 0 && cmd.Reason == "" {
		return Invalid("Provide a reason when regrading or republishing results.")
	}
	if len(cmd.WithholdAttemptIDs) == 0 {
		cmd.WithholdAttemptIDs = nil
		return nil
	}
	seen := make(map[string]bool, len(cmd.WithholdAttemptIDs))
	clean := make([]string, 0, len(cmd.WithholdAttemptIDs))
	for _, raw := range cmd.WithholdAttemptIDs {
		id := strings.ToLower(strings.TrimSpace(raw))
		if !isUUID(id) {
			return Invalid("Withheld attempt IDs must be valid UUIDs.")
		}
		if !seen[id] {
			seen[id] = true
			clean = append(clean, id)
		}
	}
	slices.Sort(clean)
	cmd.WithholdAttemptIDs = clean
	return nil
}

type CodingSubmissionDetail struct {
	ID          string
	Question    int
	Language    string
	Source      string
	SubmittedAt time.Time
	Status      string
	Verdict     string
}

type ReportBuildInput struct {
	AssessmentID        string
	Title               string
	Version             int
	AttemptID           string
	PublishedAt         time.Time
	ReleaseExplanations bool
	Withheld            bool
	Rank                *int
	TotalRanked         int
	ReviewStatus        string
	ReviewNote          string
	Questions           []Question
	Answers             map[int]Answer
	Score               Score
	Submissions         []CodingSubmissionDetail
}

// BuildStudentReport constructs a student's published or withheld report projection.
// Withheld reports omit scores, rank, and question breakdowns while retaining private
// review status/note. Hidden coding test cases are never included in breakdowns.
func BuildStudentReport(in ReportBuildInput) StudentReport {
	reviewStatus := in.ReviewStatus
	if reviewStatus == "" {
		reviewStatus = "clear"
	}
	status := "published"
	if in.Withheld {
		status = "withheld"
	}
	report := StudentReport{
		AssessmentID:   in.AssessmentID,
		Title:          in.Title,
		Version:        in.Version,
		Status:         status,
		AttemptID:      in.AttemptID,
		TotalRanked:    in.TotalRanked,
		PossiblePoints: in.Score.Possible,
		PublishedAt:    in.PublishedAt,
		ReviewStatus:   reviewStatus,
		ReviewNote:     in.ReviewNote,
		Breakdown:      make([]QuestionBreakdown, 0),
	}
	if in.Withheld {
		return report
	}

	if in.Rank != nil {
		r := *in.Rank
		report.Rank = &r
	}
	if in.Score.Points != nil {
		pts := *in.Score.Points
		report.Points = &pts
	}
	if in.Score.Elapsed != nil {
		el := *in.Score.Elapsed
		report.ElapsedMs = &el
	}

	byID := make(map[string]CodingSubmissionDetail, len(in.Submissions))
	latestByQ := make(map[int]CodingSubmissionDetail)
	for _, sub := range in.Submissions {
		byID[sub.ID] = sub
		prev, ok := latestByQ[sub.Question]
		if !ok || sub.SubmittedAt.After(prev.SubmittedAt) || (sub.SubmittedAt.Equal(prev.SubmittedAt) && sub.ID < prev.ID) {
			latestByQ[sub.Question] = sub
		}
	}

	report.Breakdown = make([]QuestionBreakdown, 0, len(in.Questions))
	for idx, q := range in.Questions {
		var qScore QuestionScore
		if idx < len(in.Score.Details) {
			qScore = in.Score.Details[idx]
		}
		qb := QuestionBreakdown{
			QuestionIndex:  idx,
			Kind:           q.Kind,
			Prompt:         q.Prompt,
			Points:         qScore.Points,
			PossiblePoints: q.Points,
		}
		switch q.Kind {
		case "mcq":
			report.McqPoints += qScore.Points
			order := ChoiceOrder(in.AttemptID, idx, len(q.Choices))
			choices := make([]string, len(order))
			displayedCorrect := -1
			for dispIdx, origIdx := range order {
				choices[dispIdx] = q.Choices[origIdx]
				if q.CorrectChoice != nil && origIdx == *q.CorrectChoice {
					displayedCorrect = dispIdx
				}
			}
			qb.Choices = choices
			if ans, ok := in.Answers[idx]; ok && ans.Choice != nil {
				c := *ans.Choice
				qb.StudentChoice = &c
				if qScore.Points > 0 {
					qb.Verdict = "correct"
				} else {
					qb.Verdict = "incorrect"
				}
			} else {
				qb.Verdict = "unanswered"
			}
			if in.ReleaseExplanations {
				if displayedCorrect >= 0 {
					qb.CorrectChoice = &displayedCorrect
				}
				qb.Explanation = q.Explanation
			}
		case "coding":
			report.CodingPoints += qScore.Points
			if qScore.SubmissionID != "" {
				if sub, ok := byID[qScore.SubmissionID]; ok {
					qb.OfficialSubmissionID = sub.ID
					qb.Language = sub.Language
					qb.Source = sub.Source
					t := sub.SubmittedAt
					qb.SubmittedAt = &t
					qb.Verdict = codingVerdict(sub)
				} else {
					qb.OfficialSubmissionID = qScore.SubmissionID
					qb.Verdict = "Accepted"
				}
			} else if sub, ok := latestByQ[idx]; ok {
				qb.OfficialSubmissionID = sub.ID
				qb.Language = sub.Language
				qb.Source = sub.Source
				t := sub.SubmittedAt
				qb.SubmittedAt = &t
				qb.Verdict = codingVerdict(sub)
			} else {
				qb.Verdict = "not_submitted"
				if ans, ok := in.Answers[idx]; ok {
					qb.Language = ans.Lang
					qb.Source = ans.Source
				}
			}
		}
		report.Breakdown = append(report.Breakdown, qb)
	}
	return report
}

func codingVerdict(sub CodingSubmissionDetail) string {
	if sub.Verdict != "" {
		return sub.Verdict
	}
	switch sub.Status {
	case "accepted":
		return "Accepted"
	case "failed":
		return "Judge Error"
	default:
		return sub.Status
	}
}
