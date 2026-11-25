package memory

import (
	"slices"

	"vedam/lms/internal/domain/academic"
	"vedam/lms/internal/domain/assessment"
	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/finance"
	"vedam/lms/internal/domain/iam"
	"vedam/lms/internal/domain/learning"
	syncdomain "vedam/lms/internal/domain/sync"
)

type txStateKey struct{}

type storedProgramEnrolment struct {
	id            int64
	programID     int
	cprID         int64
	enrolmentDate int64
}

type storedCourseOfferingEnrolment struct {
	id               int64
	courseOfferingID int64
	cprID            int64
	enrolmentDate    int64
	presentCount     int
	absentCount      int
	lateCount        int
	leaveCount       int
	excusedCount     int
	awaitingCount    int
	totalCount       int
}

type dataState struct {
	users          map[string]iam.User
	campuses       map[int]academic.Campus
	programs       map[int]academic.Program
	cprs           map[int64]academic.CampusPersonRole
	progEnrols     map[int64]storedProgramEnrolment
	biometrics     map[int64]academic.FaceBiometricRecord
	semRecords     map[int]academic.SemesterRecord
	offerings      map[int64]academic.CourseOfferingSpec
	offeringEnrols map[int64]storedCourseOfferingEnrolment
	classSessions  map[int64]academic.ClassSessionItem
	semesters      map[int]catalog.Semester
	courses        map[string]catalog.Course
	lectures       map[int64]catalog.Lecture
	schedule       catalog.WeekSchedule
	notices        []catalog.Notice
	assets         map[string]catalog.Asset
	blobs          map[string][]byte
	attendance     map[string]map[string]learning.AttendanceSummary
	progress       map[string]map[int64]struct{}
	discussions    []learning.Discussion
	writtenAssign  map[string]storedWrittenAssignment
	codingAssign   map[string]storedCodingAssignment
	submissions    map[string]assessment.Submission
	codingAttempts []assessment.CodingAttempt
	contestSums    map[string]map[string]assessment.ContestSummaryItem
	recentContests map[string][]assessment.ContestRecord
	invoices       map[string]finance.FeeInvoice
	payments       map[string]finance.PaymentRecord
	outbox         []syncdomain.OutboxEvent
	changelog      []syncdomain.ChangelogEntry
	idem           map[string][]byte
	nextLectureID  int64
	nextSeq        int64
}

type storedWrittenAssignment struct {
	id         string
	lectureID  int64
	courseCode string
	title      string
	dueDate    string
}

type storedCodingAssignment struct {
	id         string
	lectureID  int64
	courseCode string
	title      string
	dueDate    string
	problem    assessment.CodingProblemSpec
}

func newDataState() *dataState {
	return &dataState{
		users:          make(map[string]iam.User),
		campuses:       make(map[int]academic.Campus),
		programs:       make(map[int]academic.Program),
		cprs:           make(map[int64]academic.CampusPersonRole),
		progEnrols:     make(map[int64]storedProgramEnrolment),
		biometrics:     make(map[int64]academic.FaceBiometricRecord),
		semRecords:     make(map[int]academic.SemesterRecord),
		offerings:      make(map[int64]academic.CourseOfferingSpec),
		offeringEnrols: make(map[int64]storedCourseOfferingEnrolment),
		classSessions:  make(map[int64]academic.ClassSessionItem),
		semesters:      make(map[int]catalog.Semester),
		courses:        make(map[string]catalog.Course),
		lectures:       make(map[int64]catalog.Lecture),
		schedule:       catalog.WeekSchedule{Days: make(map[string][]catalog.ScheduleClass)},
		assets:         make(map[string]catalog.Asset),
		blobs:          make(map[string][]byte),
		attendance:     make(map[string]map[string]learning.AttendanceSummary),
		progress:       make(map[string]map[int64]struct{}),
		writtenAssign:  make(map[string]storedWrittenAssignment),
		codingAssign:   make(map[string]storedCodingAssignment),
		submissions:    make(map[string]assessment.Submission),
		contestSums:    make(map[string]map[string]assessment.ContestSummaryItem),
		recentContests: make(map[string][]assessment.ContestRecord),
		invoices:       make(map[string]finance.FeeInvoice),
		payments:       make(map[string]finance.PaymentRecord),
		idem:           make(map[string][]byte),
		nextLectureID:  1,
		nextSeq:        1,
	}
}

