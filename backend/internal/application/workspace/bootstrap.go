package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"vedam/lms/internal/domain/assessment"
	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/iam"
	"vedam/lms/internal/domain/learning"
	finance "vedam/lms/internal/domain/finance"
)

type BootstrapResponse struct {
	SemesterSubjects  map[int][]catalog.Course                 `json:"semesterSubjects"`
	LecturesBySubject map[string][]catalog.Lecture             `json:"lecturesBySubject"`
	ContestSummary    map[string]assessment.ContestSummaryItem `json:"contestSummary"`
	RecentContests    []assessment.ContestRecord               `json:"recentContests"`
	DemoWeek          *catalog.WeekSchedule                    `json:"demoWeek"`
	NoticesData       []catalog.Notice                         `json:"noticesData"`
	DrawerNotices     []catalog.Notice                         `json:"drawerNotices"`
	User              iam.PublicUser                           `json:"user"`
	Fees              []finance.FeeInvoice                     `json:"fees"`
	Submissions       []assessment.Submission                  `json:"submissions"`
	Progress          []int64                                  `json:"progress"`
}

type BootstrapService struct {
	catalogRead   catalog.CatalogReader
	scheduleRead  catalog.ScheduleRepository
	noticeRead    catalog.NoticeRepository
	attRead       learning.AttendanceReader
	progressRead  learning.ProgressRepository
	subRead       assessment.SubmissionRepository
	studentAssign assessment.StudentAssignmentReader
	staffAssign   assessment.StaffAssignmentAuthor
	contestRead   assessment.ContestRepository
	invoiceRead   finance.InvoiceReader
}

func NewBootstrapService(
	catalogRead catalog.CatalogReader,
	scheduleRead catalog.ScheduleRepository,
	noticeRead catalog.NoticeRepository,
	attRead learning.AttendanceReader,
	progressRead learning.ProgressRepository,
	subRead assessment.SubmissionRepository,
	studentAssign assessment.StudentAssignmentReader,
	staffAssign assessment.StaffAssignmentAuthor,
	contestRead assessment.ContestRepository,
	invoiceRead finance.InvoiceReader,
) *BootstrapService {
	return &BootstrapService{
		catalogRead:   catalogRead,
		scheduleRead:  scheduleRead,
		noticeRead:    noticeRead,
		attRead:       attRead,
		progressRead:  progressRead,
		subRead:       subRead,
		studentAssign: studentAssign,
		staffAssign:   staffAssign,
		contestRead:   contestRead,
		invoiceRead:   invoiceRead,
	}
}

