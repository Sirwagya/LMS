package seed

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"vedam/lms/internal/domain/academic"
	"vedam/lms/internal/domain/assessment"
	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/finance"
	"vedam/lms/internal/domain/iam"
)

//go:embed seed_data.json
var seedJSON []byte

//go:embed vedam_homepage_seed.json
var vedamHomepageJSON []byte

type seedCodingSample struct {
	ID      string                       `json:"id"`
	Title   string                       `json:"title"`
	DueDate string                       `json:"dueDate"`
	Problem assessment.CodingProblemSpec `json:"problem"`
}

type seedPayload struct {
	SemesterSubjects  map[string][]catalog.Course              `json:"semesterSubjects"`
	LecturesBySubject map[string][]catalog.Lecture             `json:"lecturesBySubject"`
	ContestSummary    map[string]assessment.ContestSummaryItem `json:"contestSummary"`
	RecentContests    []assessment.ContestRecord               `json:"recentContests"`
	DemoWeek          catalog.WeekSchedule                     `json:"demoWeek"`
	NoticesData       []catalog.Notice                         `json:"noticesData"`
	DrawerNotices     []catalog.Notice                         `json:"drawerNotices"`
	FeeRecords        []finance.FeeInvoice                     `json:"feeRecords"`
	CodingSamples     []seedCodingSample                       `json:"codingSamples"`
}

type vedamOfferingEnrolmentSeed struct {
	ID            int64 `json:"id"`
	EnrolmentDate int64 `json:"enrolmentDate"`
	PresentCount  int   `json:"presentCount"`
	AbsentCount   int   `json:"absentCount"`
	LateCount     int   `json:"lateCount"`
	LeaveCount    int   `json:"leaveCount"`
	ExcusedCount  int   `json:"excusedCount"`
	AwaitingCount int   `json:"awaitingCount"`
	TotalCount    int   `json:"totalCount"`
}

type vedamOfferingSeed struct {
	ID                       int64                       `json:"id"`
	CourseID                 int64                       `json:"courseId"`
	CourseCode               string                      `json:"courseCode"`
	SemesterID               int                         `json:"semesterId"`
	Label                    string                      `json:"label"`
	Title                    string                      `json:"title"`
	Batch                    string                      `json:"batch"`
	Credits                  int                         `json:"credits"`
	Description              string                      `json:"description"`
	IconURL                  string                      `json:"iconUrl"`
	BgColor                  string                      `json:"bgColor"`
	EnrolmentCount           int                         `json:"enrolmentCount"`
	ClassSessionCount        int                         `json:"classSessionCount"`
	CourseRequiredAttendance int                         `json:"courseRequiredAttendance"`
	Enrolment                *vedamOfferingEnrolmentSeed `json:"enrolment"`
}

type vedamHomepagePayload struct {
	Campus           academic.Campus               `json:"campus"`
	Program          academic.Program              `json:"program"`
	ProgramEnrolment academic.ProgramEnrolmentItem `json:"programEnrolment"`
	Semesters        []academic.SemesterRecord     `json:"semesters"`
	FaceBiometrics   academic.FaceBiometricRecord  `json:"faceBiometrics"`
	ExtraCourses     []catalog.Course              `json:"extraCourses"`
	CourseOfferings  []vedamOfferingSeed           `json:"courseOfferings"`
	ClassSessions    []academic.ClassSessionItem   `json:"classSessions"`
}

