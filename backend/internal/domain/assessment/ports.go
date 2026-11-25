package assessment

import (
	"context"
	"encoding/json"
	"time"
)

type AssignmentKind string

const (
	KindWritten AssignmentKind = "written"
	KindCoding  AssignmentKind = "coding"
)

type TestCase struct {
	ID       int64  `json:"id,omitempty"`
	Input    string `json:"input"`
	Expected string `json:"expected"`
	Hidden   bool   `json:"hidden"`
	Ordinal  int    `json:"ordinal,omitempty"`
}

type CodingProblemSpec struct {
	Entrypoint   string            `json:"entrypoint"`
	Description  string            `json:"description"`
	InputFormat  string            `json:"inputFormat"`
	OutputFormat string            `json:"outputFormat"`
	Constraints  string            `json:"constraints"`
	Difficulty   string            `json:"difficulty"`
	StarterCode  map[string]string `json:"starterCode"`
	TestCases    []TestCase        `json:"testCases,omitempty"`
}

// PublicAssignment excludes hidden test cases at the type boundary (ISP).
type PublicAssignment struct {
	ID          string            `json:"id"`
	LectureID   int64             `json:"-"`
	CourseCode  string            `json:"course"`
	Kind        AssignmentKind    `json:"kind"`
	Title       string            `json:"title"`
	DueDate     string            `json:"dueDate,omitempty"`
	Difficulty  string            `json:"difficulty,omitempty"`
	Entrypoint  string            `json:"entrypoint,omitempty"`
	Description string            `json:"description,omitempty"`
	InputFormat string            `json:"inputFormat,omitempty"`
	OutputFmt   string            `json:"outputFormat,omitempty"`
	Constraints string            `json:"constraints,omitempty"`
	StarterCode map[string]string `json:"starterCode,omitempty"`
	Examples    []TestCase        `json:"examples,omitempty"`
	TotalTests  int               `json:"totalTests,omitempty"`
}

type StaffAssignment struct {
	PublicAssignment
	AllTestCases []TestCase `json:"testCases,omitempty"`
}

type StudentAssignmentReader interface {
	GetPublicByID(ctx context.Context, assignmentID string) (*PublicAssignment, error)
	ListAllPublic(ctx context.Context) ([]PublicAssignment, error)
}

type StaffAssignmentAuthor interface {
	GetWithHiddenTests(ctx context.Context, assignmentID string) (*StaffAssignment, error)
	ListAllWithHiddenTests(ctx context.Context) ([]StaffAssignment, error)
	CreateWritten(ctx context.Context, id string, lectureID int64, courseCode, title, dueDate string) error
	CreateCoding(ctx context.Context, id string, lectureID int64, courseCode, title, dueDate string, problem CodingProblemSpec) error
}

type Submission struct {
	ID           string  `json:"id"`
	UserID       string  `json:"user_id,omitempty"`
	StudentName  string  `json:"name,omitempty"`
	AssignmentID string  `json:"assignment_id"`
	Course       string  `json:"course"`
	Answer       string  `json:"answer"`
	Filename     string  `json:"filename"`
	AssetID      *string `json:"-"`
	Score        *string `json:"score"`
	Feedback     *string `json:"feedback"`
	Created      string  `json:"created"`
}

type SubmissionRepository interface {
	ListForStudent(ctx context.Context, userID string) ([]Submission, error)
	ListAll(ctx context.Context) ([]Submission, error)
	GetByID(ctx context.Context, id string) (*Submission, error)
	GetByUserAndAssignment(ctx context.Context, userID, assignmentID string) (*Submission, error)
	Upsert(ctx context.Context, sub *Submission) error
	Grade(ctx context.Context, submissionID, score, feedback string) error
}

type CodingAttempt struct {
	ID           string          `json:"id"`
	UserID       string          `json:"-"`
	AssignmentID string          `json:"-"`
	Language     string          `json:"language"`
	Source       string          `json:"source"`
	Verdict      string          `json:"verdict"`
	Passed       int             `json:"passed"`
	Total        int             `json:"total"`
	Result       json.RawMessage `json:"result"`
	Created      string          `json:"created"`
	CreatedAt    time.Time       `json:"-"`
}

type CodingAttemptRepository interface {
	Save(ctx context.Context, attempt *CodingAttempt) error
	History(ctx context.Context, userID, assignmentID string, limit int) ([]CodingAttempt, error)
}

type ContestAttempt struct {
	Label   string `json:"label"`
	Title   string `json:"title"`
	Date    string `json:"date"`
	Score   string `json:"score"`
	Percent int    `json:"percent"`
	Rank    string `json:"rank"`
}

type ContestSummaryItem struct {
	Percent   int              `json:"percent"`
	Detail    string           `json:"detail"`
	Completed *int             `json:"completed,omitempty"`
	Total     *int             `json:"total,omitempty"`
	BestRank  string           `json:"bestRank,omitempty"`
	History   []ContestAttempt `json:"history,omitempty"`
}

type ContestRecord struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	SubjectCode string `json:"subjectCode"`
	Date        string `json:"date"`
	Score       string `json:"score"`
	Percent     int    `json:"percent"`
	Rank        string `json:"rank"`
	Status      string `json:"status"`
}

type ContestRepository interface {
	ListSummariesForStudent(ctx context.Context, userID string) (map[string]ContestSummaryItem, error)
	ListRecentForStudent(ctx context.Context, userID string) ([]ContestRecord, error)
}