func (s *dataState) clone() *dataState {
	cp := newDataState()
	for k, v := range s.users {
		v.Profile = slices.Clone(v.Profile)
		cp.users[k] = v
	}
	for k, v := range s.campuses {
		cp.campuses[k] = v
	}
	for k, v := range s.programs {
		cp.programs[k] = v
	}
	for k, v := range s.cprs {
		cp.cprs[k] = v
	}
	for k, v := range s.progEnrols {
		cp.progEnrols[k] = v
	}
	for k, v := range s.biometrics {
		v.Embedding = slices.Clone(v.Embedding)
		cp.biometrics[k] = v
	}
	for k, v := range s.semRecords {
		cp.semRecords[k] = v
	}
	for k, v := range s.offerings {
		cp.offerings[k] = v
	}
	for k, v := range s.offeringEnrols {
		cp.offeringEnrols[k] = v
	}
	for k, v := range s.classSessions {
		cp.classSessions[k] = v
	}
	for k, v := range s.semesters {
		cp.semesters[k] = v
	}
	for k, v := range s.courses {
		cp.courses[k] = v
	}
	for k, l := range s.lectures {
		l.Resources = slices.Clone(l.Resources)
		l.Assignments = slices.Clone(l.Assignments)
		cp.lectures[k] = l
	}
	cp.schedule = catalog.WeekSchedule{
		Start:     s.schedule.Start,
		Published: s.schedule.Published,
		Days:      make(map[string][]catalog.ScheduleClass, len(s.schedule.Days)),
	}
	for d, slots := range s.schedule.Days {
		cp.schedule.Days[d] = slices.Clone(slots)
	}
	cp.notices = slices.Clone(s.notices)
	for k, v := range s.assets {
		cp.assets[k] = v
	}
	for k, v := range s.blobs {
		cp.blobs[k] = slices.Clone(v)
	}
	for u, m := range s.attendance {
		nm := make(map[string]learning.AttendanceSummary, len(m))
		for c, val := range m {
			nm[c] = val
		}
		cp.attendance[u] = nm
	}
	for u, m := range s.progress {
		nm := make(map[int64]struct{}, len(m))
		for lid := range m {
			nm[lid] = struct{}{}
		}
		cp.progress[u] = nm
	}
	cp.discussions = slices.Clone(s.discussions)
	for k, v := range s.writtenAssign {
		cp.writtenAssign[k] = v
	}
	for k, v := range s.codingAssign {
		sc := make(map[string]string, len(v.problem.StarterCode))
		for sk, sv := range v.problem.StarterCode {
			sc[sk] = sv
		}
		v.problem.StarterCode = sc
		v.problem.TestCases = slices.Clone(v.problem.TestCases)
		cp.codingAssign[k] = v
	}
	for k, v := range s.submissions {
		cp.submissions[k] = v
	}
	cp.codingAttempts = slices.Clone(s.codingAttempts)
	for u, m := range s.contestSums {
		nm := make(map[string]assessment.ContestSummaryItem, len(m))
		for c, val := range m {
			val.History = slices.Clone(val.History)
			nm[c] = val
		}
		cp.contestSums[u] = nm
	}
	for u, list := range s.recentContests {
		cp.recentContests[u] = slices.Clone(list)
	}
	for k, v := range s.invoices {
		cp.invoices[k] = v
	}
	for k, v := range s.payments {
		cp.payments[k] = v
	}
	cp.outbox = slices.Clone(s.outbox)
	cp.changelog = slices.Clone(s.changelog)
	for k, v := range s.idem {
		cp.idem[k] = slices.Clone(v)
	}
	cp.nextLectureID = s.nextLectureID
	cp.nextSeq = s.nextSeq
	return cp
}
