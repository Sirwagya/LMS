package memory

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"sync"

	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/errs"
	"vedam/lms/internal/domain/iam"
)

type Store struct {
	mu    sync.RWMutex
	state *dataState
}

func NewStore() *Store {
	return &Store{state: newDataState()}
}

func (s *Store) active(ctx context.Context) *dataState {
	if txSt, ok := ctx.Value(txStateKey{}).(*dataState); ok && txSt != nil {
		return txSt
	}
	return s.state
}

func (s *Store) lockRead(ctx context.Context) func() {
	if _, inTx := ctx.Value(txStateKey{}).(*dataState); inTx {
		return func() {}
	}
	s.mu.RLock()
	return s.mu.RUnlock
}

func (s *Store) lockWrite(ctx context.Context) func() {
	if _, inTx := ctx.Value(txStateKey{}).(*dataState); inTx {
		return func() {}
	}
	s.mu.Lock()
	return s.mu.Unlock
}

func (s *Store) WithinTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if _, ok := ctx.Value(txStateKey{}).(*dataState); ok {
		return fn(ctx)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := s.state.clone()
	txCtx := context.WithValue(ctx, txStateKey{}, snapshot)
	if err := fn(txCtx); err != nil {
		return err
	}
	s.state = snapshot
	return nil
}

type UserRepo struct{ s *Store }

func (s *Store) Users() *UserRepo { return &UserRepo{s: s} }

func (r *UserRepo) GetByID(ctx context.Context, id string) (*iam.User, error) {
	defer r.s.lockRead(ctx)()
	u, ok := r.s.active(ctx).users[id]
	if !ok {
		return nil, errs.ErrNotFound
	}
	cp := u
	return &cp, nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*iam.User, error) {
	defer r.s.lockRead(ctx)()
	target := strings.ToLower(email)
	for _, u := range r.s.active(ctx).users {
		if strings.ToLower(u.Email) == target {
			cp := u
			return &cp, nil
		}
	}
	return nil, errs.ErrNotFound
}

func (r *UserRepo) FindByRole(ctx context.Context, id string, role iam.Role) (*iam.User, error) {
	u, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u.Role != role {
		return nil, errs.ErrNotFound
	}
	return u, nil
}

func (r *UserRepo) ListAll(ctx context.Context) ([]iam.User, error) {
	defer r.s.lockRead(ctx)()
	out := make([]iam.User, 0, len(r.s.active(ctx).users))
	for _, u := range r.s.active(ctx).users {
		out = append(out, u)
	}
	slices.SortFunc(out, func(a, b iam.User) int { return strings.Compare(a.Email, b.Email) })
	return out, nil
}

func (r *UserRepo) Create(ctx context.Context, user *iam.User) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	targetEmail := strings.ToLower(user.Email)
	for _, existing := range st.users {
		if strings.ToLower(existing.Email) == targetEmail || existing.ID == user.ID {
			return errs.ErrConflict
		}
	}
	cp := *user
	if cp.Version == 0 {
		cp.Version = 1
	}
	if len(cp.Profile) == 0 {
		cp.Profile = json.RawMessage(`{}`)
	}
	st.users[cp.ID] = cp
	return nil
}

func (r *UserRepo) UpdateProfile(ctx context.Context, userID, name string, profile json.RawMessage, expectedVersion *int64) (int64, error) {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	u, ok := st.users[userID]
	if !ok {
		return 0, errs.ErrNotFound
	}
	if expectedVersion != nil && u.Version != *expectedVersion {
		return 0, errs.ErrConflict
	}
	u.Name = name
	u.Profile = slices.Clone(profile)
	u.Version++
	st.users[userID] = u
	return u.Version, nil
}

func (r *UserRepo) UpdatePasswordHash(ctx context.Context, userID, newHash string) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	u, ok := st.users[userID]
	if !ok {
		return errs.ErrNotFound
	}
	u.PasswordHash = newHash
	st.users[userID] = u
	return nil
}

type CatalogRepo struct{ s *Store }

func (s *Store) Catalog() *CatalogRepo { return &CatalogRepo{s: s} }

func (r *CatalogRepo) EnsureSemester(ctx context.Context, sem catalog.Semester) {
	defer r.s.lockWrite(ctx)()
	r.s.active(ctx).semesters[sem.Number] = sem
}

