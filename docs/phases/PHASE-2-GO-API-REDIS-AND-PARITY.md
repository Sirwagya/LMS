# Phase 2 — Go HTTP/SSE API, Redis Services & Strangler Fig Parity

> **Prerequisite**: [Phase 1](file:///Users/sirwagyashekhar/Projects/LMS/docs/phases/PHASE-1-GO-CORE-AND-POSTGRES.md) complete (`postgresql@17` & `redis` running, schema migrated, Go core + LSP contract tests passing).
> **Phase Goal**: Build the Go HTTP/SSE server (`backend/cmd/api`), background Outbox/Grading worker (`backend/cmd/worker`), and Redis 7 adapters; achieve 100% API parity with every endpoint in [`server/routes/`](file:///Users/sirwagyashekhar/Projects/LMS/server/index.mjs); verify against existing integration suites; switch [`package.json`](file:///Users/sirwagyashekhar/Projects/LMS/package.json) to the Go backend; and retire `server/`.


The Go API now serves the legacy routes and their v1 aliases. The original HTTP workflow scenario lives in [tests/lms.test.mjs](../../tests/lms.test.mjs). Go race-enabled integration tests cover Redis and PostgreSQL behavior, including duplicate payments, batch sync retries, event delivery and grading jobs. The SQLite-only test units were replaced by the Go repository and payment tests. The Node HTTP server has been retired. Phase 2 invokes the existing macOS sandbox through the temporary [grader bridge](../../grader/bridge.mjs).

For asynchronous grading, send `Prefer: respond-async` on a v1 coding submission. The API returns `202` and a `Location` header for polling the job. The worker also publishes `grading.completed`. The default remains a synchronous verdict.

Sync push accepts profile, lecture progress and discussion mutations. Results include `status`, `serverVersion`, `serverPayload` and an optional `reason`. An optional progress `clientTimestamp` is Unix milliseconds. The newer edit wins and the response includes the current state.

---

## 1. Middleware Chain of Responsibility (`adapters/http/middleware.go`)

Every incoming HTTP request passes through an explicit composable middleware chain:

1. **RequestID**: Reads `X-Request-ID` or assigns a fresh UUID; attaches to response header and `slog` context.
2. **StructuredLogger**: Logs method, path, status, duration (`ms`), `request_id`, and `user_id` via `log/slog` JSON handler.
3. **Recovery**: Catches panics inside handlers, logs stack trace, and emits `500 {"error":"Internal server error."}`.
4. **RateLimiter (Lua + Redis)**: Sliding-window rate limit per IP/actor (`30 req/min` on `/api/login` & `/api/register`, `20 req/min` on `/api/coding/evaluate`). Returns `429` with `Retry-After`.
5. **SessionAuthenticator**: Resolves either the `lms_session` HttpOnly cookie (Web / parity mode) or `Authorization: Bearer <token>` header (macOS Tauri Keychain mode) via `RedisSessionStore`.
6. **RoleGuard (`RBACPolicy`)**: Enforces `public`, `authenticated`, `student`, `staff` (`teacher | admin`), or `admin` (`401` when unauthenticated, `403` when role-mismatched).
7. **IdempotencyMiddleware**: Checks `Idempotency-Key` header on `/api/payments` and `/api/v1/sync/push` mutations to guarantee at-most-once side effects.

---

## 2. Redis 7 Keyspace & Adapters (`internal/adapters/redis`)

### 2.1 Keyspace Inventory

| Key Pattern | Redis Type | TTL / Retention | Purpose |
| :--- | :--- | :--- | :--- |
| `sess:{token_sha256}` | `HASH` (`user_id`, `role`, `expires_ms`) | 24 hours (sliding configurable) | O(1) session validation without hitting PostgreSQL on every request |
| `user_sess:{user_id}` | `SET` of `{token_sha256}` | 24 hours | Enables instant revocation of all active sessions on `/api/logout` and `/api/password` |
| `rl:{bucket}:{actor}` | `ZSET` (score = Unix ms) | Window duration (`60s`) | Atomic sliding-window rate limiter via Lua script |
| `lock:judge:{user_id}` | `STRING` (`"1"`) | `30s` safety expiry (`DEL` in `defer`) | Enforces max 1 concurrent evaluation per student (`429 Wait for your current execution to finish.`) |
| `cache:catalog:v{ver}` | `STRING` (JSON) | 10 minutes + outbox invalidation | Shared catalog data cached behind `singleflight` |
| `cache:bootstrap:{user_id}:v{ver}` | `STRING` (JSON) | 5 minutes + outbox invalidation | Personalized CQRS bootstrap projection served in `< 2 ms` |
| `idem:{user_id}:{key}` | `STRING` (cached response JSON) | 24 hours | Fast idempotency cache backed by PostgreSQL `processed_mutations` table |
| `stream:grading` | `STREAM` (group `graders`) | Bounded (`MAXLEN ~ 5000`) | Durable async coding grading job queue consumed by `cmd/worker` |
| `pubsub:user:{user_id}` | `PUBSUB` channel | Ephemeral | Real-time push notifications to SSE `/api/v1/events` connections |

### 2.2 Atomic Sliding-Window Rate Limiter (Lua Script)

```lua
-- KEYS[1] = rl:{bucket}:{actor}
-- ARGV[1] = now_ms
-- ARGV[2] = window_ms (e.g. 60000)
-- ARGV[3] = max_requests (e.g. 30 or 20)
-- ARGV[4] = unique_member_id

local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]
local cutoff = now - window

redis.call("ZREMRANGEBYSCORE", key, "-inf", cutoff)
local count = redis.call("ZCARD", key)

if count >= limit then
    local oldest = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
    local retry_ms = 1000
    if #oldest >= 2 then
        retry_ms = math.max(1000, tonumber(oldest[2]) + window - now)
    end
    return {0, count, math.ceil(retry_ms / 1000)}
end

redis.call("ZADD", key, now, member)
redis.call("PEXPIRE", key, window)
return {1, count + 1, 0}
```

### 2.3 Cache-Aside + Singleflight Decorator (`BootstrapQuery`)

```go
// backend/internal/adapters/redis/cached_bootstrap.go
package redisadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"vedam/lms/internal/domain/iam"
)

type BootstrapReader interface {
	LoadBootstrap(ctx context.Context, user *iam.User) (json.RawMessage, error)
}

type CachedBootstrapReader struct {
	next   BootstrapReader
	client *redis.Client
	group  singleflight.Group
	ttl    time.Duration
}

func NewCachedBootstrapReader(next BootstrapReader, client *redis.Client, ttl time.Duration) *CachedBootstrapReader {
	return &CachedBootstrapReader{next: next, client: client, ttl: ttl}
}

func (c *CachedBootstrapReader) LoadBootstrap(ctx context.Context, user *iam.User) (json.RawMessage, error) {
	key := fmt.Sprintf("cache:bootstrap:%s:v1", user.ID)
	if raw, err := c.client.Get(ctx, key).Bytes(); err == nil && len(raw) > 0 {
		return json.RawMessage(raw), nil
	}

	val, err, _ := c.group.Do(user.ID, func() (any, error) {
		raw, err := c.next.LoadBootstrap(ctx, user)
		if err != nil {
			return nil, err
		}
		_ = c.client.Set(context.Background(), key, []byte(raw), c.ttl).Err()
		return raw, nil
	})
	if err != nil {
		return nil, err
	}
	return val.(json.RawMessage), nil
}
```

### 2.4 Outbox Relay & Dual Password Hashing

1. **Outbox Worker (`cmd/worker`)**: Polls `outbox_events WHERE published_at IS NULL ORDER BY created_at LIMIT 50 FOR UPDATE SKIP LOCKED`, invalidates `cache:bootstrap:{userID}:v1` / `cache:catalog:v1`, publishes JSON events to `pubsub:user:{userID}`, and stamps `published_at = NOW()`.
2. **Dual Password Hasher (`adapters/crypto`)**:
   - Supports verifying existing Node `scryptSync` hashes (`"<32-hex-salt>:<128-hex-digest>"`) using constant-time comparison.
   - Hashes all newly registered users and password changes using **Argon2id** (`"$argon2id$v=19$..."`) and transparently upgrades verified legacy `scrypt` logins (`needsRehash == true`).

---

## 3. Complete Strangler Fig API Specification (`/api/*` + `/api/v1/*`)

Every existing `/api/*` route preserves 100% payload, validation, and status-code compatibility with [`server/routes/*.mjs`](file:///Users/sirwagyashekhar/Projects/LMS/server/index.mjs) while exposing `/api/v1/*` aliases and sync/SSE extensions.

### 3.1 Health & Authentication

| Method | Path | Access | Request Contract | Success Response | Error Codes |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `GET` | `/api/health` | Public | None | `200 {"ok":true,"db":"up","redis":"up"}` | `503` if dependency down |
| `POST` | `/api/login` | Public | `{"email","password"}` | `200 {"user":{...},"token":"..."}` + `Set-Cookie: lms_session=...; HttpOnly; SameSite=Strict; Path=/; Max-Age=86400` | `400`, `401`, `429` |
| `POST` | `/api/register` | Public | `{"name","email","password"}` (`len>=10`) | `200 {"user":{...},"token":"..."}` + `Set-Cookie` | `400`, `409`, `429` |
| `POST` | `/api/logout` | Authenticated | None | `200 {"ok":true}` (revokes user sessions + clears cookie) | `401` |
| `POST` | `/api/password` | Authenticated | `{"current","next"}` (`len>=10`) | `200 {"user":{...}}` + rotated session cookie | `400`, `401` |

### 3.2 Student Workspace, Learning, Assets & Finance

| Method | Path | Access | Request Contract | Success Response | Error Codes |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `GET` | `/api/bootstrap` | Authenticated | None | `200` full hydrated workspace JSON matching `BootstrapResponse` (`semesterSubjects`, `lecturesBySubject` with hidden tests stripped for students, `contestSummary`, `recentContests`, `demoWeek`, `drawerNotices`, `user`, `fees`, `submissions`, `progress`) | `401` |
| `PUT` | `/api/profile` | Authenticated | `{"profile":{name, avatarUrl, bannerUrl, platforms[], ...}}` | `200 {"ok":true,"version":N}` (validates image MIME prefix + HTTP/HTTPS links) | `400`, `401`, `409` |
| `POST` | `/api/progress` | Authenticated | `{"lectureId":number,"complete":boolean}` | `200 {"ok":true}` | `401`, `404` |
| `POST` | `/api/submissions` | Student | `{"assignmentId","answer","file"?:base64,"filename"?:string}` (`<=10 MB`) | `200 {"ok":true,"id":"..."}` (rejects `kind === 'coding'` or already graded) | `400`, `401`, `403`, `404`, `409`, `413` |
| `GET` | `/api/submission-file/{id}` | Authenticated | None | Binary download `Content-Disposition: attachment` | `401`, `403`, `404` |
| `GET` | `/api/discussions?lecture={id}` | Authenticated | Query `lecture` | `200 [{id,user_id,lecture_id,body,created,name}]` | `401` |
| `POST` | `/api/discussions` | Authenticated | `{"lectureId","body"}` | `201 {"ok":true,"id":"..."}` | `400`, `401`, `404` |
| `GET` | `/api/notes/{lectureId}` | Authenticated | None | `200` `text/plain` downloadable lecture notes | `401`, `404` |
| `GET` | `/api/assets/{assetId}` | Authenticated | None | `200` asset stream from `AssetStore` (`Range` support for MP4/WebM) | `401`, `404` |
| `POST` | `/api/payments` | Student | `{"feeId","idempotencyKey"?}` | `200 {"ok":true}` inside ACID `TxManager` | `401`, `403`, `404`, `409` |
| `GET` | `/api/receipts/{feeId}` | Authenticated | None | `200` `text/plain` receipt (`status === 'Paid'` required) | `401`, `404`, `409` |
| `POST` | `/api/coding/evaluate` | Student | `{"assignmentId","language","source","action":"run"|"submit","customInput"?}` | `200` verdict + per-case results (hidden details stripped); if `submit` + `Accepted`, auto-grades `submissions` | `400`, `401`, `403`, `404`, `429` |
| `GET` | `/api/coding/history?assignmentId={id}` | Authenticated | Query `assignmentId` | `200` last 20 attempts `{id,language,source,verdict,passed,total,result,created}` | `401`, `404` |

### 3.3 Staff & Administrator Endpoints

| Method | Path | Access | Summary |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/staff` | Staff (`teacher \| admin`) | Returns `{users, submissions, attendance, fees}` (no password hashes) |
| `PUT` | `/api/staff/catalog` | Staff | Edits course, lecture, or notice metadata + triggers outbox cache invalidation |
| `POST` | `/api/staff/courses` | Staff | Creates a course in semesters `1..3` (`409` on duplicate code) |
| `POST` | `/api/staff/lectures` | Staff | Prepends a lecture to a course |
| `POST` | `/api/staff/assignments` | Staff | Adds a written assignment (`dueDate` validated) |
| `POST` | `/api/staff/coding` | Staff | Adds a coding assignment (`2..10` test cases, >= 1 example & >= 1 hidden) |
| `POST` | `/api/staff/notices` | Staff | Publishes an `ACADEMIC` or `URGENT` notice + emits SSE event |
| `POST` | `/api/staff/assets` | Staff | Streams attachment/MP4/WebM (`<=10 MB`) to `AssetStore`, writes metadata row in DB |
| `POST` | `/api/staff/users` | **Admin only** | Creates a student or teacher user (`password >= 10` chars) |
| `POST` | `/api/staff/fees` | **Admin only** | Issues a fee invoice for a student (`Pending` or `Overdue`) |
| `PUT` | `/api/staff/attendance` | Staff | Upserts `attended` / `total` (`0 <= attended <= total`) |
| `PUT` | `/api/staff/grade` | Staff | Grades a submission (`earned/max` format validated) + triggers SSE update |
| `POST` | `/api/staff/schedule` | Staff | Inserts schedule slot; PostgreSQL `EXCLUDE USING gist` enforces `409` overlap protection |

### 3.4 Offline Sync & Real-Time Events (`/api/v1/*`)

| Method | Path | Access | Contract |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/v1/sync/pull?cursor={seq}&limit=200` | Authenticated | Returns `{"cursor":nextSeq,"hasMore":bool,"entries":[ChangelogEntry]}` scoped to global catalog + `target_user_id = currentUser.ID` |
| `POST` | `/api/v1/sync/push` | Authenticated | Accepts batch `{"mutations":[{"idempotencyKey","entityType","entityId","baseVersion","payload"}]}`; processes each idempotently and returns per-item `applied \| conflict \| rejected` status + authoritative snapshot |
| `GET` | `/api/v1/events` | Authenticated | `text/event-stream` (`Cache-Control: no-cache`, 25s heartbeat) subscribing to Redis `pubsub:user:{userId}` & `pubsub:global` (`sync.available`, `grade.updated`, `notice.published`) |

### 3.5 Vedam Homepage Academic & Biometric Endpoints (`/api/v1/*` Envelope)

All endpoints below wrap responses inside the canonical `ApiResponse<T>` envelope (`{data, errorCode, errorPath, fieldErrors, message, statusCode, timestamp}`) with `Page<T>` (`items`, `page`, `size`, `totalElements`, `totalPages`, `numberOfElements`, `first`, `last`, `meta`) for list endpoints:

| Method | Path | Access | Response Payload (`data`) |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/v1/program-enrolments` | Authenticated | `Page<ProgramEnrolmentItem>` (resolved via authenticated student's `cprId`) |
| `GET` | `/api/v1/semesters?programId={id}` | Authenticated | `SemesterRecord[]` ordered by `startDate` (includes Semester `1`, `2`, `6` Bootcamp, `3`) |
| `GET` | `/api/v1/course-offering-enrolments?semesterId={id}` | Authenticated | `Page<CourseOfferingEnrolmentItem>` with granular `presentCount`, `absentCount`, `lateCount`, `leaveCount`, `excusedCount`, `awaitingCount`, `totalCount`, and `courseRequiredAttendance` |
| `GET` | `/api/v1/class-sessions?semesterId={id}&courseOfferingId={id}&page=0&size=1000` | Authenticated | `Page<ClassSessionItem>` with Unix epoch `startTime` / `endTime`, `topic`, `label`, `room` |
| `GET` | `/api/v1/biometrics/face` | Authenticated | `FaceBiometricRecord` (`hasFaceData`, `faceJson` 128-d embedding, `email`, `personId`) |

---

## 4. Strangler Parity Verification & Cutover Procedure

1. Run Go API on `:3001` (`go run ./cmd/api`) and keep Vite dev proxy in [`vite.config.ts`](file:///Users/sirwagyashekhar/Projects/LMS/vite.config.ts) pointing at `http://127.0.0.1:3001` without changing a single frontend route.
2. Point Node test scenarios from [`server/lms.test.mjs`](file:///Users/sirwagyashekhar/Projects/LMS/server/lms.test.mjs), [`server/payments.test.mjs`](file:///Users/sirwagyashekhar/Projects/LMS/server/payments.test.mjs), and [`server/learning-routes.test.mjs`](file:///Users/sirwagyashekhar/Projects/LMS/server/learning-routes.test.mjs) against a live Go server instance to prove identical HTTP status codes, JSON shapes, cookie attributes, and RBAC rejections.
3. Verify hidden test case stripping in `/api/bootstrap` for `student@vedam.local` vs. presence for `teacher@vedam.local`.
4. Once `coding.test.mjs` parity is ready in Phase 3 (or temporarily bridged via Phase 2 subprocess adapter), update [`package.json`](file:///Users/sirwagyashekhar/Projects/LMS/package.json) `server` / `dev` scripts to start the Go backend + Vite, and remove `server/` in a clean dedicated commit.

---

## 5. Phase 2 Deliverables & Verification Gate

- **Deliverables**:
  - `backend/internal/adapters/redis/{sessions,ratelimit,cache,streams,pubsub}.go`
  - `backend/internal/adapters/crypto/password.go`
  - `backend/internal/adapters/storage/local_assets.go`
  - `backend/internal/adapters/http/{middleware,router,handlers_*.go,sse.go}`
  - `backend/cmd/api/main.go` and `backend/cmd/worker/main.go`
  - End-to-end HTTP `httptest` + integration suite covering RBAC, rate-limiting, concurrency, idempotency, and `/api/v1/sync/{pull,push}`
- **Verification Commands**:
  ```bash
  cd backend
  go test -race ./...
  curl -sf http://127.0.0.1:3001/api/health
  cd .. && npm run build
  ```
- **Exit Criteria**: All Go unit + HTTP integration tests pass with `-race`, the React app logs in as student/teacher/admin and exercises every screen against Go + Postgres + Redis, and Redis rate limiting + cache invalidation are verified under concurrent test load.
