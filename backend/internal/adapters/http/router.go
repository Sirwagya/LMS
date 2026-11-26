package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	redisadapter "vedam/lms/internal/adapters/redis"
	"vedam/lms/internal/adapters/storage"
	"vedam/lms/internal/application/finance"
	learningapp "vedam/lms/internal/application/learning"
	"vedam/lms/internal/application/staff"
	"vedam/lms/internal/application/workspace"
	"vedam/lms/internal/domain/academic"
	"vedam/lms/internal/domain/assessment"
	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/envelope"
	"vedam/lms/internal/domain/errs"
	financedomain "vedam/lms/internal/domain/finance"
	"vedam/lms/internal/domain/iam"
	"vedam/lms/internal/domain/learning"
	syncdomain "vedam/lms/internal/domain/sync"
	"vedam/lms/internal/domain/uow"
)

type Users interface {
	iam.UserReader
	iam.UserWriter
	UpdatePasswordIfCurrent(context.Context, string, string, string) (bool, error)
}
type Catalog interface {
	catalog.CatalogReader
	catalog.CatalogWriter
}
type Academic interface {
	academic.EnrolmentReader
	academic.ClassSessionReader
	academic.BiometricRepository
	CPRForUser(context.Context, string) (int64, error)
}
type Sync interface {
	syncdomain.OutboxWriter
	syncdomain.SyncLogReader
	syncdomain.IdempotencyStore
	LockMutations(context.Context) error
}
type Progress interface {
	learning.ProgressRepository
	Version(context.Context, string, int64) (int64, bool, int64, error)
}
type Invoices interface {
	financedomain.InvoiceReader
	financedomain.InvoiceWriter
}
type Assignments interface {
	assessment.StudentAssignmentReader
	assessment.StaffAssignmentAuthor
}

type Dependencies struct {
	Log            *slog.Logger
	PingDB         func(context.Context) error
	Redis          *redis.Client
	Sessions       *redisadapter.Sessions
	Cache          *redisadapter.Cache
	CookieSecure   bool
	PublicOrigin   string
	Production     bool
	TrustedProxies []netip.Prefix
	SessionSliding bool
	SessionTTL     time.Duration
	Web            http.Handler
	Tx             uow.TxManager
	Users          Users
	Hasher         iam.PasswordHasher
	Catalog        Catalog
	Notices        catalog.NoticeRepository
	Assignments    Assignments
	Submissions    assessment.SubmissionRepository
	Attendance     learning.AttendanceReader
	Discussions    interface {
		GetByID(context.Context, string) (learning.Discussion, error)
	}
	Progress  Progress
	Invoices  Invoices
	Academic  Academic
	Sync      Sync
	AssetMeta catalog.AssetMetadataRepository
	Assets    *storage.LocalAssets
	Bootstrap *workspace.BootstrapService
	Learning  *learningapp.Service
	Staff     *staff.Service
	Payments  *finance.PayInvoiceService
	Coding    CodingService
	Queue     *redisadapter.Queue
}

type api struct {
	Dependencies
	passwords chan struct{}
	streams   streamSlots
}
type endpoint func(http.ResponseWriter, *http.Request) error
type access string

const (
	public        access = "public"
	authenticated access = "authenticated"
	student       access = "student"
	staffOnly     access = "staff"
	admin         access = "admin"
)

type route struct {
	method, path string
	access       access
	handle       endpoint
}

