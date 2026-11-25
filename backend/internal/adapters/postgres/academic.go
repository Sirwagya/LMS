package postgres

import (
	"context"
	"fmt"
	"strings"

	"vedam/lms/internal/domain/academic"
)

type AcademicRepo struct{ tx *TxManager }

func NewAcademicRepo(tx *TxManager) *AcademicRepo { return &AcademicRepo{tx: tx} }

func (r *AcademicRepo) UpsertCampus(ctx context.Context, c academic.Campus) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO campuses (id, name, organization_id) VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, organization_id = EXCLUDED.organization_id
	`, c.ID, c.Name, c.OrganizationID)
	return mapPGError(err)
}

func (r *AcademicRepo) UpsertProgram(ctx context.Context, p academic.Program) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO programs (id, campus_id, title, label) VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET campus_id = EXCLUDED.campus_id, title = EXCLUDED.title, label = EXCLUDED.label
	`, p.ID, p.CampusID, p.Title, p.Label)
	return mapPGError(err)
}

func (r *AcademicRepo) UpsertCampusPersonRole(ctx context.Context, role academic.CampusPersonRole) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO campus_person_roles (cpr_id, user_id, campus_id, person_name, email)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (cpr_id) DO UPDATE
		SET user_id = EXCLUDED.user_id, campus_id = EXCLUDED.campus_id, person_name = EXCLUDED.person_name, email = EXCLUDED.email
	`, role.CPRID, role.UserID, role.CampusID, role.PersonName, role.Email)
	return mapPGError(err)
}

func (r *AcademicRepo) UpsertProgramEnrolment(ctx context.Context, id int64, programID int, cprID int64, enrolmentDate int64) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO program_enrolments (id, program_id, cpr_id, enrolment_date)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET program_id = EXCLUDED.program_id, cpr_id = EXCLUDED.cpr_id, enrolment_date = EXCLUDED.enrolment_date
	`, id, programID, cprID, enrolmentDate)
	return mapPGError(err)
}

func (r *AcademicRepo) UpsertSemester(ctx context.Context, sem academic.SemesterRecord) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO semesters (number, label, code_label, campus_id, program_id, program_title, program_label, start_epoch, end_epoch, required_attendance, is_current)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (number) DO UPDATE
		SET label = EXCLUDED.label,
		    code_label = EXCLUDED.code_label,
		    campus_id = EXCLUDED.campus_id,
		    program_id = EXCLUDED.program_id,
		    program_title = EXCLUDED.program_title,
		    program_label = EXCLUDED.program_label,
		    start_epoch = EXCLUDED.start_epoch,
		    end_epoch = EXCLUDED.end_epoch,
		    required_attendance = EXCLUDED.required_attendance,
		    is_current = EXCLUDED.is_current
	`, sem.ID, sem.Title, sem.Label, sem.CampusID, sem.ProgramID, sem.ProgramTitle, sem.ProgramLabel, sem.StartDate, sem.EndDate, sem.RequiredAttendance, sem.ID == 3)
	return mapPGError(err)
}

func (r *AcademicRepo) UpsertCourseOffering(ctx context.Context, off academic.CourseOfferingSpec) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO course_offerings (
			id, course_id, course_code, semester_id, campus_id, program_id, label, title, batch,
			credits, description, icon_url, bg_color, enrolment_count, class_session_count, required_attendance
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (id) DO UPDATE
		SET course_id = EXCLUDED.course_id,
		    course_code = EXCLUDED.course_code,
		    semester_id = EXCLUDED.semester_id,
		    label = EXCLUDED.label,
		    title = EXCLUDED.title,
		    batch = EXCLUDED.batch,
		    credits = EXCLUDED.credits,
		    description = EXCLUDED.description,
		    icon_url = EXCLUDED.icon_url,
		    bg_color = EXCLUDED.bg_color,
		    enrolment_count = EXCLUDED.enrolment_count,
		    class_session_count = EXCLUDED.class_session_count,
		    required_attendance = EXCLUDED.required_attendance
	`, off.ID, off.CourseID, off.CourseCode, off.SemesterID, off.CampusID, off.ProgramID, off.Label, off.Title, off.Batch,
		off.Credits, off.Description, off.IconURL, off.BgColor, off.EnrolmentCount, off.ClassSessionCount, off.CourseRequiredAttendance)
	return mapPGError(err)
}

