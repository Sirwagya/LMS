package academic

import "context"

type Campus struct {
	ID             int
	Name           string
	OrganizationID *int
}

type Program struct {
	ID       int
	CampusID int
	Title    string
	Label    string
}

type CampusPersonRole struct {
	CPRID      int64
	UserID     string
	CampusID   int
	PersonName string
	Email      string
}

type ProgramEnrolmentItem struct {
	ProgramEnrolmentID int64  `json:"programEnrolmentId"`
	ProgramID          int    `json:"programId"`
	EnrolmentDate      int64  `json:"enrolmentDate"`
	CPRID              int64  `json:"cprId"`
	CampusID           int    `json:"campusId"`
	OrganizationID     *int   `json:"organizationId"`
	PersonName         string `json:"personName"`
	Email              string `json:"email"`
	ProgramTitle       string `json:"programTitle"`
	ProgramLabel       string `json:"programLabel"`
}

type SemesterRecord struct {
	ID                 int    `json:"id"`
	CampusID           int    `json:"campusId"`
	ProgramID          int    `json:"programId"`
	Title              string `json:"title"`
	Label              string `json:"label"`
	StartDate          int64  `json:"startDate"`
	EndDate            int64  `json:"endDate"`
	ProgramTitle       string `json:"programTitle"`
	ProgramLabel       string `json:"programLabel"`
	RequiredAttendance int    `json:"requiredAttendance"`
}

type FaceBiometricRecord struct {
	HasFaceData bool      `json:"hasFaceData"`
	FaceJSON    string    `json:"faceJson"`
	Email       string    `json:"email"`
	PersonID    int64     `json:"personId"`
	Embedding   []float64 `json:"-"`
}

type CourseOfferingSpec struct {
	ID                       int64
	CourseID                 int64
	CourseCode               string
	SemesterID               int
	CampusID                 int
	ProgramID                int
	Label                    string
	Title                    string
	Batch                    string
	Credits                  int
	Description              string
	IconURL                  string
	BgColor                  string
	EnrolmentCount           int
	ClassSessionCount        int
	CourseRequiredAttendance int
}

type CourseOfferingEnrolmentItem struct {
	CourseOfferingEnrolmentID int64  `json:"courseOfferingEnrolmentId"`
	CourseOfferingID          int64  `json:"courseOfferingId"`
	Label                     string `json:"label"`
	CourseID                  int64  `json:"courseId"`
	SemesterID                int    `json:"semesterId"`
	CampusID                  int    `json:"campusId"`
	Title                     string `json:"title"`
	Batch                     string `json:"batch"`
	ProgramID                 int    `json:"programId"`
	Credits                   int    `json:"credits"`
	Description               string `json:"description"`
	IconURL                   string `json:"iconUrl"`
	BgColor                   string `json:"bgColor"`
	EnrolmentCount            int    `json:"enrolmentCount"`
	ClassSessionCount         int    `json:"classSessionCount"`
	CampusPersonRoleID        int64  `json:"campusPersonRoleId"`
	Name                      string `json:"name"`
	Email                     string `json:"email"`
	EnrolmentDate             int64  `json:"enrolmentDate"`
	PresentCount              int    `json:"presentCount"`
	AbsentCount               int    `json:"absentCount"`
	LateCount                 int    `json:"lateCount"`
	LeaveCount                int    `json:"leaveCount"`
	ExcusedCount              int    `json:"excusedCount"`
	AwaitingCount             int    `json:"awaitingCount"`
	TotalCount                int    `json:"totalCount"`
	CourseRequiredAttendance  int    `json:"courseRequiredAttendance"`
}

type ClassSessionItem struct {
	ID               int64   `json:"id"`
	CourseOfferingID int64   `json:"courseOfferingId"`
	StartTime        int64   `json:"startTime"`
	EndTime          int64   `json:"endTime"`
	Title            string  `json:"title"`
	Room             *string `json:"room"`
	ProgramID        int     `json:"programId"`
	SemesterID       int     `json:"semesterId"`
	CPRID            *int64  `json:"cprId"`
	CSCreatedAt      int64   `json:"csCreatedAt"`
	CSUpdatedAt      int64   `json:"csUpdatedAt"`
	Label            string  `json:"label"`
	Topic            string  `json:"topic"`
}

type ClassSessionFilter struct {
	SemesterID       *int
	CourseOfferingID *int64
	Page             int
	Size             int
}

type EnrolmentReader interface {
	ListProgramEnrolments(ctx context.Context, cprID int64) ([]ProgramEnrolmentItem, error)
	ListCourseOfferingEnrolments(ctx context.Context, cprID int64, semesterID *int) ([]CourseOfferingEnrolmentItem, error)
}

type ClassSessionReader interface {
	ListClassSessions(ctx context.Context, filter ClassSessionFilter) ([]ClassSessionItem, int64, error)
	ListSemestersByProgram(ctx context.Context, programID int) ([]SemesterRecord, error)
}

type BiometricRepository interface {
	GetFaceBiometrics(ctx context.Context, personID int64) (*FaceBiometricRecord, error)
	UpsertFaceBiometrics(ctx context.Context, rec FaceBiometricRecord) error
}

type AcademicHierarchyWriter interface {
	UpsertCampus(ctx context.Context, campus Campus) error
	UpsertProgram(ctx context.Context, program Program) error
	UpsertCampusPersonRole(ctx context.Context, role CampusPersonRole) error
	UpsertProgramEnrolment(ctx context.Context, id int64, programID int, cprID int64, enrolmentDate int64) error
	UpsertSemester(ctx context.Context, sem SemesterRecord) error
	UpsertCourseOffering(ctx context.Context, off CourseOfferingSpec) error
	UpsertCourseOfferingEnrolment(ctx context.Context, id, courseOfferingID, cprID, enrolmentDate int64, present, absent, late, leave, excused, awaiting, total int) error
	UpsertClassSession(ctx context.Context, session ClassSessionItem) error
}