func (s *BootstrapService) Build(ctx context.Context, user *iam.User) (*BootstrapResponse, error) {
	subjects, err := s.catalogRead.ListCoursesBySemester(ctx)
	if err != nil {
		return nil, err
	}
	lecturesByCourse, err := s.catalogRead.ListLecturesByCourse(ctx)
	if err != nil {
		return nil, err
	}
	week, err := s.scheduleRead.GetWeekSchedule(ctx)
	if err != nil {
		return nil, err
	}
	dashNotices, err := s.noticeRead.ListByPlacement(ctx, "dashboard")
	if err != nil {
		return nil, err
	}
	drawerNotices, err := s.noticeRead.ListByPlacement(ctx, "drawer")
	if err != nil {
		return nil, err
	}
	submissions, err := s.subRead.ListForStudent(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	progress, err := s.progressRead.ListCompletedLectures(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	fees, err := s.invoiceRead.ListForStudent(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	// Overlay user-specific attendance onto courses for students
	if user.Role == iam.RoleStudent {
		attMap, err := s.attRead.ListForStudent(ctx, user.ID)
		if err != nil {
			return nil, err
		}
		for sem, list := range subjects {
			for i := range list {
				if sum, ok := attMap[list[i].Code]; ok {
					list[i].AttendedClasses = sum.Attended
					list[i].TotalClasses = sum.Total
				} else {
					list[i].AttendedClasses = 0
					list[i].TotalClasses = 0
				}
			}
			subjects[sem] = list
		}
	}

	// Build lecture -> assignments map preserving student/staff hidden test-case boundary (ISP)
	byLecture := make(map[int64][]catalog.LectureAssignmentRef)
	assignTitles := make(map[string]string)
	if user.Role == iam.RoleStudent {
		list, err := s.studentAssign.ListAllPublic(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range list {
			assignTitles[a.ID] = a.Title
			var prob any
			if a.Kind == assessment.KindCoding {
				prob = map[string]any{
					"entrypoint":   a.Entrypoint,
					"description":  a.Description,
					"inputFormat":  a.InputFormat,
					"outputFormat": a.OutputFmt,
					"constraints":  a.Constraints,
					"difficulty":   a.Difficulty,
					"starterCode":  a.StarterCode,
					"examples":     a.Examples,
					"totalTests":   a.TotalTests,
				}
			}
			byLecture[a.LectureID] = append(byLecture[a.LectureID], catalog.LectureAssignmentRef{
				ID:      a.ID,
				Kind:    string(a.Kind),
				Title:   a.Title,
				DueDate: a.DueDate,
				Status:  "Pending",
				Problem: prob,
			})
		}
	} else {
		list, err := s.staffAssign.ListAllWithHiddenTests(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range list {
			assignTitles[a.ID] = a.Title
			var prob any
			if a.Kind == assessment.KindCoding {
				prob = map[string]any{
					"entrypoint":   a.Entrypoint,
					"description":  a.Description,
					"inputFormat":  a.InputFormat,
					"outputFormat": a.OutputFmt,
					"constraints":  a.Constraints,
					"difficulty":   a.Difficulty,
					"starterCode":  a.StarterCode,
					"testCases":    a.AllTestCases,
				}
			}
			byLecture[a.LectureID] = append(byLecture[a.LectureID], catalog.LectureAssignmentRef{
				ID:      a.ID,
				Kind:    string(a.Kind),
				Title:   a.Title,
				DueDate: a.DueDate,
				Status:  "Pending",
				Problem: prob,
			})
		}
	}

	isDemoStudent := user.Email == "student@vedam.local"
	for code, lectures := range lecturesByCourse {
		for i := range lectures {
			if user.Role == iam.RoleStudent && !isDemoStudent {
				lectures[i].Attendance = "Awaiting"
			}
			if refs, ok := byLecture[lectures[i].ID]; ok {
				lectures[i].Assignments = refs
			} else if lectures[i].Assignments == nil {
				lectures[i].Assignments = []catalog.LectureAssignmentRef{}
			}
			if lectures[i].Resources == nil {
				lectures[i].Resources = []catalog.Resource{}
			}
		}
		lecturesByCourse[code] = lectures
	}

	summaries, err := s.contestRead.ListSummariesForStudent(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if summaries == nil {
		summaries = map[string]assessment.ContestSummaryItem{}
	}
	recentContests, err := s.contestRead.ListRecentForStudent(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if recentContests == nil {
		recentContests = []assessment.ContestRecord{}
	}

	// Merge dynamically graded submissions per course matching server/domain/workspace.mjs
	for code := range lecturesByCourse {
		var history []assessment.ContestAttempt
		sumPct := 0
		for _, sub := range submissions {
			if sub.Course != code || sub.Score == nil || *sub.Score == "" {
				continue
			}
			pct, ok := parseScorePercent(*sub.Score)
			if !ok {
				continue
			}
			title := assignTitles[sub.AssignmentID]
			if title == "" {
				title = "Assignment"
			}
			date := sub.Created
			if len(date) >= 10 {
				date = date[:10]
			}
			history = append(history, assessment.ContestAttempt{
				Label:   fmt.Sprintf("A%d", len(history)+1),
				Title:   title,
				Date:    date,
				Score:   *sub.Score,
				Percent: pct,
				Rank:    "—",
			})
			sumPct += pct
		}
		if len(history) > 0 {
			summaries[code] = assessment.ContestSummaryItem{
				Percent: int(math.Round(float64(sumPct) / float64(len(history)))),
				Detail:  fmt.Sprintf("%d graded assignments", len(history)),
				History: history,
			}
		}
	}

	if submissions == nil {
		submissions = []assessment.Submission{}
	}
	if progress == nil {
		progress = []int64{}
	}
	if fees == nil {
		fees = []finance.FeeInvoice{}
	}
	if dashNotices == nil {
		dashNotices = []catalog.Notice{}
	}
	if drawerNotices == nil {
		drawerNotices = []catalog.Notice{}
	}

	return &BootstrapResponse{
		SemesterSubjects:  subjects,
		LecturesBySubject: lecturesByCourse,
		ContestSummary:    summaries,
		RecentContests:    recentContests,
		DemoWeek:          week,
		NoticesData:       dashNotices,
		DrawerNotices:     drawerNotices,
		User:              user.Public(),
		Fees:              fees,
		Submissions:       submissions,
		Progress:          progress,
	}, nil
}

func parseScorePercent(score string) (int, bool) {
	parts := strings.Split(score, "/")
	if len(parts) != 2 {
		return 0, false
	}
	earned, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	maxScore, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil || maxScore <= 0 {
		return 0, false
	}
	return int(math.Round((earned / maxScore) * 100.0)), true
}

func (r *BootstrapResponse) MarshalJSONBytes() (json.RawMessage, error) {
	return json.Marshal(r)
}
