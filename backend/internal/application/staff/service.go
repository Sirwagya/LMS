package staff

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"vedam/lms/internal/domain/assessment"
	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/clock"
	"vedam/lms/internal/domain/errs"
	finance "vedam/lms/internal/domain/finance"
	"vedam/lms/internal/domain/iam"
	"vedam/lms/internal/domain/learning"
	syncdomain "vedam/lms/internal/domain/sync"
	"vedam/lms/internal/domain/uow"
)

var (
	courseCodeRe = regexp.MustCompile(`^[\w-]+$`)
	isoDateRe    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	hhmmRe       = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	scoreRe      = regexp.MustCompile(`^\d+(\.\d+)?\s*/\s*\d+(\.\d+)?$`)
)

type Service struct {
	tx         uow.TxManager
	catRead    catalog.CatalogReader
	catWrite   catalog.CatalogWriter
	schedRepo  catalog.ScheduleRepository
	noticeRepo catalog.NoticeRepository
	attWrite   learning.AttendanceWriter
	assignAuth assessment.StaffAssignmentAuthor
	subRepo    assessment.SubmissionRepository
	userRead   iam.UserReader
	userWrite  iam.UserWriter
	hasher     iam.PasswordHasher
	invWrite   finance.InvoiceWriter
	outbox     syncdomain.OutboxWriter
	clk        clock.Clock
	newID      func() string
}

func NewService(
	tx uow.TxManager,
	catRead catalog.CatalogReader,
	catWrite catalog.CatalogWriter,
	schedRepo catalog.ScheduleRepository,
	noticeRepo catalog.NoticeRepository,
	attWrite learning.AttendanceWriter,
	assignAuth assessment.StaffAssignmentAuthor,
	subRepo assessment.SubmissionRepository,
	userRead iam.UserReader,
	userWrite iam.UserWriter,
	hasher iam.PasswordHasher,
	invWrite finance.InvoiceWriter,
	outbox syncdomain.OutboxWriter,
	clk clock.Clock,
	newID func() string,
) *Service {
	return &Service{
		tx:         tx,
		catRead:    catRead,
		catWrite:   catWrite,
		schedRepo:  schedRepo,
		noticeRepo: noticeRepo,
		attWrite:   attWrite,
		assignAuth: assignAuth,
		subRepo:    subRepo,
		userRead:   userRead,
		userWrite:  userWrite,
		hasher:     hasher,
		invWrite:   invWrite,
		outbox:     outbox,
		clk:        clk,
		newID:      newID,
	}
}

func (s *Service) UpdateAttendance(ctx context.Context, actorRole iam.Role, userID, courseCode string, attended, total int) error {
	if actorRole != iam.RoleTeacher && actorRole != iam.RoleAdmin {
		return errs.New(errs.ErrForbidden, "STAFF_REQUIRED", "Staff access required.")
	}
	if _, err := s.userRead.FindByRole(ctx, userID, iam.RoleStudent); err != nil {
		return errs.New(errs.ErrNotFound, "STUDENT_NOT_FOUND", "Student not found.")
	}
	if _, err := s.catRead.GetCourse(ctx, courseCode); err != nil {
		return err
	}
	if total < 0 || total > 1000 || attended < 0 || attended > total {
		return errs.New(errs.ErrValidation, "INVALID_ATTENDANCE", "Attended classes must be between 0 and total (max 1000).")
	}
	return s.attWrite.Upsert(ctx, learning.AttendanceSummary{
		UserID:     userID,
		CourseCode: courseCode,
		Attended:   attended,
		Total:      total,
	})
}

func (s *Service) GradeSubmission(ctx context.Context, actorRole iam.Role, submissionID, score, feedback string) error {
	if actorRole != iam.RoleTeacher && actorRole != iam.RoleAdmin {
		return errs.New(errs.ErrForbidden, "STAFF_REQUIRED", "Staff access required.")
	}
	trimmed := strings.TrimSpace(score)
	if !scoreRe.MatchString(trimmed) {
		return errs.New(errs.ErrValidation, "INVALID_SCORE", "Use earned / maximum, e.g. 18/20.")
	}
	parts := strings.Split(trimmed, "/")
	earned, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	maxScore, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil || maxScore <= 0 || earned > maxScore {
		return errs.New(errs.ErrValidation, "INVALID_SCORE_RANGE", "Score must be between zero and maximum.")
	}
	if _, err := s.subRepo.GetByID(ctx, submissionID); err != nil {
		return err
	}
	if len(feedback) > 5000 {
		feedback = feedback[:5000]
	}
	return s.subRepo.Grade(ctx, submissionID, trimmed, feedback)
}

func (s *Service) CreateCourse(ctx context.Context, actorRole iam.Role, semester int, code, name, instructor string, credits int) error {
	if actorRole != iam.RoleTeacher && actorRole != iam.RoleAdmin {
		return errs.New(errs.ErrForbidden, "STAFF_REQUIRED", "Staff access required.")
	}
	if semester < 1 || semester > 3 {
		return errs.New(errs.ErrValidation, "INVALID_SEMESTER", "Invalid semester.")
	}
	code = strings.TrimSpace(code)
	if !courseCodeRe.MatchString(code) || len(code) > 30 {
		return errs.New(errs.ErrValidation, "INVALID_COURSE_CODE", "Use letters, numbers and hyphens for course identifier.")
	}
	if credits < 1 || credits > 10 {
		return errs.New(errs.ErrValidation, "INVALID_CREDITS", "Credits must be between 1 and 10.")
	}
	return s.catWrite.CreateCourse(ctx, &catalog.Course{
		Code:       code,
		Semester:   semester,
		Name:       strings.TrimSpace(name),
		Instructor: strings.TrimSpace(instructor),
		Credits:    credits,
		Next:       "New course",
		Att:        "0%",
		Skips:      "0",
	})
}

func (s *Service) AddScheduleSlot(ctx context.Context, actorRole iam.Role, dateStr, from, to, courseCode, room, topic string) error {
	if actorRole != iam.RoleTeacher && actorRole != iam.RoleAdmin {
		return errs.New(errs.ErrForbidden, "STAFF_REQUIRED", "Staff access required.")
	}
	if _, err := s.catRead.GetCourse(ctx, courseCode); err != nil {
		return err
	}
	if !isoDateRe.MatchString(dateStr) {
		return errs.New(errs.ErrValidation, "INVALID_DATE", "Invalid date.")
	}
	parsedDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return errs.New(errs.ErrValidation, "INVALID_DATE", "Invalid date.")
	}
	if !hhmmRe.MatchString(from) || !hhmmRe.MatchString(to) || to <= from {
		return errs.New(errs.ErrValidation, "INVALID_TIME", "End time must follow start time.")
	}
	weekdayOffset := (int(parsedDate.Weekday()) + 6) % 7
	weekStart := parsedDate.AddDate(0, 0, -weekdayOffset).Format("2006-01-02")

	err = s.schedRepo.AddSlot(ctx, catalog.ScheduleClass{
		ID:       s.newID(),
		Week:     weekStart,
		SlotDate: dateStr,
		From:     from,
		To:       to,
		Code:     courseCode,
		Room:     strings.TrimSpace(room),
		Topic:    strings.TrimSpace(topic),
		Type:     "Lecture",
	})
	if errors.Is(err, errs.ErrScheduleOverlap) {
		return errs.New(errs.ErrScheduleOverlap, "SCHEDULE_OVERLAP", "This session overlaps an existing class.")
	}
	return err
}