func (r *AcademicRepo) UpsertCourseOfferingEnrolment(ctx context.Context, id, courseOfferingID, cprID, enrolmentDate int64, present, absent, late, leave, excused, awaiting, total int) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO course_offering_enrolments (
			id, course_offering_id, cpr_id, enrolment_date,
			present_count, absent_count, late_count, leave_count, excused_count, awaiting_count, total_count
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO UPDATE
		SET present_count = EXCLUDED.present_count,
		    absent_count = EXCLUDED.absent_count,
		    late_count = EXCLUDED.late_count,
		    leave_count = EXCLUDED.leave_count,
		    excused_count = EXCLUDED.excused_count,
		    awaiting_count = EXCLUDED.awaiting_count,
		    total_count = EXCLUDED.total_count
	`, id, courseOfferingID, cprID, enrolmentDate, present, absent, late, leave, excused, awaiting, total)
	return mapPGError(err)
}

func (r *AcademicRepo) UpsertClassSession(ctx context.Context, s academic.ClassSessionItem) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO class_sessions (
			id, course_offering_id, program_id, semester_id, cpr_id,
			start_time, end_time, title, topic, label, room, cs_created_at, cs_updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (id) DO UPDATE
		SET title = EXCLUDED.title,
		    topic = EXCLUDED.topic,
		    label = EXCLUDED.label,
		    room = EXCLUDED.room,
		    start_time = EXCLUDED.start_time,
		    end_time = EXCLUDED.end_time,
		    cs_updated_at = EXCLUDED.cs_updated_at
	`, s.ID, s.CourseOfferingID, s.ProgramID, s.SemesterID, s.CPRID,
		s.StartTime, s.EndTime, s.Title, s.Topic, s.Label, s.Room, s.CSCreatedAt, s.CSUpdatedAt)
	return mapPGError(err)
}