func New(d Dependencies) http.Handler {
	a := &api{Dependencies: d, passwords: make(chan struct{}, 4)}
	mux := http.NewServeMux()
	routes := []route{
		{"GET", "/health", public, a.health},
		{"POST", "/login", public, a.login},
		{"POST", "/register", public, a.register},
		{"POST", "/logout", authenticated, a.logout},
		{"POST", "/password", authenticated, a.password},
		{"GET", "/bootstrap", authenticated, a.bootstrap},
		{"PUT", "/profile", authenticated, a.profile},
		{"POST", "/progress", authenticated, a.progress},
		{"POST", "/submissions", student, a.submit},
		{"GET", "/submission-file/{id}", authenticated, a.submissionFile},
		{"GET", "/discussions", authenticated, a.discussions},
		{"POST", "/discussions", authenticated, a.postDiscussion},
		{"GET", "/notes/{id}", authenticated, a.notes},
		{"GET", "/assets/{id}", authenticated, a.asset},
		{"POST", "/payments", student, a.pay},
		{"GET", "/receipts/{id}", authenticated, a.receipt},
		{"POST", "/coding/evaluate", student, a.evaluate},
		{"GET", "/coding/history", authenticated, a.history},
		{"GET", "/staff", staffOnly, a.staffWorkspace},
		{"PUT", "/staff/catalog", staffOnly, a.editCatalog},
		{"POST", "/staff/courses", staffOnly, a.createCourse},
		{"POST", "/staff/lectures", staffOnly, a.createLecture},
		{"POST", "/staff/assignments", staffOnly, a.createAssignment},
		{"POST", "/staff/coding/assignments", staffOnly, a.createCoding},
		{"POST", "/staff/coding", staffOnly, a.createCoding},
		{"POST", "/staff/notices", staffOnly, a.createNotice},
		{"POST", "/staff/assets", staffOnly, a.uploadAsset},
		{"POST", "/staff/users", admin, a.createUser},
		{"POST", "/staff/fees", admin, a.createInvoice},
		{"PUT", "/staff/attendance", staffOnly, a.attendance},
		{"PUT", "/staff/grade", staffOnly, a.grade},
		{"POST", "/staff/schedule", staffOnly, a.schedule},
	}
	for _, rt := range routes {
		a.mount(mux, "/api", rt)
		a.mount(mux, "/api/v1", rt)
	}
	for _, rt := range []route{
		{"GET", "/sync/pull", authenticated, a.pull},
		{"POST", "/sync/push", authenticated, a.push},
		{"GET", "/events", authenticated, a.events},
		{"GET", "/coding/jobs/{id}", student, a.gradingJob},
		{"GET", "/program-enrolments", authenticated, a.programs},
		{"GET", "/semesters", authenticated, a.semesters},
		{"GET", "/course-offering-enrolments", authenticated, a.offerings},
		{"GET", "/class-sessions", authenticated, a.classSessions},
		{"GET", "/biometrics/face", authenticated, a.face},
	} {
		a.mount(mux, "/api/v1", rt)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { a.failure(w, r, problem(404, "Endpoint not found.")) })
	if d.Web != nil {
		mux.Handle("/", d.Web)
	}
	return a.requestID(a.logging(a.recovery(mux)))
}

func (a *api) mount(mux *http.ServeMux, prefix string, rt route) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := rt.handle(w, r); err != nil {
			a.failure(w, r, err)
		}
	})
	mux.Handle(rt.method+" "+prefix+rt.path, a.transport(a.authenticate(rt.access, a.rate(rt.path, a.role(rt.access, a.idempotency(rt.path, handler))))))
}

type httpError struct {
	status  int
	message string
}

func (e httpError) Error() string              { return e.message }
func problem(status int, message string) error { return httpError{status, message} }
func invalid(message string) error             { return problem(400, message) }
func missing(err error, message string) error {
	if errors.Is(err, errs.ErrNotFound) {
		return problem(404, message)
	}
	return err
}
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func ok(w http.ResponseWriter, status int, err error) error {
	if err == nil {
		writeJSON(w, status, map[string]bool{"ok": true})
	}
	return err
}
func (a *api) failure(w http.ResponseWriter, r *http.Request, err error) {
	status, message := 500, "Internal server error."
	var httpErr httpError
	switch {
	case errors.As(err, &httpErr):
		status, message = httpErr.status, httpErr.message
	case errors.Is(err, errs.ErrValidation):
		status, message = 400, err.Error()
	case errors.Is(err, errs.ErrUnauthorized):
		status, message = 401, "Please sign in."
	case errors.Is(err, errs.ErrForbidden):
		status, message = 403, err.Error()
	case errors.Is(err, errs.ErrNotFound):
		status, message = 404, err.Error()
	case errors.Is(err, errs.ErrConflict), errors.Is(err, errs.ErrAlreadyPaid), errors.Is(err, errs.ErrAlreadyGraded), errors.Is(err, errs.ErrScheduleOverlap):
		status, message = 409, err.Error()
	default:
		a.Log.ErrorContext(r.Context(), "request failed", "request_id", r.Context().Value(requestIDKey), "err", err)
	}
	if academicPath(r.URL.Path) {
		now := time.Now()
		writeJSON(w, status, envelope.ApiResponse[any]{ErrorCode: http.StatusText(status), ErrorPath: r.URL.Path, Message: message, StatusCode: status, Timestamp: now.UnixMilli()})
		return
	}
	writeJSON(w, status, map[string]string{"error": message})
}
func academicPath(path string) bool {
	switch strings.TrimPrefix(path, "/api/v1/") {
	case "program-enrolments", "semesters", "course-offering-enrolments", "class-sessions", "biometrics/face":
		return true
	default:
		return false
	}
}
