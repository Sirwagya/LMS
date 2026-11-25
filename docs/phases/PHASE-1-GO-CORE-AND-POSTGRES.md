# Phase 1 — Infrastructure, PostgreSQL 17 Schema & Go Hexagonal Core

> **Prerequisite**: Review [Master System Design](file:///Users/sirwagyashekhar/Projects/LMS/docs/SYSTEM-DESIGN.md).
> **Phase Goal**: Stand up local PostgreSQL 17 and Redis 7 (Homebrew + Docker Compose), create the normalized 3NF relational schema replacing [`server/infrastructure/database.mjs`](file:///Users/sirwagyashekhar/Projects/LMS/server/infrastructure/database.mjs), migrate all seed data from [`server/seed.ts`](file:///Users/sirwagyashekhar/Projects/LMS/server/seed.ts), and implement the pure Go 1.25 Hexagonal core (`internal/domain` + `internal/application` + `postgres`/`memory` adapters) verified by shared Liskov Substitution contract tests.

---

## 1. Local Infrastructure Setup

Both **Homebrew services** (lowest RAM overhead on Apple Silicon) and **Docker Compose** (exact version parity in CI or containerized deployments) share the same connection contract via `.env`.

### 1.1 Homebrew Setup (macOS Apple Silicon)

```bash
brew install postgresql@17 redis golang-migrate
brew services start postgresql@17
brew services start redis

# Create application role and databases (app + integration test DB)
/opt/homebrew/opt/postgresql@17/bin/psql postgres -c "CREATE USER lms WITH PASSWORD 'lms_local_dev' CREATEDB;"
/opt/homebrew/opt/postgresql@17/bin/psql postgres -c "CREATE DATABASE lms_dev OWNER lms;"
/opt/homebrew/opt/postgresql@17/bin/psql postgres -c "CREATE DATABASE lms_test OWNER lms;"
redis-cli ping
```

### 1.2 `docker-compose.yml` (Reproducible Team / CI Environment)

```yaml
services:
  postgres:
    image: postgres:17-alpine
    container_name: vedam-lms-postgres
    environment:
      POSTGRES_USER: lms
      POSTGRES_PASSWORD: lms_local_dev
      POSTGRES_DB: lms_dev
    ports:
      - "5432:5432"
    volumes:
      - lms_pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U lms -d lms_dev"]
      interval: 5s
      timeout: 3s
      retries: 10

  redis:
    image: redis:7-alpine
    container_name: vedam-lms-redis
    command: ["redis-server", "--appendonly", "yes", "--maxmemory-policy", "allkeys-lru"]
    ports:
      - "6379:6379"
    volumes:
      - lms_redisdata:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10

volumes:
  lms_pgdata:
  lms_redisdata:
```

### 1.3 `.env.example`

```dotenv
APP_ENV=development
HTTP_ADDR=127.0.0.1:3001
DATABASE_URL=postgres://lms:lms_local_dev@127.0.0.1:5432/lms_dev?sslmode=disable
TEST_DATABASE_URL=postgres://lms:lms_local_dev@127.0.0.1:5432/lms_test?sslmode=disable
REDIS_URL=redis://127.0.0.1:6379/0
ASSET_STORAGE_DIR=./backend/.data/assets
GRADER_BINARY_PATH=./target/release/lms-grader
SESSION_TTL_HOURS=24
COOKIE_SECURE=false
```

---

## 2. Go Module & Package Architecture (`backend/`)

### 2.1 Dependencies (`backend/go.mod`)

Keep external dependencies minimal and audited:
- `github.com/jackc/pgx/v5` (native PostgreSQL protocol, `pgxpool`, `pgx.Tx` Unit of Work)
- `github.com/redis/go-redis/v9` (Redis 7 client + Lua `SCRIPT` execution + Pub/Sub + Streams)
- `github.com/google/uuid` (`UUIDv4` / `UUIDv7` generation)
- `golang.org/x/crypto` (`scrypt` backward verification + `argon2id` hashing)
- `golang.org/x/sync` (`singleflight.Group` cache stampede protection)

### 2.2 Bounded Contexts in `internal/domain`

`internal/domain` has **zero imports** outside the Go standard library (`context`, `errors`, `fmt`, `time`).

```go
// backend/internal/domain/errs/errors.go
package errs

import "errors"

var (
	ErrNotFound      = errors.New("resource not found")
	ErrConflict      = errors.New("resource conflict")
	ErrUnauthorized  = errors.New("authentication required")
	ErrForbidden     = errors.New("insufficient permissions")
	ErrValidation    = errors.New("validation failed")
	ErrRateLimited   = errors.New("rate limit exceeded")
	ErrAlreadyGraded = errors.New("graded submission is immutable")
	ErrAlreadyPaid   = errors.New("invoice already paid")
	ErrScheduleOverlap = errors.New("session overlaps an existing class")
)

type DomainError struct {
	Sentinel error
	Code     string
	Message  string
}

func (e *DomainError) Error() string { return e.Message }
func (e *DomainError) Unwrap() error { return e.Sentinel }

func New(sentinel error, code, message string) error {
	return &DomainError{Sentinel: sentinel, Code: code, Message: message}
}
```

#### Segregated Consumer-Owned Interfaces (ISP + DIP)

Notice that no single repository interface has more than 4 methods, and `UnitOfWork` binds PostgreSQL transaction context invisibly via `context.Context`:

```go
// backend/internal/domain/uow/uow.go
package uow

import "context"

// TxManager executes fn atomically. Domain and Application code never imports pgx.Tx.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(txCtx context.Context) error) error
}
```

```go
// backend/internal/domain/iam/ports.go
package iam

import (
	"context"
	"encoding/json"
	"time"
)

type Role string

const (
	RoleStudent Role = "student"
	RoleTeacher Role = "teacher"
	RoleAdmin   Role = "admin"
)

type CodingHandle struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Handle      string `json:"handle"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Initial     string `json:"initial"`
}

type User struct {
	ID           string
	Email        string
	Name         string
	Role         Role
	PasswordHash string
	Profile      json.RawMessage
	Version      int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type UserReader interface {
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	ListAll(ctx context.Context) ([]User, error)
}

type UserWriter interface {
	Create(ctx context.Context, user *User) error
	UpdateProfile(ctx context.Context, userID, name string, profile json.RawMessage, expectedVersion *int64) (int64, error)
	UpdatePasswordHash(ctx context.Context, userID, newHash string) error
}

type PasswordHasher interface {
	Hash(plain string) (string, error)
	Verify(plain, encoded string) (valid bool, needsRehash bool, err error)
}

type SessionStore interface {
	CreateSession(ctx context.Context, userID string, ttl time.Duration) (token string, err error)
	LookupSession(ctx context.Context, token string) (userID string, err error)
	RevokeAllForUser(ctx context.Context, userID string) error
}
```

```go
// backend/internal/domain/finance/ports.go
package finance

import (
	"context"
	"time"
)

type InvoiceStatus string

const (
	StatusPending InvoiceStatus = "Pending"
	StatusOverdue InvoiceStatus = "Overdue"
	StatusPaid    InvoiceStatus = "Paid"
)

type FeeInvoice struct {
	ID               string        `json:"id"`
	UserID           string        `json:"userId,omitempty"`
	InvoiceNo        string        `json:"invoiceNo"`
	Description      string        `json:"description"`
	Semester         string        `json:"semester"`
	Amount           int64         `json:"amount"`
	DueDate          string        `json:"dueDate"`
	PaidDate         *string       `json:"paidDate,omitempty"`
	Status           InvoiceStatus `json:"status"`
	ReceiptAvailable bool          `json:"receiptAvailable"`
	Simulated        bool          `json:"simulated,omitempty"`
	Version          int64         `json:"version,omitempty"`
}

type PaymentRecord struct {
	ID             string
	UserID         string
	FeeID          string
	IdempotencyKey string
	CreatedAt      time.Time
}

type InvoiceReader interface {
	ListForStudent(ctx context.Context, userID string) ([]FeeInvoice, error)
	ListAll(ctx context.Context) ([]FeeInvoice, error)
	GetOwnedForUpdate(ctx context.Context, feeID, userID string) (*FeeInvoice, error)
}

type InvoiceWriter interface {
	Create(ctx context.Context, invoice *FeeInvoice) error
	MarkPaid(ctx context.Context, feeID, paidDate string) error
}

type PaymentRecorder interface {
	Record(ctx context.Context, payment *PaymentRecord) error
}
```

```go
// backend/internal/domain/assessment/ports.go
package assessment

import (
	"context"
	"time"
)

type AssignmentKind string

const (
	KindWritten AssignmentKind = "written"
	KindCoding  AssignmentKind = "coding"
)

type TestCase struct {
	ID       int64  `json:"id,omitempty"`
	Input    string `json:"input"`
	Expected string `json:"expected"`
	Hidden   bool   `json:"hidden"`
	Ordinal  int    `json:"ordinal"`
}

type CodingProblem struct {
	Entrypoint   string            `json:"entrypoint"`
	Description  string            `json:"description"`
	InputFormat  string            `json:"inputFormat"`
	OutputFormat string            `json:"outputFormat"`
	Constraints  string            `json:"constraints"`
	Difficulty   string            `json:"difficulty"`
	StarterCode  map[string]string `json:"starterCode"`
	TestCases    []TestCase        `json:"testCases,omitempty"`
}

// PublicAssignment excludes hidden test cases at the type boundary (ISP).
type PublicAssignment struct {
	ID          string         `json:"id"`
	LectureID   int64          `json:"lectureId"`
	CourseCode  string         `json:"course"`
	Kind        AssignmentKind `json:"kind"`
	Title       string         `json:"title"`
	DueDate     string         `json:"dueDate"`
	Difficulty  string         `json:"difficulty,omitempty"`
	Entrypoint  string         `json:"entrypoint,omitempty"`
	Description string         `json:"description,omitempty"`
	InputFormat string         `json:"inputFormat,omitempty"`
	OutputFmt   string         `json:"outputFormat,omitempty"`
	Constraints string         `json:"constraints,omitempty"`
	StarterCode map[string]string `json:"starterCode,omitempty"`
	Examples    []TestCase     `json:"examples,omitempty"`
	TotalTests  int            `json:"totalTests,omitempty"`
}

type StaffAssignment struct {
	PublicAssignment
	AllTestCases []TestCase `json:"testCases,omitempty"`
}

type StudentAssignmentReader interface {
	GetPublicByID(ctx context.Context, assignmentID string) (*PublicAssignment, error)
}

type StaffAssignmentAuthor interface {
	GetWithHiddenTests(ctx context.Context, assignmentID string) (*StaffAssignment, error)
	CreateWritten(ctx context.Context, lectureID int64, title, dueDate string) (string, error)
	CreateCoding(ctx context.Context, lectureID int64, title, dueDate string, problem CodingProblem) (string, error)
}

type Submission struct {
	ID           string  `json:"id"`
	UserID       string  `json:"user_id,omitempty"`
	StudentName  string  `json:"name,omitempty"`
	AssignmentID string  `json:"assignment_id"`
	Course       string  `json:"course"`
	Answer       string  `json:"answer"`
	Filename     string  `json:"filename"`
	AssetID      *string `json:"-"`
	Score        *string `json:"score"`
	Feedback     *string `json:"feedback"`
	Created      string  `json:"created"`
}

type SubmissionRepository interface {
	ListForStudent(ctx context.Context, userID string) ([]Submission, error)
	ListAll(ctx context.Context) ([]Submission, error)
	GetByID(ctx context.Context, id string) (*Submission, error)
	GetScore(ctx context.Context, userID, assignmentID string) (*string, error)
	Upsert(ctx context.Context, sub *Submission) error
	Grade(ctx context.Context, submissionID, score, feedback string) error
}

type CodingAttempt struct {
	ID           string    `json:"id"`
	UserID       string    `json:"-"`
	AssignmentID string    `json:"assignmentId,omitempty"`
	Language     string    `json:"language"`
	Source       string    `json:"source"`
	Verdict      string    `json:"verdict"`
	Passed       int       `json:"passed"`
	Total        int       `json:"total"`
	ResultJSON   []byte    `json:"result"`
	CreatedAt    time.Time `json:"created"`
}

type CodingAttemptRepository interface {
	Save(ctx context.Context, attempt *CodingAttempt) error
	History(ctx context.Context, userID, assignmentID string, limit int) ([]CodingAttempt, error)
}
```

```go
// backend/internal/domain/sync/ports.go
package sync

import (
	"context"
	"encoding/json"
	"time"
)

type OutboxEvent struct {
	ID          string
	AggregateID string
	Topic       string
	Payload     json.RawMessage
	CreatedAt   time.Time
}

type ChangelogEntry struct {
	Seq        int64           `json:"seq"`
	Scope      string          `json:"scope"`
	EntityType string          `json:"entityType"`
	EntityID   string          `json:"entityId"`
	Op         string          `json:"op"`
	Payload    json.RawMessage `json:"payload"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}

type OutboxWriter interface {
	EnqueueEvent(ctx context.Context, event OutboxEvent) error
	AppendChangelog(ctx context.Context, scope, entityType, entityID, op string, payload json.RawMessage) (int64, error)
}

type SyncLogReader interface {
	ReadSince(ctx context.Context, userID string, cursor int64, limit int) (entries []ChangelogEntry, latestSeq int64, err error)
}
```

---

## 3. PostgreSQL 17 3NF Relational Schema (`backend/migrations/`)

Unlike the Node prototype in [`server/infrastructure/database.mjs`](file:///Users/sirwagyashekhar/Projects/LMS/server/infrastructure/database.mjs) which serialized the entire academic catalog into a single `settings('academic')` row and stored large files as SQLite BLOBs, PostgreSQL 17 uses normalized tables with foreign keys, partial/unique indexes, exclusion-ready schedule constraints, `sync_changelog`, and `outbox_events`.

### 3.1 `001_initial_schema.up.sql`

```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "btree_gist";

-- ============================================================================
-- 1. IAM: Users, Profiles, Coding Handles
-- ============================================================================
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('student', 'teacher', 'admin')),
    password_hash   TEXT NOT NULL,
    profile_json    JSONB NOT NULL DEFAULT '{}'::jsonb,
    version         BIGINT NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE coding_handles (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform_name   TEXT NOT NULL,
    handle          TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    profile_url     TEXT NOT NULL DEFAULT '',
    initial         TEXT NOT NULL DEFAULT '',
    sort_order      INT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_coding_handles_user ON coding_handles(user_id, sort_order);

-- ============================================================================
-- 2. Catalog: Semesters, Courses, Lectures, Assets, Resources, Schedule, Notices
-- ============================================================================
CREATE TABLE semesters (
    number          INT PRIMARY KEY CHECK (number BETWEEN 1 AND 8),
    label           TEXT NOT NULL,
    is_current      BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE courses (
    code            TEXT PRIMARY KEY,
    semester_number INT NOT NULL REFERENCES semesters(number),
    name            TEXT NOT NULL,
    instructor      TEXT NOT NULL DEFAULT '',
    credits         INT NOT NULL CHECK (credits BETWEEN 1 AND 10),
    next_topic      TEXT NOT NULL DEFAULT '',
    version         BIGINT NOT NULL DEFAULT 1,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_courses_semester ON courses(semester_number);

CREATE TABLE assets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    filename        TEXT NOT NULL,
    mime_type       TEXT NOT NULL,
    byte_size       BIGINT NOT NULL CHECK (byte_size >= 0),
    storage_key     TEXT NOT NULL,
    sha256          TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE lectures (
    id              BIGSERIAL PRIMARY KEY,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    lecture_code    TEXT NOT NULL,
    lecture_date    TEXT NOT NULL,
    month_label     TEXT NOT NULL DEFAULT '',
    unit_number     INT NOT NULL DEFAULT 1,
    title           TEXT NOT NULL,
    raw_title       TEXT NOT NULL,
    default_att     TEXT NOT NULL DEFAULT 'Awaiting' CHECK (default_att IN ('Present', 'Awaiting', 'Excused')),
    summary         TEXT NOT NULL DEFAULT '',
    slides          TEXT NOT NULL DEFAULT '',
    duration        TEXT NOT NULL DEFAULT '',
    recording_asset UUID NULL REFERENCES assets(id) ON DELETE SET NULL,
    sort_order      INT NOT NULL DEFAULT 0,
    version         BIGINT NOT NULL DEFAULT 1,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_lectures_course ON lectures(course_code, sort_order);

CREATE TABLE lecture_resources (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lecture_id      BIGINT NOT NULL REFERENCES lectures(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    resource_type   TEXT NOT NULL DEFAULT 'File',
    size_label      TEXT NOT NULL DEFAULT '',
    url             TEXT NULL,
    asset_id        UUID NULL REFERENCES assets(id) ON DELETE SET NULL,
    sort_order      INT NOT NULL DEFAULT 0
);
CREATE INDEX idx_lecture_resources_lecture ON lecture_resources(lecture_id, sort_order);

CREATE TABLE schedule_slots (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    week_start      DATE NOT NULL,
    slot_date       DATE NOT NULL,
    time_from       TIME NOT NULL,
    time_to         TIME NOT NULL,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    room            TEXT NOT NULL,
    topic           TEXT NOT NULL,
    slot_type       TEXT NOT NULL DEFAULT 'Lecture' CHECK (slot_type IN ('Lecture', 'Lab', 'Workshop')),
    published       BOOLEAN NOT NULL DEFAULT TRUE,
    CHECK (time_to > time_from),
    -- Prevent overlapping schedule slots on the exact same date at the DB level
    EXCLUDE USING gist (
        slot_date WITH =,
        tsrange(slot_date + time_from, slot_date + time_to, '[)') WITH &&
    )
);
CREATE INDEX idx_schedule_slots_date ON schedule_slots(slot_date, time_from);

CREATE TABLE notices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    placement       TEXT NOT NULL CHECK (placement IN ('dashboard', 'drawer')),
    tag             TEXT NOT NULL,
    tag_bg          TEXT NOT NULL DEFAULT '',
    time_label      TEXT NOT NULL,
    title           TEXT NOT NULL,
    description     TEXT NOT NULL,
    action_label    TEXT NULL,
    action_route    TEXT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_notices_placement ON notices(placement, created_at DESC);

-- ============================================================================
-- 3. Learning: Attendance, Progress, Discussions
-- ============================================================================
CREATE TABLE attendance_summaries (
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    attended        INT NOT NULL DEFAULT 0 CHECK (attended >= 0),
    total           INT NOT NULL DEFAULT 0 CHECK (total >= attended),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, course_code)
);

CREATE TABLE lecture_progress (
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lecture_id      BIGINT NOT NULL REFERENCES lectures(id) ON DELETE CASCADE,
    completed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, lecture_id)
);

CREATE TABLE discussions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lecture_id      BIGINT NOT NULL REFERENCES lectures(id) ON DELETE CASCADE,
    body            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_discussions_lecture ON discussions(lecture_id, created_at ASC);

-- ============================================================================
-- 4. Assessment: Assignments, Coding Problems, Test Cases, Submissions, Contests
-- ============================================================================
CREATE TABLE assignments (
    id              TEXT PRIMARY KEY,
    lecture_id      BIGINT NOT NULL REFERENCES lectures(id) ON DELETE CASCADE,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    kind            TEXT NOT NULL DEFAULT 'written' CHECK (kind IN ('written', 'coding')),
    title           TEXT NOT NULL,
    due_date        TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_assignments_lecture ON assignments(lecture_id);
CREATE INDEX idx_assignments_course ON assignments(course_code);

CREATE TABLE coding_problems (
    assignment_id   TEXT PRIMARY KEY REFERENCES assignments(id) ON DELETE CASCADE,
    entrypoint      TEXT NOT NULL CHECK (entrypoint IN ('twoSum', 'isValid', 'solve', 'stdin')),
    description     TEXT NOT NULL,
    input_format    TEXT NOT NULL,
    output_format   TEXT NOT NULL,
    constraints     TEXT NOT NULL,
    difficulty      TEXT NOT NULL CHECK (difficulty IN ('Easy', 'Medium', 'Hard')),
    starter_code    JSONB NOT NULL
);

CREATE TABLE coding_test_cases (
    id              BIGSERIAL PRIMARY KEY,
    assignment_id   TEXT NOT NULL REFERENCES coding_problems(assignment_id) ON DELETE CASCADE,
    ordinal         INT NOT NULL,
    input_data      TEXT NOT NULL,
    expected_output TEXT NOT NULL,
    is_hidden       BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (assignment_id, ordinal)
);

CREATE TABLE submissions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assignment_id   TEXT NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    answer          TEXT NOT NULL DEFAULT '',
    filename        TEXT NOT NULL DEFAULT '',
    asset_id        UUID NULL REFERENCES assets(id) ON DELETE SET NULL,
    score           TEXT NULL,
    feedback        TEXT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, assignment_id)
);
CREATE INDEX idx_submissions_user ON submissions(user_id);

CREATE TABLE coding_attempts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assignment_id   TEXT NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    language        TEXT NOT NULL CHECK (language IN ('python', 'java')),
    source          TEXT NOT NULL,
    verdict         TEXT NOT NULL,
    passed          INT NOT NULL CHECK (passed >= 0),
    total           INT NOT NULL CHECK (total >= passed),
    result_json     JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_coding_attempts_user_assignment
    ON coding_attempts(user_id, assignment_id, created_at DESC);

CREATE TABLE contest_attempts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    label           TEXT NOT NULL,
    title           TEXT NOT NULL,
    contest_date    TEXT NOT NULL,
    score           TEXT NOT NULL,
    percent         INT NOT NULL CHECK (percent BETWEEN 0 AND 100),
    rank_label      TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'Completed' CHECK (status IN ('Completed', 'Reviewed')),
    is_recent       BOOLEAN NOT NULL DEFAULT FALSE,
    ordinal         INT NOT NULL DEFAULT 0
);
CREATE INDEX idx_contest_attempts_user_course ON contest_attempts(user_id, course_code, ordinal);

-- ============================================================================
-- 5. Finance: Fee Invoices & Idempotent Payments
-- ============================================================================
CREATE TABLE fee_invoices (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invoice_no      TEXT NOT NULL,
    description     TEXT NOT NULL,
    semester        TEXT NOT NULL,
    amount          BIGINT NOT NULL CHECK (amount > 0),
    due_date        TEXT NOT NULL,
    paid_date       TEXT NULL,
    status          TEXT NOT NULL CHECK (status IN ('Paid', 'Overdue', 'Pending')),
    receipt_ready   BOOLEAN NOT NULL DEFAULT FALSE,
    simulated       BOOLEAN NOT NULL DEFAULT TRUE,
    version         BIGINT NOT NULL DEFAULT 1,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_fee_invoices_user ON fee_invoices(user_id);

CREATE TABLE payments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    fee_id          TEXT NOT NULL UNIQUE REFERENCES fee_invoices(id) ON DELETE RESTRICT,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- 6. Transactional Outbox & Offline Sync Changelog
-- ============================================================================
CREATE TABLE outbox_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_id    TEXT NOT NULL,
    topic           TEXT NOT NULL,
    payload         JSONB NOT NULL,
    published_at    TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_outbox_unpublished ON outbox_events(created_at ASC) WHERE published_at IS NULL;

CREATE TABLE sync_changelog (
    seq             BIGSERIAL PRIMARY KEY,
    scope           TEXT NOT NULL,
    target_user_id  UUID NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type     TEXT NOT NULL,
    entity_id       TEXT NOT NULL,
    op              TEXT NOT NULL CHECK (op IN ('upsert', 'delete')),
    payload         JSONB NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_sync_changelog_user_seq ON sync_changelog(target_user_id, seq);

CREATE TABLE processed_mutations (
    idempotency_key TEXT PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    response_json   JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 3.2 `001_initial_schema.down.sql`

```sql
DROP TABLE IF EXISTS processed_mutations;
DROP TABLE IF EXISTS sync_changelog;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS fee_invoices;
DROP TABLE IF EXISTS contest_attempts;
DROP TABLE IF EXISTS coding_attempts;
DROP TABLE IF EXISTS submissions;
DROP TABLE IF EXISTS coding_test_cases;
DROP TABLE IF EXISTS coding_problems;
DROP TABLE IF EXISTS assignments;
DROP TABLE IF EXISTS discussions;
DROP TABLE IF EXISTS lecture_progress;
DROP TABLE IF EXISTS attendance_summaries;
DROP TABLE IF EXISTS notices;
DROP TABLE IF EXISTS schedule_slots;
DROP TABLE IF EXISTS lecture_resources;
DROP TABLE IF EXISTS lectures;
DROP TABLE IF EXISTS assets;
DROP TABLE IF EXISTS courses;
DROP TABLE IF EXISTS semesters;
DROP TABLE IF EXISTS coding_handles;
DROP TABLE IF EXISTS users;
```

---

## 4. Transactional Unit of Work & Outbox Pattern

Every mutation wraps its PostgreSQL queries inside `postgres.TxManager.WithinTx`. Using `context.WithValue` keeps `pgx.Tx` strictly inside `adapters/postgres` so domain and application services obey **Dependency Inversion**:

```go
// backend/internal/adapters/postgres/tx.go
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type txKey struct{}

// Querier matches both *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type TxManager struct {
	pool *pgxpool.Pool
}

func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool}
}

func (m *TxManager) Querier(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok && tx != nil {
		return tx
	}
	return m.pool
}

func (m *TxManager) WithinTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if _, alreadyInTx := ctx.Value(txKey{}).(pgx.Tx); alreadyInTx {
		return fn(ctx)
	}
	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	txCtx := context.WithValue(ctx, txKey{}, tx)
	if err := fn(txCtx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
```

### Atomic Payment + Outbox + Changelog Example

```go
// backend/internal/application/finance/pay_invoice.go
package finance

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"vedam/lms/internal/domain/errs"
	domain "vedam/lms/internal/domain/finance"
	syncdomain "vedam/lms/internal/domain/sync"
	"vedam/lms/internal/domain/uow"
)

type PayInvoiceService struct {
	tx      uow.TxManager
	invRead domain.InvoiceReader
	invWrit domain.InvoiceWriter
	payRec  domain.PaymentRecorder
	outbox  syncdomain.OutboxWriter
	nowFn   func() time.Time
	uuidFn  func() string
}

func (s *PayInvoiceService) Pay(ctx context.Context, userID, feeID, idempotencyKey string) error {
	return s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		inv, err := s.invRead.GetOwnedForUpdate(txCtx, feeID, userID)
		if err != nil {
			return err
		}
		if inv.Status == domain.StatusPaid {
			return errs.New(errs.ErrAlreadyPaid, "INVOICE_PAID", "Invoice already paid.")
		}

		now := s.nowFn()
		paidDate := now.Format("2006-01-02")

		if err := s.payRec.Record(txCtx, &domain.PaymentRecord{
			ID:             s.uuidFn(),
			UserID:         userID,
			FeeID:          feeID,
			IdempotencyKey: idempotencyKey,
			CreatedAt:      now,
		}); err != nil {
			if errors.Is(err, errs.ErrConflict) {
				return errs.New(errs.ErrAlreadyPaid, "INVOICE_PAID", "Invoice already paid.")
			}
			return err
		}

		if err := s.invWrit.MarkPaid(txCtx, feeID, paidDate); err != nil {
			return err
		}

		payload, _ := json.Marshal(map[string]any{
			"feeId":    feeID,
			"userId":   userID,
			"paidDate": paidDate,
			"status":   domain.StatusPaid,
		})
		if err := s.outbox.EnqueueEvent(txCtx, syncdomain.OutboxEvent{
			ID:          s.uuidFn(),
			AggregateID: feeID,
			Topic:       "cache.invalidate.bootstrap",
			Payload:     payload,
			CreatedAt:   now,
		}); err != nil {
			return err
		}

		_, err = s.outbox.AppendChangelog(txCtx, "user:"+userID, "fee_invoice", feeID, "upsert", payload)
		return err
	})
}
```

---

## 5. Shared Liskov Substitution Contract Tests (`internal/domain/contracttest`)

To guarantee **LSP** and allow millisecond unit tests without a running database alongside true integration verification, every repository port has a single shared contract test suite run against both `adapters/memory` and `adapters/postgres`:

```go
// backend/internal/domain/contracttest/finance_contract.go
package contracttest

