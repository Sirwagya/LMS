package memory

import (
	"context"
	"slices"

	"vedam/lms/internal/domain/academic"
	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/errs"
)

type AcademicRepo struct{ s *Store }

func (s *Store) Academic() *AcademicRepo { return &AcademicRepo{s: s} }

func (r *AcademicRepo) UpsertCampus(ctx context.Context, campus academic.Campus) error {
	defer r.s.lockWrite(ctx)()
	r.s.active(ctx).campuses[campus.ID] = campus
	return nil
}

func (r *AcademicRepo) UpsertProgram(ctx context.Context, program academic.Program) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, ok := st.campuses[program.CampusID]; !ok {
		return errs.ErrNotFound
	}
	st.programs[program.ID] = program
	return nil
}

func (r *AcademicRepo) UpsertCampusPersonRole(ctx context.Context, role academic.CampusPersonRole) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, ok := st.users[role.UserID]; !ok {
		return errs.ErrNotFound
	}
	if _, ok := st.campuses[role.CampusID]; !ok {
		return errs.ErrNotFound
	}
	st.cprs[role.CPRID] = role
	return nil
}

func (r *AcademicRepo) UpsertProgramEnrolment(ctx context.Context, id int64, programID int, cprID int64, enrolmentDate int64) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, ok := st.programs[programID]; !ok {
		return errs.ErrNotFound
	}
	if _, ok := st.cprs[cprID]; !ok {
		return errs.ErrNotFound
	}
	st.progEnrols[id] = storedProgramEnrolment{
		id:            id,
		programID:     programID,
		cprID:         cprID,
		enrolmentDate: enrolmentDate,
	}
	return nil
}

func (r *AcademicRepo) UpsertSemester(ctx context.Context, sem academic.SemesterRecord) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	st.semRecords[sem.ID] = sem
	st.semesters[sem.ID] = catalog.Semester{
		Number:    sem.ID,
		Label:     sem.Title,
		IsCurrent: sem.ID == 3,
	}
	return nil
}

func (r *AcademicRepo) UpsertCourseOffering(ctx context.Context, off academic.CourseOfferingSpec) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, ok := st.courses[off.CourseCode]; !ok {
		return errs.ErrNotFound
	}
	st.offerings[off.ID] = off
	return nil
}

func (r *AcademicRepo) UpsertCourseOfferingEnrolment(ctx context.Context, id, courseOfferingID, cprID, enrolmentDate int64, present, absent, late, leave, excused, awaiting, total int) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, ok := st.offerings[courseOfferingID]; !ok {
		return errs.ErrNotFound
	}
	if _, ok := st.cprs[cprID]; !ok {
		return errs.ErrNotFound
	}
	st.offeringEnrols[id] = storedCourseOfferingEnrolment{
		id:               id,
		courseOfferingID: courseOfferingID,
		cprID:            cprID,
		enrolmentDate:    enrolmentDate,
		presentCount:     present,
		absentCount:      absent,
		lateCount:        late,
		leaveCount:       leave,
		excusedCount:     excused,
		awaitingCount:    awaiting,
		totalCount:       total,
	}
	return nil
}

func (r *AcademicRepo) UpsertClassSession(ctx context.Context, session academic.ClassSessionItem) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, ok := st.offerings[session.CourseOfferingID]; !ok {
		return errs.ErrNotFound
	}
	st.classSessions[session.ID] = session
	return nil
}

func (r *AcademicRepo) ListProgramEnrolments(ctx context.Context, cprID int64) ([]academic.ProgramEnrolmentItem, error) {
	defer r.s.lockRead(ctx)()
	st := r.s.active(ctx)
	cpr, ok := st.cprs[cprID]
	if !ok {
		return nil, nil
	}
	campus := st.campuses[cpr.CampusID]
	var out []academic.ProgramEnrolmentItem
	for _, pe := range st.progEnrols {
		if pe.cprID != cprID {
			continue
		}
		prog := st.programs[pe.programID]
		out = append(out, academic.ProgramEnrolmentItem{
			ProgramEnrolmentID: pe.id,
			ProgramID:          pe.programID,
			EnrolmentDate:      pe.enrolmentDate,
			CPRID:              cpr.CPRID,
			CampusID:           cpr.CampusID,
			OrganizationID:     campus.OrganizationID,
			PersonName:         cpr.PersonName,
			Email:              cpr.Email,
			ProgramTitle:       prog.Title,
			ProgramLabel:       prog.Label,
		})
	}
	slices.SortFunc(out, func(a, b academic.ProgramEnrolmentItem) int {
		return int(a.ProgramEnrolmentID - b.ProgramEnrolmentID)
	})
	return out, nil
}

