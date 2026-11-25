package learning

import (
	"context"
	"time"
)

type AttendanceSummary struct {
	UserID     string `json:"user_id"`
	CourseCode string `json:"course"`
	Attended   int    `json:"attended"`
	Total      int    `json:"total"`
}

type Discussion struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	LectureID  int64     `json:"lecture_id"`
	AuthorName string    `json:"name"`
	Body       string    `json:"body"`
	Created    string    `json:"created"`
	CreatedAt  time.Time `json:"-"`
}

type AttendanceReader interface {
	GetForCourse(ctx context.Context, userID, courseCode string) (*AttendanceSummary, error)
	ListForStudent(ctx context.Context, userID string) (map[string]AttendanceSummary, error)
	ListAll(ctx context.Context) ([]AttendanceSummary, error)
}

type AttendanceWriter interface {
	Upsert(ctx context.Context, summary AttendanceSummary) error
}

type ProgressRepository interface {
	ListCompletedLectures(ctx context.Context, userID string) ([]int64, error)
	MarkComplete(ctx context.Context, userID string, lectureID int64) error
	MarkIncomplete(ctx context.Context, userID string, lectureID int64) error
}

type DiscussionRepository interface {
	ListForLecture(ctx context.Context, lectureID int64) ([]Discussion, error)
	Create(ctx context.Context, d *Discussion) error
}
