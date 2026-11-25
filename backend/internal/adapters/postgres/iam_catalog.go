package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/errs"
	"vedam/lms/internal/domain/iam"
)

type UserRepo struct{ tx *TxManager }

func NewUserRepo(tx *TxManager) *UserRepo { return &UserRepo{tx: tx} }

func (r *UserRepo) GetByID(ctx context.Context, id string) (*iam.User, error) {
	var u iam.User
	var role string
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT id, email, name, role, password_hash, profile_json, version, created_at, updated_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.Name, &role, &u.PasswordHash, &u.Profile, &u.Version, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, mapPGError(err)
	}
	u.Role = iam.Role(role)
	return &u, nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*iam.User, error) {
	var u iam.User
	var role string
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT id, email, name, role, password_hash, profile_json, version, created_at, updated_at
		FROM users WHERE LOWER(email) = LOWER($1)
	`, strings.TrimSpace(email)).Scan(&u.ID, &u.Email, &u.Name, &role, &u.PasswordHash, &u.Profile, &u.Version, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, mapPGError(err)
	}
	u.Role = iam.Role(role)
	return &u, nil
}

func (r *UserRepo) FindByRole(ctx context.Context, id string, role iam.Role) (*iam.User, error) {
	var u iam.User
	var roleStr string
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT id, email, name, role, password_hash, profile_json, version, created_at, updated_at
		FROM users WHERE id = $1 AND role = $2
	`, id, string(role)).Scan(&u.ID, &u.Email, &u.Name, &roleStr, &u.PasswordHash, &u.Profile, &u.Version, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, mapPGError(err)
	}
	u.Role = iam.Role(roleStr)
	return &u, nil
}

func (r *UserRepo) ListAll(ctx context.Context) ([]iam.User, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT id, email, name, role, password_hash, profile_json, version, created_at, updated_at
		FROM users ORDER BY email ASC
	`)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []iam.User
	for rows.Next() {
		var u iam.User
		var role string
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &role, &u.PasswordHash, &u.Profile, &u.Version, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		u.Role = iam.Role(role)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *UserRepo) Create(ctx context.Context, user *iam.User) error {
	profile := user.Profile
	if len(profile) == 0 {
		profile = json.RawMessage(`{}`)
	}
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO users (id, email, name, role, password_hash, profile_json)
		VALUES ($1, LOWER($2), $3, $4, $5, $6)
	`, user.ID, strings.TrimSpace(user.Email), user.Name, string(user.Role), user.PasswordHash, []byte(profile))
	return mapPGError(err)
}

func (r *UserRepo) UpdateProfile(ctx context.Context, userID, name string, profile json.RawMessage, expectedVersion *int64) (int64, error) {
	var newVersion int64
	if expectedVersion != nil {
		err := r.tx.Querier(ctx).QueryRow(ctx, `
			UPDATE users SET name = $1, profile_json = $2, version = version + 1, updated_at = NOW()
			WHERE id = $3 AND version = $4
			RETURNING version
		`, name, []byte(profile), userID, *expectedVersion).Scan(&newVersion)
		if err != nil {
			mapped := mapPGError(err)
			if mapped == errs.ErrNotFound {
				var exists bool
				_ = r.tx.Querier(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&exists)
				if exists {
					return 0, errs.ErrConflict
				}
			}
			return 0, mapped
		}
		return newVersion, nil
	}
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		UPDATE users SET name = $1, profile_json = $2, version = version + 1, updated_at = NOW()
		WHERE id = $3
		RETURNING version
	`, name, []byte(profile), userID).Scan(&newVersion)
	if err != nil {
		return 0, mapPGError(err)
	}
	return newVersion, nil
}

func (r *UserRepo) UpdatePasswordHash(ctx context.Context, userID, newHash string) error {
	tag, err := r.tx.Querier(ctx).Exec(ctx, `
		UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2
	`, newHash, userID)
	if err != nil {
		return mapPGError(err)
	}
	if tag.RowsAffected() == 0 {
		return errs.ErrNotFound
	}
	return nil
}

type CatalogRepo struct{ tx *TxManager }

func NewCatalogRepo(tx *TxManager) *CatalogRepo { return &CatalogRepo{tx: tx} }

func (r *CatalogRepo) ListSemesters(ctx context.Context) ([]catalog.Semester, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `SELECT number, label, is_current FROM semesters ORDER BY number ASC`)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []catalog.Semester
	for rows.Next() {
		var s catalog.Semester
		if err := rows.Scan(&s.Number, &s.Label, &s.IsCurrent); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *CatalogRepo) ListCoursesBySemester(ctx context.Context) (map[int][]catalog.Course, error) {
	sems, err := r.ListSemesters(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int][]catalog.Course, len(sems))
	for _, s := range sems {
		out[s.Number] = []catalog.Course{}
	}
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT code, semester_number, credits, name, instructor, next_topic, default_att_pct, skips_label, version
		FROM courses ORDER BY semester_number ASC, code ASC
	`)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c catalog.Course
		if err := rows.Scan(&c.Code, &c.Semester, &c.Credits, &c.Name, &c.Instructor, &c.Next, &c.Att, &c.Skips, &c.Version); err != nil {
			return nil, err
		}
		out[c.Semester] = append(out[c.Semester], c)
	}
	return out, rows.Err()
}

