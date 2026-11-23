# Local LMS architecture

The implementation uses composition and small factory functions, rather than inheritance. The server's composition root is `server/index.mjs`.

## Layers and patterns

| Boundary | Responsibility | Pattern |
| --- | --- | --- |
| `server/infrastructure/database.mjs` | SQLite schema and database lifecycle | Database adapter |
| `server/infrastructure/repositories.mjs` | SQL behind named domain operations | Repository |
| `server/domain/catalog.mjs` | Academic catalog state and persistence | Catalog facade |
| `server/domain/workspace.mjs` | Compose the signed-in user's records | Application service |
| `server/domain/payments.mjs` | Atomic simulated payment + invoice transition | Service + Unit of Work |
| `server/domain/auth.mjs` | Roles, student lookup, authentication rate limits | Authorization policy |
| `server/http/` | HTTP parsing, cookies, errors, authorization boundary, static assets | Transport adapters |
| `server/routes/` | Small feature route factories with injected ports | Route registry + dependency injection |
| `src/portal/staff/` | Separate staff feature views and reusable forms | Component composition |
| `src/portal/api.ts` | Request/error handling and file conversion | API facade |
| `src/portal/session.ts` | Session contract, submissions and learning progress | Context provider |
| `src/data/lmsData.ts` | Existing screen compatibility facade; hydrated from the API before mount | Facade |

## SOLID decisions

- **Single responsibility:** Server bootstrapping, transport, authorization, persistence and individual features live in different modules. Staff navigation is separate from course, attendance, grading, fee and people views.
- **Open/closed:** Feature routes register predicates, access requirements and handlers. The HTTP dispatcher handles new registered routes without new endpoint-specific branches. No payment-provider switch is introduced for a local simulation.
- **Liskov substitution:** The application favors composition; no artificial inheritance hierarchy is introduced. The payment service can use SQLite adapters or in-memory repository ports with the same operations, verified in tests.
- **Interface segregation:** Factories destructure only the repository/service ports they use. Staff view props specify only the data/actions needed by that view. HTTP sessions are separate from domain authorization policies.
- **Dependency inversion:** Feature modules receive repositories and services from the composition root. SQL appears only in infrastructure; domain payment logic imports neither SQLite nor HTTP.

## Payment repository contract

`createPaymentService` requires three narrow ports:

- `payments.create(id, userId, feeId, createdAt)` records a simulated payment.
- `fees.update(serializedInvoice, feeId)` updates the invoice ledger.
- `transaction(operation)` executes synchronously and atomically: all writes commit together or roll back together. Async operations must not be passed to this synchronous SQLite unit of work.

The rollback test injects a SQLite trigger failure after the payment insert and verifies that the invoice and payment remain unchanged. The substitution test exercises the same service with in-memory ports.

## Existing UI boundary

The original student screens use a shared academic-data module. Hydration is kept behind `hydrateAcademic` so the existing UI can consume server snapshots without a wholesale UI rewrite. New flows use explicit session and API contracts. This compatibility layer is not a general-purpose global state architecture; future integrations can replace it behind the facade.

## Local scope

All student accounts share the sample catalog. Attendance, grades, submissions, progress, profiles and invoices are user-specific. Teachers share curriculum management permissions; invoice creation and account creation are administrator-only. Production cohort enrollment, teacher-to-course assignment, external payments, email recovery and remote deployment are outside this local sample version.

## Coding judge extension

The Java/Python feature registers routes through the existing route registry. `coding/service.mjs` owns assignment rules, rate limiting and submission persistence; `coding/judge.mjs` orchestrates compilation and test evaluation through an injected runner; `coding/languages.mjs` is the language strategy registry; `coding/sandbox.mjs` adapts local macOS process isolation; `coding/repository.mjs` persists attempts. Student catalog projection strips hidden tests on the server. The editor, workspace state hook and test-results renderer are separate frontend modules. Language parsers and the coding screen load on demand.


`coding/harness.mjs` defines solution-method contracts and platform-owned executable drivers. The judge accepts a trusted assignment entrypoint, wraps the submitted method, and executes it through the existing runner. Client submissions cannot choose a different method contract. Persisted history stores only student source and the result, while the shared SubmissionHistory component renders attempt selection separately from workspace execution state.


`src/utils/learningRoutes.ts` owns route construction, parent lookup and subject/lecture/assignment validation. Subject catalog, lecture detail, written assignment detail, coding workspace and breadcrumbs consume that shared contract. Pure route tests cover parent resolution, mismatched ownership and old bookmark migration. Lecture attendance is displayed separately from completion; new students do not inherit the demo student's lecture-attendance statuses.