func (r *AcademicRepo) ListSemestersByProgram(ctx context.Context, programID int) ([]academic.SemesterRecord, error) {
	defer r.s.lockRead(ctx)()
	var out []academic.SemesterRecord
	for _, s := range r.s.active(ctx).semRecords {
		if s.ProgramID == programID {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b academic.SemesterRecord) int {
		if a.StartDate != b.StartDate {
			if a.StartDate < b.StartDate {
				return -1
			}
			return 1
		}
		return a.ID - b.ID
	})
	return out, nil
}

func (r *AcademicRepo) ListCourseOfferingEnrolments(ctx context.Context, cprID int64, semesterID *int) ([]academic.CourseOfferingEnrolmentItem, error) {
	defer r.s.lockRead(ctx)()
	st := r.s.active(ctx)
	cpr, ok := st.cprs[cprID]
	if !ok {
		return nil, nil
	}
	var out []academic.CourseOfferingEnrolmentItem
	for _, coe := range st.offeringEnrols {
		if coe.cprID != cprID {
			continue
		}
		off, ok := st.offerings[coe.courseOfferingID]
		if !ok {
			continue
		}
		if semesterID != nil && off.SemesterID != *semesterID {
			continue
		}
		out = append(out, academic.CourseOfferingEnrolmentItem{
			CourseOfferingEnrolmentID: coe.id,
			CourseOfferingID:          off.ID,
			Label:                     off.Label,
			CourseID:                  off.CourseID,
			SemesterID:                off.SemesterID,
			CampusID:                  off.CampusID,
			Title:                     off.Title,
			Batch:                     off.Batch,
			ProgramID:                 off.ProgramID,
			Credits:                   off.Credits,
			Description:               off.Description,
			IconURL:                   off.IconURL,
			BgColor:                   off.BgColor,
			EnrolmentCount:            off.EnrolmentCount,
			ClassSessionCount:         off.ClassSessionCount,
			CampusPersonRoleID:        cpr.CPRID,
			Name:                      cpr.PersonName,
			Email:                     cpr.Email,
			EnrolmentDate:             coe.enrolmentDate,
			PresentCount:              coe.presentCount,
			AbsentCount:               coe.absentCount,
			LateCount:                 coe.lateCount,
			LeaveCount:                coe.leaveCount,
			ExcusedCount:              coe.excusedCount,
			AwaitingCount:             coe.awaitingCount,
			TotalCount:                coe.totalCount,
			CourseRequiredAttendance:  off.CourseRequiredAttendance,
		})
	}
	slices.SortFunc(out, func(a, b academic.CourseOfferingEnrolmentItem) int {
		return int(b.CourseOfferingEnrolmentID - a.CourseOfferingEnrolmentID)
	})
	return out, nil
}

func (r *AcademicRepo) ListClassSessions(ctx context.Context, filter academic.ClassSessionFilter) ([]academic.ClassSessionItem, int64, error) {
	defer r.s.lockRead(ctx)()
	var matched []academic.ClassSessionItem
	for _, cs := range r.s.active(ctx).classSessions {
		if filter.SemesterID != nil && cs.SemesterID != *filter.SemesterID {
			continue
		}
		if filter.CourseOfferingID != nil && cs.CourseOfferingID != *filter.CourseOfferingID {
			continue
		}
		matched = append(matched, cs)
	}
	slices.SortFunc(matched, func(a, b academic.ClassSessionItem) int {
		return int(a.ID - b.ID)
	})
	total := int64(len(matched))
	size := filter.Size
	if size <= 0 {
		size = 100
	}
	offset := filter.Page * size
	if offset >= len(matched) {
		return []academic.ClassSessionItem{}, total, nil
	}
	end := offset + size
	if end > len(matched) {
		end = len(matched)
	}
	return slices.Clone(matched[offset:end]), total, nil
}

func (r *AcademicRepo) GetFaceBiometrics(ctx context.Context, personID int64) (*academic.FaceBiometricRecord, error) {
	defer r.s.lockRead(ctx)()
	rec, ok := r.s.active(ctx).biometrics[personID]
	if !ok {
		return nil, errs.ErrNotFound
	}
	cp := rec
	cp.Embedding = slices.Clone(rec.Embedding)
	return &cp, nil
}

func (r *AcademicRepo) UpsertFaceBiometrics(ctx context.Context, rec academic.FaceBiometricRecord) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, ok := st.cprs[rec.PersonID]; !ok {
		return errs.ErrNotFound
	}
	cp := rec
	cp.Embedding = slices.Clone(rec.Embedding)
	st.biometrics[rec.PersonID] = cp
	return nil
}