func (r *CatalogRepo) GetCourse(ctx context.Context, code string) (*catalog.Course, error) {
	var c catalog.Course
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT code, semester_number, credits, name, instructor, next_topic, default_att_pct, skips_label, version
		FROM courses WHERE code = $1
	`, code).Scan(&c.Code, &c.Semester, &c.Credits, &c.Name, &c.Instructor, &c.Next, &c.Att, &c.Skips, &c.Version)
	if err != nil {
		return nil, mapPGError(err)
	}
	return &c, nil
}

func (r *CatalogRepo) CreateCourse(ctx context.Context, c *catalog.Course) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO courses (code, semester_number, name, instructor, credits, next_topic, default_att_pct, skips_label)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, c.Code, c.Semester, c.Name, c.Instructor, c.Credits, c.Next, c.Att, c.Skips)
	return mapPGError(err)
}

func (r *CatalogRepo) UpdateCourseMeta(ctx context.Context, code, name, instructor string) error {
	tag, err := r.tx.Querier(ctx).Exec(ctx, `
		UPDATE courses SET name = $1, instructor = $2, version = version + 1, updated_at = NOW() WHERE code = $3
	`, name, instructor, code)
	if err != nil {
		return mapPGError(err)
	}
	if tag.RowsAffected() == 0 {
		return errs.ErrNotFound
	}
	return nil
}

func (r *CatalogRepo) ListLecturesByCourse(ctx context.Context) (map[string][]catalog.Lecture, error) {
	coursesRows, err := r.tx.Querier(ctx).Query(ctx, `SELECT code FROM courses`)
	if err != nil {
		return nil, mapPGError(err)
	}
	out := make(map[string][]catalog.Lecture)
	for coursesRows.Next() {
		var code string
		if err := coursesRows.Scan(&code); err != nil {
			coursesRows.Close()
			return nil, err
		}
		out[code] = []catalog.Lecture{}
	}
	coursesRows.Close()

	resByLecture := make(map[int64][]catalog.Resource)
	resRows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT id, lecture_id, title, resource_type, size_label, url
		FROM lecture_resources ORDER BY lecture_id ASC, sort_order ASC
	`)
	if err != nil {
		return nil, mapPGError(err)
	}
	for resRows.Next() {
		var lid int64
		var res catalog.Resource
		if err := resRows.Scan(&res.ID, &lid, &res.Title, &res.Type, &res.Size, &res.URL); err != nil {
			resRows.Close()
			return nil, err
		}
		resByLecture[lid] = append(resByLecture[lid], res)
	}
	resRows.Close()

	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT id, course_code, lecture_code, lecture_date, month_label, unit_number,
		       title, raw_title, default_att, summary, slides, duration, recording_url, sort_order, version
		FROM lectures ORDER BY course_code ASC, sort_order ASC, id ASC
	`)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var l catalog.Lecture
		if err := rows.Scan(
			&l.ID, &l.CourseCode, &l.Code, &l.Date, &l.Month, &l.Unit,
			&l.Title, &l.RawTitle, &l.Attendance, &l.Summary, &l.Slides, &l.Duration,
			&l.RecordingURL, &l.SortOrder, &l.Version,
		); err != nil {
			return nil, err
		}
		l.Resources = resByLecture[l.ID]
		if l.Resources == nil {
			l.Resources = []catalog.Resource{}
		}
		l.Assignments = []catalog.LectureAssignmentRef{}
		out[l.CourseCode] = append(out[l.CourseCode], l)
	}
	return out, rows.Err()
}

func (r *CatalogRepo) GetLecture(ctx context.Context, id int64) (*catalog.Lecture, error) {
	var l catalog.Lecture
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT id, course_code, lecture_code, lecture_date, month_label, unit_number,
		       title, raw_title, default_att, summary, slides, duration, recording_url, sort_order, version
		FROM lectures WHERE id = $1
	`, id).Scan(
		&l.ID, &l.CourseCode, &l.Code, &l.Date, &l.Month, &l.Unit,
		&l.Title, &l.RawTitle, &l.Attendance, &l.Summary, &l.Slides, &l.Duration,
		&l.RecordingURL, &l.SortOrder, &l.Version,
	)
	if err != nil {
		return nil, mapPGError(err)
	}
	l.Resources = []catalog.Resource{}
	l.Assignments = []catalog.LectureAssignmentRef{}
	return &l, nil
}

