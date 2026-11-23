# Vedam LMS — local student and staff portals

A working local learning management system built with React, TypeScript, Vite and a Node.js / SQLite backend. The shared 60/30/10 theme supports light and dark modes.

## Run

Requires **Node.js 24 or newer** (uses built-in SQLite and TypeScript stripping).

```sh
npm ci
npm run dev
```

Open http://127.0.0.1:5173. The development command starts the API on port 3001 and Vite on port 5173. For a compiled app with one server:

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

`server/index.mjs` serves authenticated APIs and the compiled frontend. SQLite stores users, password hashes, sessions, academic catalog, attendance, submissions/attachments, discussions, progress, invoices, simulated payments and uploaded materials. Data persists at `.data/lms.sqlite` and is excluded from Git. The database is initialized once from `server/seed.ts`. Back up the entire `.data` directory while the app is stopped. Do not commit it.

`PORT` overrides the backend port; `LMS_DATA_DIR` overrides its data directory. If the API port changes during Vite development, also update the proxy in `vite.config.ts`.

Authentication uses scrypt password hashes, expiring HttpOnly SameSite cookies, server-enforced roles and ownership checks. Changing a password revokes other sessions. API writes accept JSON from local origins only; request/file size limits and authentication attempt limits are enforced.

This version is intended for local demo use. It does not connect to an institution, email provider, external video host, payment gateway or production identity service. Seeded historical academic records and receipts are illustrative. There is no email password recovery; use the documented sample accounts or another locally created account. Deleting/archiving historical academic records is intentionally not offered in the UI.

## Validation

```sh
npm run build
npm run lint
npm test
```

The integration test uses an isolated temporary SQLite database and checks authentication, student/staff authorization, new-user isolation, courses, lectures, assignments, uploads/download ownership, submission grading, attendance, discussions, scheduling conflicts, simulated payments/duplicate protection, receipts, persistence after restart, profile updates, password changes and logout.

## Design and maintainability

See [architecture and SOLID decisions](docs/ARCHITECTURE.md) for module boundaries, repository ports, dependency injection and transaction semantics.

## Java / Python coding assignments

Open **Assignments → Solve coding challenge**. The coding workspace includes a CodeMirror editor with highlighting, line numbers, indentation, Java/Python switching, per-user local drafts, example/custom-input runs, hidden-test submission judging, diagnostics and saved submission history. Accepted submissions receive a server-calculated grade and appear in the teacher's submissions list. Two sample problems (Two Sum and Valid Parentheses) are added automatically to existing databases. Teachers can create more challenges in **Assignments → Create coding challenge**, including public and hidden stdin/stdout test cases.

Use `public class Main` in Java, or a normal stdin/stdout program in Python. Java/Python run using the machine's installed runtimes. This local implementation requires **macOS**, `/usr/bin/sandbox-exec`, Python (default `/opt/homebrew/bin/python3`) and a JDK discoverable by `/usr/libexec/java_home`. `LMS_PYTHON` and `JAVA_HOME` override runtime locations. Unsupported hosts fail closed; there is no unrestricted execution fallback.

Each run uses an isolated temporary directory with a clean environment. The OS sandbox blocks networking, private user-file access and child-process creation. CPU/wall-clock, source/input/output, open-file, file-count and concurrent-run limits are enforced; the parent monitors memory and temporary files. Java uses a limited heap. The macOS memory monitor is polling-based, not a hard container memory quota. This is a **local teaching runner**, not a production multi-tenant judge; sandbox-exec is a deprecated macOS interface. Use an isolated container/VM execution provider for public deployment.

Hidden test inputs, expected answers and output are removed from student API responses. Only staff can configure them. Run checks examples; Submit checks all cases. A custom run displays stdout without an expected-answer verdict. Up to 20 recent submission attempts are shown. An accepted/graded assignment is locked; code can still be edited and run for practice.


### Method-only solutions and history

Two Sum now exposes only `Solution.twoSum(nums, target)` (`int[]` return in Java, list of indices in Python); Valid Parentheses exposes `Solution.isValid(s)` returning a boolean. New teacher-created challenges expose `Solution.solve(input)` returning a string. Imports, input parsing, the executable entry point and output serialization are supplied by the runner. Existing custom stdin/stdout challenges keep their original contract. Sample assignments migrate in place on restart; previous submissions and old drafts are preserved, with a separate method-editor draft namespace.

The problem pane has dedicated Description / Submissions tabs. The submission panel lists verdict, language, passed tests, time, timestamp and submitted source; select any attempt for its code and test details, copy it, or load compatible code into the editor. Older full-program attempts remain available for reference. Repeated coding submissions are supported after acceptance; unsuccessful attempts do not remove the last accepted grade. Written assignments retain their existing graded-submission lock.


### Subject → lecture → learning content

A subject opens a dedicated lecture catalog. Each lecture owns its recording, notes/materials, assignments and attendance section. The header breadcrumb links back through the subject and parent lecture. Written and coding assignments use the canonical address `#/subjects/{subject}/lectures/{lecture}/assignments/{assignment}`; coding screens return to their parent lecture. The assignments overview remains a shortcut into the same hierarchy. Legacy `#/coding/{assignment}` links redirect to the canonical nested URL. Route resolution rejects mismatched subject/lecture/assignment combinations. Searching the lecture list does not change the lecture displayed at the current URL.