func Run(ctx context.Context, pool *pgxpool.Pool, hasher iam.PasswordHasher) error {
	var data seedPayload
	if err := json.Unmarshal(seedJSON, &data); err != nil {
		return fmt.Errorf("parse embedded seed_data.json: %w", err)
	}
	var homepage vedamHomepagePayload
	if err := json.Unmarshal(vedamHomepageJSON, &homepage); err != nil {
		return fmt.Errorf("parse embedded vedam_homepage_seed.json: %w", err)
	}

	var userCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		return fmt.Errorf("check users: %w", err)
	}
	if userCount > 0 {
		return nil // Already seeded idempotently
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Campus & Program
	if _, err := tx.Exec(ctx, `
		INSERT INTO campuses (id, name, organization_id) VALUES ($1, $2, $3)
		ON CONFLICT (id) DO NOTHING
	`, homepage.Campus.ID, homepage.Campus.Name, homepage.Campus.OrganizationID); err != nil {
		return fmt.Errorf("seed campus: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO programs (id, campus_id, title, label) VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO NOTHING
	`, homepage.Program.ID, homepage.Program.CampusID, homepage.Program.Title, homepage.Program.Label); err != nil {
		return fmt.Errorf("seed program: %w", err)
	}

	// 2. Semesters (1, 2, 6 Bootcamp, 3)
	for _, s := range homepage.Semesters {
		if _, err := tx.Exec(ctx, `
			INSERT INTO semesters (number, label, code_label, campus_id, program_id, program_title, program_label, start_epoch, end_epoch, required_attendance, is_current)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (number) DO UPDATE
			SET label = EXCLUDED.label, code_label = EXCLUDED.code_label, campus_id = EXCLUDED.campus_id,
			    program_id = EXCLUDED.program_id, program_title = EXCLUDED.program_title, program_label = EXCLUDED.program_label,
			    start_epoch = EXCLUDED.start_epoch, end_epoch = EXCLUDED.end_epoch, required_attendance = EXCLUDED.required_attendance,
			    is_current = EXCLUDED.is_current
		`, s.ID, s.Title, s.Label, s.CampusID, s.ProgramID, s.ProgramTitle, s.ProgramLabel, s.StartDate, s.EndDate, s.RequiredAttendance, s.ID == 3); err != nil {
			return fmt.Errorf("seed semester %d: %w", s.ID, err)
		}
	}

	// 3. Courses (existing seed + Bootcamp/extra courses)
	allSubjects := make([]catalog.Course, 0)
	for semKey, courses := range data.SemesterSubjects {
		semNum, err := strconv.Atoi(semKey)
		if err != nil {
			return fmt.Errorf("invalid semester key %s: %w", semKey, err)
		}
		for _, c := range courses {
			c.Semester = semNum
			allSubjects = append(allSubjects, c)
			if _, err := tx.Exec(ctx, `
				INSERT INTO courses (code, semester_number, name, instructor, credits, next_topic, default_att_pct, skips_label)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (code) DO NOTHING
			`, c.Code, semNum, c.Name, c.Instructor, c.Credits, c.Next, c.Att, c.Skips); err != nil {
				return fmt.Errorf("seed course %s: %w", c.Code, err)
			}
		}
	}
	for _, c := range homepage.ExtraCourses {
		if _, err := tx.Exec(ctx, `
			INSERT INTO courses (code, semester_number, name, instructor, credits, next_topic, default_att_pct, skips_label)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (code) DO NOTHING
		`, c.Code, c.Semester, c.Name, c.Instructor, c.Credits, c.Next, c.Att, c.Skips); err != nil {
			return fmt.Errorf("seed extra course %s: %w", c.Code, err)
		}
	}

	// 4. Lectures, downloadable notes resources & written assignments
	firstADA := int64(0)
	for courseCode, lectures := range data.LecturesBySubject {
		for idx, l := range lectures {
			if courseCode == "E0460A" && idx == 0 {
				firstADA = l.ID
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO lectures (id, course_code, lecture_code, lecture_date, month_label, unit_number, title, raw_title, default_att, summary, slides, duration, recording_url, sort_order)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NULL, $13)
				ON CONFLICT (id) DO NOTHING
			`, l.ID, courseCode, l.Code, l.Date, l.Month, l.Unit, l.Title, l.RawTitle, l.Attendance, l.Summary, l.Slides, l.Duration, idx); err != nil {
				return fmt.Errorf("seed lecture %d: %w", l.ID, err)
			}
			resTitle := fmt.Sprintf("%s — notes.txt", l.Title)
			resURL := fmt.Sprintf("/api/notes/%d", l.ID)
			if _, err := tx.Exec(ctx, `
				INSERT INTO lecture_resources (id, lecture_id, title, resource_type, url, sort_order)
				VALUES ($1, $2, $3, 'Text notes', $4, 0)
			`, uuid.NewString(), l.ID, resTitle, resURL); err != nil {
				return fmt.Errorf("seed resource for lecture %d: %w", l.ID, err)
			}
			for _, a := range l.Assignments {
				if _, err := tx.Exec(ctx, `
					INSERT INTO assignments (id, lecture_id, course_code, kind, title, due_date)
					VALUES ($1, $2, $3, 'written', $4, $5)
					ON CONFLICT (id) DO NOTHING
				`, a.ID, l.ID, courseCode, a.Title, a.DueDate); err != nil {
					return fmt.Errorf("seed assignment %s: %w", a.ID, err)
				}
			}
		}
	}
	if _, err := tx.Exec(ctx, `SELECT setval(pg_get_serial_sequence('lectures', 'id'), COALESCE((SELECT MAX(id) FROM lectures), 1))`); err != nil {
		return fmt.Errorf("advance lectures sequence: %w", err)
	}

	// 5. Coding Sample Problems (attached to first E0460A lecture)
	if firstADA > 0 {
		for _, sample := range data.CodingSamples {
			starterBytes, _ := json.Marshal(sample.Problem.StarterCode)
			if _, err := tx.Exec(ctx, `
				INSERT INTO assignments (id, lecture_id, course_code, kind, title, due_date)
				VALUES ($1, $2, 'E0460A', 'coding', $3, $4)
				ON CONFLICT (id) DO NOTHING
			`, sample.ID, firstADA, sample.Title, sample.DueDate); err != nil {
				return fmt.Errorf("seed coding assignment %s: %w", sample.ID, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO coding_problems (assignment_id, entrypoint, description, input_format, output_format, constraints, difficulty, starter_code)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (assignment_id) DO NOTHING
			`, sample.ID, sample.Problem.Entrypoint, sample.Problem.Description, sample.Problem.InputFormat, sample.Problem.OutputFormat, sample.Problem.Constraints, sample.Problem.Difficulty, starterBytes); err != nil {
				return fmt.Errorf("seed coding problem %s: %w", sample.ID, err)
			}
			for i, tc := range sample.Problem.TestCases {
				if _, err := tx.Exec(ctx, `
					INSERT INTO coding_test_cases (assignment_id, ordinal, input_data, expected_output, is_hidden)
					VALUES ($1, $2, $3, $4, $5)
					ON CONFLICT (assignment_id, ordinal) DO NOTHING
				`, sample.ID, i+1, tc.Input, tc.Expected, tc.Hidden); err != nil {
					return fmt.Errorf("seed coding test case %s#%d: %w", sample.ID, i+1, err)
				}
			}
		}
	}

	// 6. Schedule Slots & Notices
	for day, slots := range data.DemoWeek.Days {
		for _, slot := range slots {
			slotType := slot.Type
			if slotType == "" {
				slotType = "Lecture"
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO schedule_slots (id, week_start, slot_date, time_from, time_to, course_code, room, topic, slot_type, published)
				VALUES ($1, $2::date, $3::date, $4::time, $5::time, $6, $7, $8, $9, $10)
			`, uuid.NewString(), data.DemoWeek.Start, day, slot.From, slot.To, slot.Code, slot.Room, slot.Topic, slotType, data.DemoWeek.Published); err != nil {
				return fmt.Errorf("seed schedule slot %s %s: %w", day, slot.From, err)
			}
		}
	}

	for placement, list := range map[string][]catalog.Notice{
		"dashboard": data.NoticesData,
		"drawer":    data.DrawerNotices,
	} {
		for idx, n := range list {
			id := n.ID
			if id == "" {
				id = uuid.NewString()
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO notices (id, placement, tag, tag_bg, time_label, title, description, action_label, action_route, sort_order)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (id) DO NOTHING
			`, id, placement, n.Tag, n.TagBg, n.Time, n.Title, n.Desc, n.ActionLabel, n.ActionRoute, idx); err != nil {
				return fmt.Errorf("seed notice %s: %w", id, err)
			}
		}
	}

	// 7. Course Offerings & Class Sessions
	for _, off := range homepage.CourseOfferings {
		reqAtt := off.CourseRequiredAttendance
		if reqAtt == 0 {
			reqAtt = 75
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO course_offerings (id, course_id, course_code, semester_id, campus_id, program_id, label, title, batch, credits, description, icon_url, bg_color, enrolment_count, class_session_count, required_attendance)
			VALUES ($1,$2,$3,$4,1,1,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			ON CONFLICT (id) DO NOTHING
		`, off.ID, off.CourseID, off.CourseCode, off.SemesterID, off.Label, off.Title, off.Batch, off.Credits, off.Description, off.IconURL, off.BgColor, off.EnrolmentCount, off.ClassSessionCount, reqAtt); err != nil {
			return fmt.Errorf("seed course offering %d: %w", off.ID, err)
		}
	}
	for _, cs := range homepage.ClassSessions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO class_sessions (id, course_offering_id, program_id, semester_id, cpr_id, start_time, end_time, title, topic, label, room, cs_created_at, cs_updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT (id) DO NOTHING
		`, cs.ID, cs.CourseOfferingID, cs.ProgramID, cs.SemesterID, cs.CPRID, cs.StartTime, cs.EndTime, cs.Title, cs.Topic, cs.Label, cs.Room, cs.CSCreatedAt, cs.CSUpdatedAt); err != nil {
			return fmt.Errorf("seed class session %d: %w", cs.ID, err)
		}
	}

	// 8. Users + Student Attendance, Enrolments, Biometrics, Contests & Invoices
	pwHash, err := hasher.Hash("VedamLocal123!")
	if err != nil {
		return fmt.Errorf("hash seed password: %w", err)
	}
	accounts := []struct {
		Role  iam.Role
		Email string
		Name  string
	}{
		{Role: iam.RoleAdmin, Email: "admin@vedam.local", Name: "Academic Admin"},
		{Role: iam.RoleTeacher, Email: "teacher@vedam.local", Name: "Dr. Nandini Kulkarni"},
		{Role: iam.RoleStudent, Email: "student@vedam.local", Name: "Sirwagya Shekhar"},
	}
	for _, acc := range accounts {
		uid := uuid.NewString()
		profileJSON := json.RawMessage(`{}`)
		if acc.Role == iam.RoleStudent {
			profileJSON = json.RawMessage(`{"campusEmail":"student@vedam.local","cprId":125}`)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, email, name, role, password_hash, profile_json)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, uid, acc.Email, acc.Name, string(acc.Role), pwHash, []byte(profileJSON)); err != nil {
			return fmt.Errorf("seed user %s: %w", acc.Email, err)
		}
		if acc.Role != iam.RoleStudent {
			continue
		}

		pe := homepage.ProgramEnrolment
		if _, err := tx.Exec(ctx, `
			INSERT INTO campus_person_roles (cpr_id, user_id, campus_id, person_name, email)
			VALUES ($1, $2, $3, $4, $5)
		`, pe.CPRID, uid, pe.CampusID, pe.PersonName, pe.Email); err != nil {
			return fmt.Errorf("seed campus person role: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO program_enrolments (id, program_id, cpr_id, enrolment_date)
			VALUES ($1, $2, $3, $4)
		`, pe.ProgramEnrolmentID, pe.ProgramID, pe.CPRID, pe.EnrolmentDate); err != nil {
			return fmt.Errorf("seed program enrolment: %w", err)
		}

		var faceStruct struct {
			Embedding []float64 `json:"embedding"`
		}
		_ = json.Unmarshal([]byte(homepage.FaceBiometrics.FaceJSON), &faceStruct)
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_face_biometrics (person_id, email, has_face_data, face_json, embedding)
			VALUES ($1, $2, $3, $4, $5)
		`, homepage.FaceBiometrics.PersonID, homepage.FaceBiometrics.Email, homepage.FaceBiometrics.HasFaceData, homepage.FaceBiometrics.FaceJSON, faceStruct.Embedding); err != nil {
			return fmt.Errorf("seed face biometrics: %w", err)
		}

		for _, off := range homepage.CourseOfferings {
			if off.Enrolment == nil {
				continue
			}
			en := off.Enrolment
			if _, err := tx.Exec(ctx, `
				INSERT INTO course_offering_enrolments (
					id, course_offering_id, cpr_id, enrolment_date,
					present_count, absent_count, late_count, leave_count, excused_count, awaiting_count, total_count
				)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			`, en.ID, off.ID, pe.CPRID, en.EnrolmentDate,
				en.PresentCount, en.AbsentCount, en.LateCount, en.LeaveCount, en.ExcusedCount, en.AwaitingCount, en.TotalCount); err != nil {
				return fmt.Errorf("seed course offering enrolment %d: %w", en.ID, err)
			}
		}

		for _, s := range allSubjects {
			if _, err := tx.Exec(ctx, `
				INSERT INTO attendance_summaries (user_id, course_code, attended, total)
				VALUES ($1, $2, $3, $4)
			`, uid, s.Code, s.AttendedClasses, s.TotalClasses); err != nil {
				return fmt.Errorf("seed attendance %s: %w", s.Code, err)
			}
		}
		for _, f := range data.FeeRecords {
			if _, err := tx.Exec(ctx, `
				INSERT INTO fee_invoices (id, user_id, invoice_no, description, semester, amount, due_date, paid_date, status, receipt_ready, simulated)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, TRUE)
			`, f.ID, uid, f.InvoiceNo, f.Description, f.Semester, f.Amount, f.DueDate, f.PaidDate, string(f.Status), f.ReceiptAvailable); err != nil {
				return fmt.Errorf("seed fee %s: %w", f.ID, err)
			}
		}
		for courseCode, summary := range data.ContestSummary {
			if _, err := tx.Exec(ctx, `
				INSERT INTO contest_summaries (user_id, course_code, percent, detail, completed, total, best_rank)
				VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))
			`, uid, courseCode, summary.Percent, summary.Detail, summary.Completed, summary.Total, summary.BestRank); err != nil {
				return fmt.Errorf("seed contest summary %s: %w", courseCode, err)
			}
			for ord, attempt := range summary.History {
				if _, err := tx.Exec(ctx, `
					INSERT INTO contest_attempts (id, user_id, course_code, label, title, contest_date, score, percent, rank_label, ordinal)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				`, uuid.NewString(), uid, courseCode, attempt.Label, attempt.Title, attempt.Date, attempt.Score, attempt.Percent, attempt.Rank, ord); err != nil {
					return fmt.Errorf("seed contest attempt %s#%d: %w", courseCode, ord, err)
				}
			}
		}
		for ord, rc := range data.RecentContests {
			if _, err := tx.Exec(ctx, `
				INSERT INTO recent_contests (id, user_id, title, course_code, contest_date, score, percent, rank_label, status, ordinal)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			`, rc.ID, uid, rc.Title, rc.SubjectCode, rc.Date, rc.Score, rc.Percent, rc.Rank, rc.Status, ord); err != nil {
				return fmt.Errorf("seed recent contest %s: %w", rc.ID, err)
			}
		}
	}

	return tx.Commit(ctx)
}