func (r *CatalogRepo) CreateLecture(ctx context.Context, l *catalog.Lecture) (int64, error) {
	var id int64
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		INSERT INTO lectures (course_code, lecture_code, lecture_date, month_label, unit_number, title, raw_title, default_att, summary, slides, duration, recording_url, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id
	`, l.CourseCode, l.Code, l.Date, l.Month, l.Unit, l.Title, l.RawTitle, l.Attendance, l.Summary, l.Slides, l.Duration, l.RecordingURL, l.SortOrder).Scan(&id)
	return id, mapPGError(err)
}

func (r *CatalogRepo) UpdateLectureMeta(ctx context.Context, id int64, title, summary string) error {
	tag, err := r.tx.Querier(ctx).Exec(ctx, `
		UPDATE lectures SET title = $1, summary = $2, version = version + 1, updated_at = NOW() WHERE id = $3
	`, title, summary, id)
	if err != nil {
		return mapPGError(err)
	}
	if tag.RowsAffected() == 0 {
		return errs.ErrNotFound
	}
	return nil
}

func (r *CatalogRepo) SetLectureRecording(ctx context.Context, lectureID int64, recordingURL, assetID string) error {
	tag, err := r.tx.Querier(ctx).Exec(ctx, `
		UPDATE lectures SET recording_url = $1, recording_asset = $2, version = version + 1, updated_at = NOW() WHERE id = $3
	`, recordingURL, assetID, lectureID)
	if err != nil {
		return mapPGError(err)
	}
	if tag.RowsAffected() == 0 {
		return errs.ErrNotFound
	}
	return nil
}

func (r *CatalogRepo) AddLectureResource(ctx context.Context, lectureID int64, res catalog.Resource, assetID *string) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO lecture_resources (id, lecture_id, title, resource_type, size_label, url, asset_id)
		VALUES (COALESCE(NULLIF($1, ''), gen_random_uuid()::text), $2, $3, $4, $5, $6, $7)
	`, res.ID, lectureID, res.Title, res.Type, res.Size, res.URL, assetID)
	return mapPGError(err)
}

type ScheduleRepo struct{ tx *TxManager }

func NewScheduleRepo(tx *TxManager) *ScheduleRepo { return &ScheduleRepo{tx: tx} }