func (r *CatalogRepo) ListSemesters(ctx context.Context) ([]catalog.Semester, error) {
	defer r.s.lockRead(ctx)()
	out := make([]catalog.Semester, 0, len(r.s.active(ctx).semesters))
	for _, s := range r.s.active(ctx).semesters {
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b catalog.Semester) int { return a.Number - b.Number })
	return out, nil
}

func (r *CatalogRepo) ListCoursesBySemester(ctx context.Context) (map[int][]catalog.Course, error) {
	defer r.s.lockRead(ctx)()
	st := r.s.active(ctx)
	out := make(map[int][]catalog.Course)
	for _, sem := range st.semesters {
		out[sem.Number] = []catalog.Course{}
	}
	for _, c := range st.courses {
		out[c.Semester] = append(out[c.Semester], c)
	}
	for sem, list := range out {
		slices.SortFunc(list, func(a, b catalog.Course) int { return strings.Compare(a.Code, b.Code) })
		out[sem] = list
	}
	return out, nil
}

func (r *CatalogRepo) GetCourse(ctx context.Context, code string) (*catalog.Course, error) {
	defer r.s.lockRead(ctx)()
	c, ok := r.s.active(ctx).courses[code]
	if !ok {
		return nil, errs.ErrNotFound
	}
	cp := c
	return &cp, nil
}

func (r *CatalogRepo) CreateCourse(ctx context.Context, course *catalog.Course) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, exists := st.courses[course.Code]; exists {
		return errs.ErrConflict
	}
	if _, ok := st.semesters[course.Semester]; !ok {
		st.semesters[course.Semester] = catalog.Semester{Number: course.Semester, Label: "Semester", IsCurrent: course.Semester == 3}
	}
	cp := *course
	cp.Version = 1
	st.courses[cp.Code] = cp
	return nil
}

func (r *CatalogRepo) UpdateCourseMeta(ctx context.Context, code, name, instructor string) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	c, ok := st.courses[code]
	if !ok {
		return errs.ErrNotFound
	}
	c.Name = name
	c.Instructor = instructor
	c.Version++
	st.courses[code] = c
	return nil
}

func (r *CatalogRepo) ListLecturesByCourse(ctx context.Context) (map[string][]catalog.Lecture, error) {
	defer r.s.lockRead(ctx)()
	st := r.s.active(ctx)
	out := make(map[string][]catalog.Lecture)
	for code := range st.courses {
		out[code] = []catalog.Lecture{}
	}
	for _, l := range st.lectures {
		cp := l
		cp.Resources = slices.Clone(l.Resources)
		cp.Assignments = slices.Clone(l.Assignments)
		out[l.CourseCode] = append(out[l.CourseCode], cp)
	}
	for code, list := range out {
		slices.SortFunc(list, func(a, b catalog.Lecture) int {
			if a.SortOrder != b.SortOrder {
				return a.SortOrder - b.SortOrder
			}
			return int(a.ID - b.ID)
		})
		out[code] = list
	}
	return out, nil
}

func (r *CatalogRepo) GetLecture(ctx context.Context, id int64) (*catalog.Lecture, error) {
	defer r.s.lockRead(ctx)()
	l, ok := r.s.active(ctx).lectures[id]
	if !ok {
		return nil, errs.ErrNotFound
	}
	cp := l
	cp.Resources = slices.Clone(l.Resources)
	cp.Assignments = slices.Clone(l.Assignments)
	return &cp, nil
}

func (r *CatalogRepo) CreateLecture(ctx context.Context, lecture *catalog.Lecture) (int64, error) {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, ok := st.courses[lecture.CourseCode]; !ok {
		return 0, errs.ErrNotFound
	}
	id := lecture.ID
	if id == 0 {
		id = st.nextLectureID
		st.nextLectureID++
	} else if id >= st.nextLectureID {
		st.nextLectureID = id + 1
	}
	cp := *lecture
	cp.ID = id
	cp.Version = 1
	cp.Resources = slices.Clone(lecture.Resources)
	cp.Assignments = slices.Clone(lecture.Assignments)
	st.lectures[id] = cp
	return id, nil
}

func (r *CatalogRepo) UpdateLectureMeta(ctx context.Context, id int64, title, summary string) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	l, ok := st.lectures[id]
	if !ok {
		return errs.ErrNotFound
	}
	l.Title = title
	l.Summary = summary
	l.Version++
	st.lectures[id] = l
	return nil
}

