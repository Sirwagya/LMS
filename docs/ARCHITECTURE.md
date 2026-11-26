# Local LMS architecture

The Go API and worker wire adapters in `backend/internal/runtime`. Domain and application packages depend on repository interfaces and the standard library.

| Boundary | Responsibility |
| --- | --- |
| `backend/cmd/api` | HTTP/SSE server and compiled SPA |
| `backend/cmd/worker` | Transactional outbox relay and Redis Streams grading consumer |
| `backend/internal/adapters/http` | Request IDs, logging, request validation, authenticated routes, role policies and idempotency |
| `backend/internal/adapters/postgres` | Repositories and unit of work |
| `backend/internal/adapters/redis` | Sessions, throttling, cache generations, job queue and notifications |
| `backend/internal/adapters/grader` | JSON-over-stdin/stdout client for the Rust `lms-grader` binary and the remote contest judge HTTP client |
| `backend/internal/adapters/storage` | Local asset storage |
| `backend/internal/application` | Workspace projection, learning, staff operations, contests and simulated payment |
| `backend/internal/domain/contest` | Contest authoring, attempts, integrity events and versioned results |
| `src-tauri/src/proctor.rs` | macOS proctoring commands: lockdown, fullscreen, app monitoring and screen capture |
| `src/portal/api.ts` | Frontend request and error handling |

A payment writes its ledger entry, updates its invoice and records its changelog and outbox event in one PostgreSQL transaction. The HTTP idempotency record commits with the mutation. Redis caches the response for fast retries. The worker claims unpublished events with `FOR UPDATE SKIP LOCKED`, invalidates projections, publishes SSE notifications and marks the batch published. SSE deliveries can repeat, and offline clients use the PostgreSQL changelog cursor to catch up.

The shared repository contracts exercise the memory and PostgreSQL adapters. HTTP parity tests cover the replacement Go API without changing frontend routes.

## Existing UI boundary

The original student screens use a shared academic-data module. Hydration is kept behind `hydrateAcademic` so the existing UI can consume server snapshots without a wholesale UI rewrite. New flows use explicit session and API contracts. This compatibility layer is not a general-purpose global state architecture; future integrations can replace it behind the facade.

## Local scope

All student accounts share the sample catalog. Attendance, grades, submissions, progress, profiles and invoices are user-specific. Teachers share curriculum management permissions; invoice creation and account creation are administrator-only. Production cohort enrollment, teacher-to-course assignment, external payments, email recovery and remote deployment are outside this local sample version.

## Coding judge extension

The Go API selects examples, hidden cases or custom input from the trusted assignment contract and invokes the Rust `lms-grader` binary (`GRADER_BINARY_PATH`) over JSON stdin/stdout through `backend/internal/adapters/grader`. The grader keeps the macOS sandbox and platform-owned solution drivers. Hidden case details are removed before returning or persisting results. PostgreSQL stores coding attempts and accepted grades. Legacy evaluation requests return a synchronous verdict. A v1 submission with `Prefer: respond-async` returns a job resource that the worker processes from Redis Streams.

`src/utils/learningRoutes.ts` owns route construction, parent lookup and subject/lecture/assignment validation. Subject catalog, lecture detail, written assignment detail, coding workspace and breadcrumbs consume that shared contract. Pure route tests cover parent resolution, mismatched ownership and old bookmark migration. Lecture attendance is displayed separately from completion; new students do not inherit the demo student's lecture-attendance statuses.

## Contests and integrity

Contest features are gated behind the `LMS_CONTEST_*` flags (`AUTHORING`, `ATTEMPTS`, `GRADING`, `INTEGRITY`, `RESULTS`). Authoring, attempts, scoring and publication follow the contracts in [CONTESTS.md](CONTESTS.md): contest revisions freeze questions and rules, attempts use server-authoritative timing and leases, and results are versioned and released atomically with cohort leaderboards.

Scored coding questions bypass the local grader: the worker posts submissions to a separate remote judge endpoint (`LMS_CONTEST_JUDGE_URL`) through `adapters/grader.RemoteContestJudge` using a bearer token and the official submission UUID as an idempotency key. Infrastructure failures are retried; unresolvable failures mark the attempt **Needs review** for faculty.

Proctored attempts rely on the macOS Tauri app. Its `proctor` module reports fullscreen state, focus changes, running applications and screen capture; the server records lockdown warnings and, at the third, freezes the attempt with an automatic submission and queues it for faculty review in `ContestIntegrityReview`. The web app runs the same attempt UI without native lockdown.
