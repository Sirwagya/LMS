package catalog

import (
	"context"
	"time"
)

type Semester struct {
	Number    int    `json:"number"`
	Label     string `json:"label"`
	IsCurrent bool   `json:"isCurrent"`
}

type Course struct {
	Code            string `json:"code"`
	Semester        int    `json:"semester,omitempty"`
	Credits         int    `json:"credits"`
	Name            string `json:"name"`
	Instructor      string `json:"instructor,omitempty"`
	Next            string `json:"next"`
	Att             string `json:"att"`
	AttendedClasses int    `json:"attendedClasses"`
	TotalClasses    int    `json:"totalClasses"`
	Skips           string `json:"skips"`
	Version         int64  `json:"version,omitempty"`
}

type Resource struct {
	ID    string  `json:"id,omitempty"`
	Title string  `json:"title"`
	URL   *string `json:"url,omitempty"`
	Type  string  `json:"type,omitempty"`
	Size  string  `json:"size,omitempty"`
}

type LectureAssignmentRef struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	DueDate string `json:"dueDate,omitempty"`
	Status  string `json:"status,omitempty"`
	Problem any    `json:"problem,omitempty"`
}

type Lecture struct {
	ID           int64                  `json:"id"`
	CourseCode   string                 `json:"-"`
	Code         string                 `json:"code"`
	Date         string                 `json:"date"`
	Month        string                 `json:"month"`
	Unit         int                    `json:"unit"`
	Title        string                 `json:"title"`
	RawTitle     string                 `json:"rawTitle"`
	Attendance   string                 `json:"attendance"`
	Summary      string                 `json:"summary"`
	Slides       string                 `json:"slides"`
	Duration     string                 `json:"duration,omitempty"`
	RecordingURL *string                `json:"recordingUrl"`
	Resources    []Resource             `json:"resources"`
	Assignments  []LectureAssignmentRef `json:"assignments"`
	SortOrder    int                    `json:"-"`
	Version      int64                  `json:"version,omitempty"`
}

type ScheduleClass struct {
	ID       string `json:"id,omitempty"`
	Week     string `json:"-"`
	SlotDate string `json:"-"`
	From     string `json:"from"`
	To       string `json:"to"`
	Code     string `json:"code"`
	Room     string `json:"room"`
	Topic    string `json:"topic"`
	Type     string `json:"type,omitempty"`
}

type WeekSchedule struct {
	Start     string                     `json:"start"`
	Published bool                       `json:"published"`
	Days      map[string][]ScheduleClass `json:"days"`
}

type Notice struct {
	ID          string  `json:"id"`
	Placement   string  `json:"-"`
	Tag         string  `json:"tag"`
	TagBg       string  `json:"tagBg"`
	Time        string  `json:"time"`
	Title       string  `json:"title"`
	Desc        string  `json:"desc"`
	ActionLabel *string `json:"actionLabel,omitempty"`
	ActionRoute *string `json:"actionRoute,omitempty"`
	CreatedAt   time.Time `json:"-"`
}

type Asset struct {
	ID         string
	Filename   string
	MimeType   string
	ByteSize   int64
	StorageKey string
	SHA256     string
	CreatedAt  time.Time
}

type CatalogReader interface {
	ListSemesters(ctx context.Context) ([]Semester, error)
	ListCoursesBySemester(ctx context.Context) (map[int][]Course, error)
	GetCourse(ctx context.Context, code string) (*Course, error)
	ListLecturesByCourse(ctx context.Context) (map[string][]Lecture, error)
	GetLecture(ctx context.Context, id int64) (*Lecture, error)
}

type CatalogWriter interface {
	CreateCourse(ctx context.Context, course *Course) error
	UpdateCourseMeta(ctx context.Context, code, name, instructor string) error
	CreateLecture(ctx context.Context, lecture *Lecture) (int64, error)
	UpdateLectureMeta(ctx context.Context, id int64, title, summary string) error
	SetLectureRecording(ctx context.Context, lectureID int64, recordingURL, assetID string) error
	AddLectureResource(ctx context.Context, lectureID int64, res Resource, assetID *string) error
}

type ScheduleRepository interface {
	GetWeekSchedule(ctx context.Context) (*WeekSchedule, error)
	AddSlot(ctx context.Context, slot ScheduleClass) error
}

type NoticeRepository interface {
	ListByPlacement(ctx context.Context, placement string) ([]Notice, error)
	UpdateNotice(ctx context.Context, id, title, desc string) error
	CreateNotice(ctx context.Context, notice *Notice) error
}

type AssetMetadataRepository interface {
	Create(ctx context.Context, asset *Asset) error
	GetByID(ctx context.Context, id string) (*Asset, error)
}

type AssetBlobStore interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
}