func (r *CatalogRepo) SetLectureRecording(ctx context.Context, lectureID int64, recordingURL, _ string) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	l, ok := st.lectures[lectureID]
	if !ok {
		return errs.ErrNotFound
	}
	url := recordingURL
	l.RecordingURL = &url
	l.Version++
	st.lectures[lectureID] = l
	return nil
}

func (r *CatalogRepo) AddLectureResource(ctx context.Context, lectureID int64, res catalog.Resource, _ *string) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	l, ok := st.lectures[lectureID]
	if !ok {
		return errs.ErrNotFound
	}
	l.Resources = append(l.Resources, res)
	l.Version++
	st.lectures[lectureID] = l
	return nil
}

type ScheduleRepo struct{ s *Store }

func (s *Store) Schedule() *ScheduleRepo { return &ScheduleRepo{s: s} }

func (r *ScheduleRepo) GetWeekSchedule(ctx context.Context) (*catalog.WeekSchedule, error) {
	defer r.s.lockRead(ctx)()
	st := r.s.active(ctx)
	cp := catalog.WeekSchedule{
		Start:     st.schedule.Start,
		Published: st.schedule.Published,
		Days:      make(map[string][]catalog.ScheduleClass, len(st.schedule.Days)),
	}
	for day, slots := range st.schedule.Days {
		cp.Days[day] = slices.Clone(slots)
	}
	return &cp, nil
}

func (r *ScheduleRepo) AddSlot(ctx context.Context, slot catalog.ScheduleClass) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	if _, ok := st.courses[slot.Code]; !ok {
		return errs.ErrNotFound
	}
	existing := st.schedule.Days[slot.SlotDate]
	for _, e := range existing {
		if slot.From < e.To && slot.To > e.From {
			return errs.ErrScheduleOverlap
		}
	}
	st.schedule.Start = slot.Week
	st.schedule.Published = true
	existing = append(existing, slot)
	slices.SortFunc(existing, func(a, b catalog.ScheduleClass) int { return strings.Compare(a.From, b.From) })
	st.schedule.Days[slot.SlotDate] = existing
	return nil
}

type NoticeRepo struct{ s *Store }

func (s *Store) Notices() *NoticeRepo { return &NoticeRepo{s: s} }

func (r *NoticeRepo) ListByPlacement(ctx context.Context, placement string) ([]catalog.Notice, error) {
	defer r.s.lockRead(ctx)()
	var out []catalog.Notice
	for _, n := range r.s.active(ctx).notices {
		if n.Placement == placement {
			out = append(out, n)
		}
	}
	return out, nil
}

func (r *NoticeRepo) UpdateNotice(ctx context.Context, id, title, desc string) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	for i := range st.notices {
		if st.notices[i].ID == id {
			st.notices[i].Title = title
			st.notices[i].Desc = desc
			return nil
		}
	}
	return errs.ErrNotFound
}

func (r *NoticeRepo) CreateNotice(ctx context.Context, notice *catalog.Notice) error {
	defer r.s.lockWrite(ctx)()
	st := r.s.active(ctx)
	st.notices = append([]catalog.Notice{*notice}, st.notices...)
	return nil
}

type AssetRepo struct{ s *Store }

func (s *Store) Assets() *AssetRepo { return &AssetRepo{s: s} }

func (r *AssetRepo) Create(ctx context.Context, asset *catalog.Asset) error {
	defer r.s.lockWrite(ctx)()
	r.s.active(ctx).assets[asset.ID] = *asset
	return nil
}

func (r *AssetRepo) GetByID(ctx context.Context, id string) (*catalog.Asset, error) {
	defer r.s.lockRead(ctx)()
	a, ok := r.s.active(ctx).assets[id]
	if !ok {
		return nil, errs.ErrNotFound
	}
	cp := a
	return &cp, nil
}

func (r *AssetRepo) Put(ctx context.Context, key string, data []byte) error {
	defer r.s.lockWrite(ctx)()
	r.s.active(ctx).blobs[key] = slices.Clone(data)
	return nil
}

func (r *AssetRepo) Get(ctx context.Context, key string) ([]byte, error) {
	defer r.s.lockRead(ctx)()
	data, ok := r.s.active(ctx).blobs[key]
	if !ok {
		return nil, errs.ErrNotFound
	}
	return slices.Clone(data), nil
}

var _ = maps.Clone[map[string]string]