import (
	"context"
	"errors"
	"testing"

	"vedam/lms/internal/domain/errs"
	domain "vedam/lms/internal/domain/finance"
	"vedam/lms/internal/domain/uow"
)

type FinanceHarness struct {
	Tx           uow.TxManager
	InvoiceRead  domain.InvoiceReader
	InvoiceWrite domain.InvoiceWriter
	PaymentRec   domain.PaymentRecorder
	SeedUser     func(t *testing.T, id string)
	InjectFault  func(fn func())
}

func RunFinanceContract(t *testing.T, makeHarness func(t *testing.T) FinanceHarness) {
	t.Run("PaymentRollbackRestoresPendingInvoice", func(t *testing.T) {
		h := makeHarness(t)
		ctx := context.Background()
		h.SeedUser(t, "user-1")

		err := h.InvoiceWrite.Create(ctx, &domain.FeeInvoice{
			ID:          "fee-1",
			UserID:      "user-1",
			InvoiceNo:   "VDM-INV-001",
			Description: "Semester 3 Tuition",
			Semester:    "Semester 3",
			Amount:      125000,
			DueDate:     "2026-10-30",
			Status:      domain.StatusPending,
		})
		if err != nil {
			t.Fatalf("seed invoice: %v", err)
		}

		injected := errors.New("simulated post-payment fault")
		txErr := h.Tx.WithinTx(ctx, func(txCtx context.Context) error {
			if err := h.InvoiceWrite.MarkPaid(txCtx, "fee-1", "2026-11-25"); err != nil {
				return err
			}
			return injected
		})
		if !errors.Is(txErr, injected) {
			t.Fatalf("expected injected error, got %v", txErr)
		}

		got, err := h.InvoiceRead.GetOwnedForUpdate(ctx, "fee-1", "user-1")
		if err != nil {
			t.Fatalf("fetch invoice: %v", err)
		}
		if got.Status != domain.StatusPending {
			t.Fatalf("expected rollback to Pending, got %s", got.Status)
		}
	})

	t.Run("ForeignOwnershipReturnsNotFound", func(t *testing.T) {
		h := makeHarness(t)
		ctx := context.Background()
		_, err := h.InvoiceRead.GetOwnedForUpdate(ctx, "nonexistent", "other-user")
		if !errors.Is(err, errs.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}
```

---

## 6. Vedam Homepage API Domain Extensions & Seed Importer (`server/seed.ts` + Homepage Payloads -> PostgreSQL 17)

1. In addition to the legacy prototype schema, `001_initial_schema.up.sql` models the full Vedam LMS institutional hierarchy:
   - `campuses`, `programs`, `campus_person_roles` (`cpr_id`), `program_enrolments`, and `user_face_biometrics` (`has_face_data`, `face_json`, `embedding DOUBLE PRECISION[]`).
   - Extended `semesters` (`id > 0` supporting `1`, `2`, `6` `"Bootcamp Summer 2026"`, `3`, plus `code_label`, `campus_id`, `program_id`, `start_epoch`, `end_epoch`, `required_attendance`).
   - `course_offerings`, `course_offering_enrolments` (tracking granular `present_count`, `absent_count`, `late_count`, `leave_count`, `excused_count`, `awaiting_count`, `total_count`), and `class_sessions` (epoch `start_time`/`end_time`, `topic`, `label`, `room`).
   - Exposed cleanly via pure domain package [`internal/domain/academic`](file:///Users/sirwagyashekhar/Projects/LMS/backend/internal/domain/academic/ports.go) and [`internal/domain/envelope`](file:///Users/sirwagyashekhar/Projects/LMS/backend/internal/domain/envelope/envelope.go).
2. Deterministic idempotent seed runner (`backend/cmd/seed/main.go`, supporting `-reset`) embeds [`seed_data.json`](file:///Users/sirwagyashekhar/Projects/LMS/backend/internal/seed/seed_data.json) and [`vedam_homepage_seed.json`](file:///Users/sirwagyashekhar/Projects/LMS/backend/internal/seed/vedam_homepage_seed.json).
3. Populates all 4 semesters (`1, 2, 6, 3`), all courses, lectures (preserving original IDs and advancing `lectures_id_seq`), coding samples (`coding-two-sum`, `coding-valid-parentheses`), `course_offerings`, `course_offering_enrolments`, `class_sessions`, `user_face_biometrics`, `program_enrolments`, notices, `demoWeek` schedule slots, and the three seeded local accounts (`admin@vedam.local`, `teacher@vedam.local`, `student@vedam.local` with password `VedamLocal123!`).

---

## 7. Phase 1 Deliverables & Verification Gate

- **Deliverables**:
  - `docker-compose.yml` & `.env.example`
  - Homebrew `postgresql@17` + `redis` running locally
  - `backend/go.mod`, `backend/migrations/001_initial_schema.{up,down}.sql`, `backend/cmd/seed/main.go`
  - `backend/internal/domain/*`, `backend/internal/application/*`, `backend/internal/adapters/{postgres,memory,clock}`
  - `backend/internal/domain/contracttest/*`
- **Verification Commands**:
  ```bash
  cd backend
  go test -race ./internal/domain/... ./internal/application/... ./internal/adapters/memory/...
  TEST_DATABASE_URL="postgres://lms:lms_local_dev@127.0.0.1:5432/lms_test?sslmode=disable" \
    go test -race -tags=integration ./internal/adapters/postgres/...
  go vet ./...
  ```
- **Exit Criteria**: All migrations apply cleanly up and down; seed importer runs twice idempotently; every LSP contract test passes identically on `memory` and `postgres` with `-race` enabled.