func (r *AcademicRepo) ListProgramEnrolments(ctx context.Context, cprID int64) ([]academic.ProgramEnrolmentItem, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT pe.id, pe.program_id, pe.enrolment_date, cpr.cpr_id, cpr.campus_id,
		       c.organization_id, cpr.person_name, cpr.email, p.title, p.label
		FROM program_enrolments pe
		JOIN campus_person_roles cpr ON cpr.cpr_id = pe.cpr_id
		JOIN campuses c ON c.id = cpr.campus_id
		JOIN programs p ON p.id = pe.program_id
		WHERE pe.cpr_id = $1
		ORDER BY pe.id ASC
	`, cprID)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []academic.ProgramEnrolmentItem
	for rows.Next() {
		var item academic.ProgramEnrolmentItem
		if err := rows.Scan(
			&item.ProgramEnrolmentID, &item.ProgramID, &item.EnrolmentDate, &item.CPRID,
			&item.CampusID, &item.OrganizationID, &item.PersonName, &item.Email,
			&item.ProgramTitle, &item.ProgramLabel,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *AcademicRepo) ListSemestersByProgram(ctx context.Context, programID int) ([]academic.SemesterRecord, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT number, campus_id, program_id, label, code_label, start_epoch, end_epoch,
		       program_title, program_label, required_attendance
		FROM semesters WHERE program_id = $1 ORDER BY start_epoch ASC, number ASC
	`, programID)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []academic.SemesterRecord
	for rows.Next() {
		var s academic.SemesterRecord
		if err := rows.Scan(
			&s.ID, &s.CampusID, &s.ProgramID, &s.Title, &s.Label, &s.StartDate, &s.EndDate,
			&s.ProgramTitle, &s.ProgramLabel, &s.RequiredAttendance,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *AcademicRepo) ListCourseOfferingEnrolments(ctx context.Context, cprID int64, semesterID *int) ([]academic.CourseOfferingEnrolmentItem, error) {
	query := `
		SELECT coe.id, co.id, co.label, co.course_id, co.semester_id, co.campus_id, co.title, co.batch,
		       co.program_id, co.credits, co.description, co.icon_url, co.bg_color, co.enrolment_count,
		       co.class_session_count, cpr.cpr_id, cpr.person_name, cpr.email, coe.enrolment_date,
		       coe.present_count, coe.absent_count, coe.late_count, coe.leave_count,
		       coe.excused_count, coe.awaiting_count, coe.total_count, co.required_attendance
		FROM course_offering_enrolments coe
		JOIN course_offerings co ON co.id = coe.course_offering_id
		JOIN campus_person_roles cpr ON cpr.cpr_id = coe.cpr_id
		WHERE coe.cpr_id = $1
	`
	args := []any{cprID}
	if semesterID != nil {
		query += ` AND co.semester_id = $2`
		args = append(args, *semesterID)
	}
	query += ` ORDER BY coe.id DESC`

	rows, err := r.tx.Querier(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []academic.CourseOfferingEnrolmentItem
	for rows.Next() {
		var item academic.CourseOfferingEnrolmentItem
		if err := rows.Scan(
			&item.CourseOfferingEnrolmentID, &item.CourseOfferingID, &item.Label, &item.CourseID,
			&item.SemesterID, &item.CampusID, &item.Title, &item.Batch, &item.ProgramID, &item.Credits,
			&item.Description, &item.IconURL, &item.BgColor, &item.EnrolmentCount, &item.ClassSessionCount,
			&item.CampusPersonRoleID, &item.Name, &item.Email, &item.EnrolmentDate,
			&item.PresentCount, &item.AbsentCount, &item.LateCount, &item.LeaveCount,
			&item.ExcusedCount, &item.AwaitingCount, &item.TotalCount, &item.CourseRequiredAttendance,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *AcademicRepo) ListClassSessions(ctx context.Context, filter academic.ClassSessionFilter) ([]academic.ClassSessionItem, int64, error) {
	var conds []string
	var args []any
	idx := 1
	if filter.SemesterID != nil {
		conds = append(conds, fmt.Sprintf("semester_id = $%d", idx))
		args = append(args, *filter.SemesterID)
		idx++
	}
	if filter.CourseOfferingID != nil {
		conds = append(conds, fmt.Sprintf("course_offering_id = $%d", idx))
		args = append(args, *filter.CourseOfferingID)
		idx++
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int64
	if err := r.tx.Querier(ctx).QueryRow(ctx, "SELECT COUNT(*) FROM class_sessions "+where, args...).Scan(&total); err != nil {
		return nil, 0, mapPGError(err)
	}

	size := filter.Size
	if size <= 0 {
		size = 100
	}
	offset := filter.Page * size
	query := fmt.Sprintf(`
		SELECT id, course_offering_id, start_time, end_time, title, room,
		       program_id, semester_id, cpr_id, cs_created_at, cs_updated_at, label, topic
		FROM class_sessions %s
		ORDER BY id ASC LIMIT $%d OFFSET $%d
	`, where, idx, idx+1)
	args = append(args, size, offset)

	rows, err := r.tx.Querier(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, 0, mapPGError(err)
	}
	defer rows.Close()
	var out []academic.ClassSessionItem
	for rows.Next() {
		var cs academic.ClassSessionItem
		if err := rows.Scan(
			&cs.ID, &cs.CourseOfferingID, &cs.StartTime, &cs.EndTime, &cs.Title, &cs.Room,
			&cs.ProgramID, &cs.SemesterID, &cs.CPRID, &cs.CSCreatedAt, &cs.CSUpdatedAt, &cs.Label, &cs.Topic,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, cs)
	}
	return out, total, rows.Err()
}

func (r *AcademicRepo) GetFaceBiometrics(ctx context.Context, personID int64) (*academic.FaceBiometricRecord, error) {
	var rec academic.FaceBiometricRecord
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT has_face_data, face_json, email, person_id, embedding
		FROM user_face_biometrics WHERE person_id = $1
	`, personID).Scan(&rec.HasFaceData, &rec.FaceJSON, &rec.Email, &rec.PersonID, &rec.Embedding)
	if err != nil {
		return nil, mapPGError(err)
	}
	return &rec, nil
}

func (r *AcademicRepo) UpsertFaceBiometrics(ctx context.Context, rec academic.FaceBiometricRecord) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO user_face_biometrics (person_id, email, has_face_data, face_json, embedding, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (person_id) DO UPDATE
		SET email = EXCLUDED.email,
		    has_face_data = EXCLUDED.has_face_data,
		    face_json = EXCLUDED.face_json,
		    embedding = EXCLUDED.embedding,
		    updated_at = NOW()
	`, rec.PersonID, rec.Email, rec.HasFaceData, rec.FaceJSON, rec.Embedding)
	return mapPGError(err)
}

func (r *AcademicRepo) CPRForUser(ctx context.Context, userID string) (int64, error) {
	var id int64
	err := r.tx.Querier(ctx).QueryRow(ctx, `SELECT cpr_id FROM campus_person_roles WHERE user_id=$1`, userID).Scan(&id)
	return id, mapPGError(err)
}
