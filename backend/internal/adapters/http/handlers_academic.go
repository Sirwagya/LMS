package httpadapter

import (
	"errors"
	"net/http"
	"time"

	"vedam/lms/internal/domain/academic"
	"vedam/lms/internal/domain/envelope"
	"vedam/lms/internal/domain/errs"
)

func academicJSON(w http.ResponseWriter, data any) {
	writeJSON(w, 200, envelope.Success(data, 200, time.Now()))
}
func paginate[T any](r *http.Request, list []T) (envelope.Page[T], error) {
	page, size, err := pageParams(r)
	if err != nil {
		return envelope.Page[T]{}, err
	}
	start := min(page*size, len(list))
	end := min(start+size, len(list))
	return envelope.NewPage(list[start:end], page, size, int64(len(list))), nil
}
func pageParams(r *http.Request) (int, int, error) {
	page, err := queryNumber(r, "page", 0, 1000000, 0)
	if err != nil {
		return 0, 0, err
	}
	size, err := queryNumber(r, "size", 1, 1000, 100)
	return int(page), int(size), err
}
func (a *api) programs(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	cpr, err := a.Academic.CPRForUser(ctx, actor(r).UserID)
	if err != nil && !errors.Is(err, errs.ErrNotFound) {
		return err
	}
	var list []academic.ProgramEnrolmentItem
	if cpr != 0 {
		list, err = a.Academic.ListProgramEnrolments(ctx, cpr)
		if err != nil {
			return err
		}
	}
	page, err := paginate(r, list)
	if err != nil {
		return err
	}
	academicJSON(w, page)
	return nil
}
func (a *api) semesters(w http.ResponseWriter, r *http.Request) error {
	program, err := queryNumber(r, "programId", 1, 1<<31-1, 0)
	if err != nil {
		return err
	}
	if program == 0 {
		return invalid("programId is required.")
	}
	list, err := a.Academic.ListSemestersByProgram(r.Context(), int(program))
	if err != nil {
		return err
	}
	academicJSON(w, items(list))
	return nil
}
func (a *api) offerings(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	semester, err := queryNumber(r, "semesterId", 1, 1<<31-1, 0)
	if err != nil {
		return err
	}
	cpr, err := a.Academic.CPRForUser(ctx, actor(r).UserID)
	if err != nil && !errors.Is(err, errs.ErrNotFound) {
		return err
	}
	var filter *int
	if semester != 0 {
		n := int(semester)
		filter = &n
	}
	var list []academic.CourseOfferingEnrolmentItem
	if cpr != 0 {
		list, err = a.Academic.ListCourseOfferingEnrolments(ctx, cpr, filter)
		if err != nil {
			return err
		}
	}
	page, err := paginate(r, list)
	if err != nil {
		return err
	}
	academicJSON(w, page)
	return nil
}
func (a *api) classSessions(w http.ResponseWriter, r *http.Request) error {
	page, size, err := pageParams(r)
	if err != nil {
		return err
	}
	semester, err := queryNumber(r, "semesterId", 1, 1<<31-1, 0)
	if err != nil {
		return err
	}
	course, err := queryNumber(r, "courseOfferingId", 1, 1<<63-1, 0)
	if err != nil {
		return err
	}
	filter := academic.ClassSessionFilter{Page: page, Size: size}
	if semester != 0 {
		n := int(semester)
		filter.SemesterID = &n
	}
	if course != 0 {
		filter.CourseOfferingID = &course
	}
	list, total, err := a.Academic.ListClassSessions(r.Context(), filter)
	if err != nil {
		return err
	}
	academicJSON(w, envelope.NewPage(list, page, size, total))
	return nil
}
func (a *api) face(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	cpr, err := a.Academic.CPRForUser(ctx, actor(r).UserID)
	if err != nil {
		return missing(err, "Face biometric record not found.")
	}
	record, err := a.Academic.GetFaceBiometrics(ctx, cpr)
	if err != nil {
		return missing(err, "Face biometric record not found.")
	}
	w.Header().Set("Pragma", "no-cache")
	academicJSON(w, record)
	return nil
}