func (r *ScheduleRepo) GetWeekSchedule(ctx context.Context) (*catalog.WeekSchedule, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT id, to_char(week_start, 'YYYY-MM-DD'), to_char(slot_date, 'YYYY-MM-DD'),
		       to_char(time_from, 'HH24:MI'), to_char(time_to, 'HH24:MI'),
		       course_code, room, topic, slot_type, published
		FROM schedule_slots ORDER BY slot_date ASC, time_from ASC
	`)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	ws := &catalog.WeekSchedule{
		Start:     "2026-10-19",
		Published: true,
		Days:      make(map[string][]catalog.ScheduleClass),
	}
	first := true
	for rows.Next() {
		var slot catalog.ScheduleClass
		var weekStart, slotDate string
		var published bool
		if err := rows.Scan(&slot.ID, &weekStart, &slotDate, &slot.From, &slot.To, &slot.Code, &slot.Room, &slot.Topic, &slot.Type, &published); err != nil {
			return nil, err
		}
		if first {
			ws.Start = weekStart
			ws.Published = published
			first = false
		}
		slot.Week = weekStart
		slot.SlotDate = slotDate
		ws.Days[slotDate] = append(ws.Days[slotDate], slot)
	}
	return ws, rows.Err()
}

func (r *ScheduleRepo) AddSlot(ctx context.Context, slot catalog.ScheduleClass) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO schedule_slots (id, week_start, slot_date, time_from, time_to, course_code, room, topic, slot_type, published)
		VALUES (COALESCE(NULLIF($1, ''), gen_random_uuid()::text), $2::date, $3::date, $4::time, $5::time, $6, $7, $8, $9, TRUE)
	`, slot.ID, slot.Week, slot.SlotDate, slot.From, slot.To, slot.Code, slot.Room, slot.Topic, slot.Type)
	return mapPGError(err)
}

type NoticeRepo struct{ tx *TxManager }

func NewNoticeRepo(tx *TxManager) *NoticeRepo { return &NoticeRepo{tx: tx} }

func (r *NoticeRepo) ListByPlacement(ctx context.Context, placement string) ([]catalog.Notice, error) {
	rows, err := r.tx.Querier(ctx).Query(ctx, `
		SELECT id, placement, tag, tag_bg, time_label, title, description, action_label, action_route, created_at
		FROM notices WHERE placement = $1 ORDER BY sort_order ASC, created_at DESC
	`, placement)
	if err != nil {
		return nil, mapPGError(err)
	}
	defer rows.Close()
	var out []catalog.Notice
	for rows.Next() {
		var n catalog.Notice
		if err := rows.Scan(&n.ID, &n.Placement, &n.Tag, &n.TagBg, &n.Time, &n.Title, &n.Desc, &n.ActionLabel, &n.ActionRoute, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *NoticeRepo) UpdateNotice(ctx context.Context, id, title, desc string) error {
	tag, err := r.tx.Querier(ctx).Exec(ctx, `UPDATE notices SET title = $1, description = $2 WHERE id = $3`, title, desc, id)
	if err != nil {
		return mapPGError(err)
	}
	if tag.RowsAffected() == 0 {
		return errs.ErrNotFound
	}
	return nil
}

func (r *NoticeRepo) CreateNotice(ctx context.Context, notice *catalog.Notice) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO notices (id, placement, tag, tag_bg, time_label, title, description, action_label, action_route, sort_order)
		VALUES (COALESCE(NULLIF($1, ''), gen_random_uuid()::text), $2, $3, $4, $5, $6, $7, $8, $9, -1)
	`, notice.ID, notice.Placement, notice.Tag, notice.TagBg, notice.Time, notice.Title, notice.Desc, notice.ActionLabel, notice.ActionRoute)
	return mapPGError(err)
}

type AssetMetaRepo struct{ tx *TxManager }

func NewAssetMetaRepo(tx *TxManager) *AssetMetaRepo { return &AssetMetaRepo{tx: tx} }

func (r *AssetMetaRepo) Create(ctx context.Context, asset *catalog.Asset) error {
	_, err := r.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO assets (id, filename, mime_type, byte_size, storage_key, sha256)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, asset.ID, asset.Filename, asset.MimeType, asset.ByteSize, asset.StorageKey, asset.SHA256)
	return mapPGError(err)
}

func (r *AssetMetaRepo) GetByID(ctx context.Context, id string) (*catalog.Asset, error) {
	var a catalog.Asset
	err := r.tx.Querier(ctx).QueryRow(ctx, `
		SELECT id, filename, mime_type, byte_size, storage_key, sha256, created_at
		FROM assets WHERE id = $1
	`, id).Scan(&a.ID, &a.Filename, &a.MimeType, &a.ByteSize, &a.StorageKey, &a.SHA256, &a.CreatedAt)
	if err != nil {
		return nil, mapPGError(err)
	}
	return &a, nil
}
