# Vedam LMS — local student and staff portals

A working local learning management system built with React, TypeScript, Vite and a Go, PostgreSQL and Redis backend. The shared 60/30/10 theme supports light and dark modes.

## Run

Requires **Node.js 24+**, **Go 1.26+**, **PostgreSQL 17** and **Redis 7**. Use the services in `docker-compose.yml` or the local setup in [Phase 1](docs/phases/PHASE-1-GO-CORE-AND-POSTGRES.md#1-local-infrastructure-setup). To initialize the database and start development:

```sh
npm ci
(cd backend && go run ./cmd/seed)
npm run dev
```

Open http://127.0.0.1:5173. The development command starts the Go API on port 3001, the outbox and grading worker, and Vite on port 5173. For a compiled app with one server:

```sh
npm run build
npm start
```

Open http://127.0.0.1:3001. Both servers bind to localhost. Stop with Ctrl+C.

## Demo accounts

| Portal | Email | Password |
| --- | --- | --- |
| Student | student@vedam.local | VedamLocal123! |
| Teacher | teacher@vedam.local | VedamLocal123! |
| Administrator | admin@vedam.local | VedamLocal123! |

These are intentionally public **local sample accounts**. The student demo has seeded academic records. New student registrations start with zero attendance, no invoices and no results. All students can access the local course catalog. Administrators create additional student and teacher accounts through People. Public registration always creates a student; it cannot create staff accounts.

## Student workflows

- Sign in / register, sign out, and change password.
- Search courses, browse semesters and lecture notes, download included sample notes and teacher-uploaded materials, watch uploaded MP4/WebM recordings.
- Mark lectures complete; ask questions in lecture discussions.
- Submit assignment answers/project links with an optional attachment (10 MB), update ungraded work, view teacher grades and feedback. Graded work is locked.
- Review attendance, performance charts and the attendance planner. Recent course results are read-only staff evaluations.
- Browse published calendar/day agenda/weekly schedule and notices.
- Save account details, coding profiles, profile photo and banner to the backend. No profile is stored in browser localStorage; only the theme preference is.
- Review invoices, simulate payment and download clearly labeled sample receipts. No card information or real payments are involved. Duplicate payments are rejected.

## Teacher and administrator workflows

- Staff overview with students, course counts and submissions awaiting review.
- Create courses; update names and instructors.
- Publish and edit lecture notes. Upload downloadable materials and small MP4/WebM recordings (10 MB per file).
- Publish assignments, review student text/attachments and enter grades with feedback.
- Record cumulative student attendance with count validation.
- Publish dated class sessions; overlapping sessions are rejected. Edit announcements and publish notices.
- Administrators additionally issue student invoices and create student/teacher accounts. Teachers cannot access these admin-only operations.

## Data and implementation

`backend/cmd/api` serves the authenticated API and compiled frontend. PostgreSQL stores accounts, catalog, course activity, grading and the simulated payment ledger. Redis stores sessions, rate limits, cached workspace projections and the grading queue. Uploaded files live in `backend/.data/assets` by default. The seed command initializes local demo data. The demo biometric fixture uses a synthetic embedding.

Copy `.env.example` to `.env` to customize connection URLs, `HTTP_ADDR`, asset storage and cookie security. Process environment variables take precedence over the file. If the API port changes during Vite development, update the proxy in `vite.config.ts`.

The web app authenticates with an expiring HttpOnly SameSite cookie. Native clients can use the bearer token returned by login. Set `SESSION_SLIDING=true` to renew the session on authenticated requests. New passwords use Argon2id, and successful login upgrades legacy scrypt hashes. Password changes and logout revoke existing sessions. The API enforces role checks, data ownership, upload limits and Redis-backed rate limits.

The Go seed command initializes PostgreSQL from the checked-in fixtures. Existing SQLite files under `.data` are preserved on disk. Moving records created in the former SQLite app into PostgreSQL requires a separate data migration.

This version is intended for local demo use. It does not connect to an institution, email provider, external video host, payment gateway or production identity service. Seeded historical academic records and receipts are illustrative. There is no email password recovery; use the documented sample accounts or another locally created account. Deleting/archiving historical academic records is intentionally not offered in the UI.

## Validation

```sh
npm run build
npm run lint
npm test
(cd backend && go test -race -tags=integration ./...)
```

HTTP tests use temporary schemas in the PostgreSQL database specified by `TEST_DATABASE_URL` (default `lms_test`), and start disposable Redis instances using `redis-server`. Set `TEST_REDIS_SERVER` to exercise a specific Redis version. The PostgreSQL repository contract tests reset the application schema in `lms_test`. The HTTP suite covers the student and staff workflows against Go, including persistence after restart, grading, authorization, sync and payments.

## Design and maintainability

See [architecture and SOLID decisions](docs/ARCHITECTURE.md) for module boundaries, repository ports, dependency injection and transaction semantics.

## Java / Python coding assignments

Open **Assignments → Solve coding challenge**. The coding workspace includes a CodeMirror editor with highlighting, line numbers, indentation, Java/Python switching, per-user local drafts, example/custom-input runs, hidden-test submission judging, diagnostics and saved submission history. Accepted submissions receive a server-calculated grade and appear in the teacher's submissions list. The Go seed includes two sample problems, Two Sum and Valid Parentheses. Teachers can create more challenges in **Assignments → Create coding challenge**, including public and hidden stdin/stdout test cases.

Use `public class Main` in Java, or a normal stdin/stdout program in Python. Java/Python run using the machine's installed runtimes. This local implementation requires **macOS**, `/usr/bin/sandbox-exec`, Python (default `/opt/homebrew/bin/python3`) and a JDK discoverable by `/usr/libexec/java_home`. `LMS_PYTHON` and `JAVA_HOME` override runtime locations. Unsupported hosts fail closed; there is no unrestricted execution fallback.

Each run uses an isolated temporary directory with a clean environment. The OS sandbox blocks networking, private user-file access and child-process creation. CPU/wall-clock, source/input/output, open-file, file-count and concurrent-run limits are enforced; the parent monitors memory and temporary files. Java uses a limited heap. The macOS memory monitor is polling-based, not a hard container memory quota. This is a **local teaching runner**, not a production multi-tenant judge; sandbox-exec is a deprecated macOS interface. Use an isolated container/VM execution provider for public deployment.

Hidden test inputs, expected answers and output are removed from student API responses. Only staff can configure them. Run checks examples; Submit checks all cases. A custom run displays stdout without an expected-answer verdict. Up to 20 recent submission attempts are shown. Written assignments lock after grading. Coding assignments accept repeated attempts.


### Method-only solutions and history

Two Sum now exposes only `Solution.twoSum(nums, target)` (`int[]` return in Java, list of indices in Python); Valid Parentheses exposes `Solution.isValid(s)` returning a boolean. New teacher-created challenges expose `Solution.solve(input)` returning a string. Imports, input parsing, the executable entry point and output serialization are supplied by the runner. Existing custom stdin/stdout challenges keep their original contract. The method editor uses a separate draft namespace so full-program drafts remain available.

The problem pane has dedicated Description / Submissions tabs. The submission panel lists verdict, language, passed tests, time, timestamp and submitted source; select any attempt for its code and test details, copy it, or load compatible code into the editor. Older full-program attempts remain available for reference. Repeated coding submissions are supported after acceptance; unsuccessful attempts do not remove the last accepted grade. Written assignments retain their existing graded-submission lock.


### Subject → lecture → learning content

A subject opens a dedicated lecture catalog. Each lecture owns its recording, notes/materials, assignments and attendance section. The header breadcrumb links back through the subject and parent lecture. Written and coding assignments use the canonical address `#/subjects/{subject}/lectures/{lecture}/assignments/{assignment}`; coding screens return to their parent lecture. The assignments overview remains a shortcut into the same hierarchy. Legacy `#/coding/{assignment}` links redirect to the canonical nested URL. Route resolution rejects mismatched subject/lecture/assignment combinations. Searching the lecture list does not change the lecture displayed at the current URL.
