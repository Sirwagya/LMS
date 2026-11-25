//go:build integration

package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"vedam/lms/internal/adapters/crypto"
	redisadapter "vedam/lms/internal/adapters/redis"
	"vedam/lms/internal/seed"
)

type liveAPI struct {
	*Runtime
	URL string
}

func startAPI(t *testing.T) liveAPI {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.MkdirTemp("/tmp", "lms-r-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(data) })
	socket := filepath.Join(data, "redis.sock")
	binary := os.Getenv("TEST_REDIS_SERVER")
	if binary == "" {
		binary = "redis-server"
	}
	command := exec.Command(binary, "--port", "0", "--unixsocket", socket, "--save", "", "--appendonly", "no", "--dir", data)
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Signal(os.Interrupt); _ = command.Wait() })
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket})
	defer client.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for client.Ping(ctx).Err() != nil {
		if ctx.Err() != nil {
			t.Fatal("Redis did not start", ctx.Err())
		}
		time.Sleep(15 * time.Millisecond)
	}
	source := os.Getenv("TEST_DATABASE_URL")
	if source == "" {
		source = "postgres://lms:lms_local_dev@127.0.0.1:5432/lms_test?sslmode=disable"
	}
	control, err := pgxpool.New(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(control.Close)
	schema := "lms_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := control.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := control.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	db, err := url.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	query := db.Query()
	query.Set("search_path", schema+",public")
	db.RawQuery = query.Encode()
	cfg := Config{DatabaseURL: db.String(), RedisURL: "unix://" + socket, Assets: filepath.Join(data, "assets"), Web: filepath.Join(root, "dist"),
		Grader: filepath.Join(root, "grader", "bridge.mjs"), SessionTTL: 24 * time.Hour}
	rt, err := Open(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	if err := seed.Run(ctx, rt.DB, cryptoadapter.NewDualPasswordHasher()); err != nil {
		t.Fatal(err)
	}
	handler, closeFiles, err := rt.Handler()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeFiles)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return liveAPI{rt, server.URL}
}

type reply struct {
	status int
	header http.Header
	raw    []byte
}

func (api liveAPI) call(ctx context.Context, method, path, token, key string, data any) (reply, error) {
	var body io.Reader
	if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			return reply{}, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, api.URL+path, body)
	if err != nil {
		return reply{}, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return reply{}, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	return reply{res.StatusCode, res.Header, raw}, err
}
func requireStatus(t *testing.T, api liveAPI, status int, method, path, token, key string, data any) reply {
	t.Helper()
	res, err := api.call(t.Context(), method, path, token, key, data)
	if err != nil {
		t.Fatal(err)
	}
	if res.status != status {
		t.Fatalf("%s %s: got %d want %d: %.500s", method, path, res.status, status, res.raw)
	}
	return res
}
func decode[T any](t *testing.T, res reply) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(res.raw, &v); err != nil {
		t.Fatalf("decode %s: %v", res.raw, err)
	}
	return v
}

type identity struct {
	Token string
	User  struct{ ID string }
}

func loginAs(t *testing.T, api liveAPI, role string) identity {
	t.Helper()
	res := requireStatus(t, api, 200, "POST", "/api/login", "", "", map[string]string{"email": role + "@vedam.local", "password": "VedamLocal123!"})
	for _, attribute := range []string{"HttpOnly", "SameSite=Strict", "Max-Age=86400", "Path=/"} {
		if !strings.Contains(res.header.Get("Set-Cookie"), attribute) {
			t.Fatalf("missing cookie attribute %s", attribute)
		}
	}
	if _, err := uuid.Parse(res.header.Get("X-Request-ID")); err != nil {
		t.Fatal(err)
	}
	return decode[identity](t, res)
}

func TestHTTPRedisPostgresParity(t *testing.T) {
	api := startAPI(t)
	requireStatus(t, api, 200, "GET", "/api/v1/health", "", "", nil)
	requireStatus(t, api, 401, "GET", "/api/v1/events", "", "", nil)
	stu := loginAs(t, api, "student")
	teacher := loginAs(t, api, "teacher")
	admin := loginAs(t, api, "admin")
	requireStatus(t, api, 403, "GET", "/api/v1/staff", stu.Token, "", nil)
	requireStatus(t, api, 403, "POST", "/api/v1/staff/users", teacher.Token, "", nil)
	requireStatus(t, api, 200, "GET", "/api/v1/staff", admin.Token, "", nil)

	studentBoot := requireStatus(t, api, 200, "GET", "/api/v1/bootstrap", stu.Token, "", nil)
	staffBoot := requireStatus(t, api, 200, "GET", "/api/v1/bootstrap", teacher.Token, "", nil)
	if bytes.Contains(studentBoot.raw, []byte(`"testCases":`)) || !bytes.Contains(staffBoot.raw, []byte(`"testCases":`)) {
		t.Fatal("role-based test case visibility violated")
	}
	for _, path := range []string{"program-enrolments", "semesters?programId=1", "course-offering-enrolments?semesterId=3", "class-sessions?semesterId=3&size=2", "biometrics/face"} {
		res := requireStatus(t, api, 200, "GET", "/api/v1/"+path, stu.Token, "", nil)
		payload := decode[struct {
			Data       json.RawMessage
			StatusCode int
		}](t, res)
		if payload.StatusCode != 200 || len(payload.Data) == 0 || string(payload.Data) == "null" {
			t.Fatalf("invalid academic envelope for %s", path)
		}
	}
	requireStatus(t, api, 404, "GET", "/api/v1/biometrics/face", teacher.Token, "", nil)

	// Populate both caches before changing a global record.
	notice := "Phase 2 catalogue update"
	requireStatus(t, api, 201, "POST", "/api/staff/notices", teacher.Token, "", map[string]string{"title": notice, "desc": "Published", "tag": "URGENT"})
	var readers sync.WaitGroup
	for range 12 {
		readers.Go(func() {
			res, err := api.call(t.Context(), "GET", "/api/bootstrap", stu.Token, "", nil)
			if err != nil || res.status != 200 || !bytes.Contains(res.raw, []byte(notice)) {
				t.Errorf("cache read after mutation: status %d, error %v", res.status, err)
			}
		})
	}
	readers.Wait()

	var boot struct {
		Fees              []struct{ ID, Status string }
		LecturesBySubject map[string][]struct{ ID int64 }
	}
	boot = decode[struct {
		Fees              []struct{ ID, Status string }
		LecturesBySubject map[string][]struct{ ID int64 }
	}](t, studentBoot)
	var paymentID string
	for _, fee := range boot.Fees {
		if fee.Status != "Paid" {
			paymentID = fee.ID
			break
		}
	}
	if paymentID == "" {
		t.Fatal("seed must include a payable invoice")
	}
	other := "different-invoice"
	for range 8 {
		readers.Go(func() {
			res, err := api.call(t.Context(), "POST", "/api/payments", stu.Token, "payment-1", map[string]string{"feeId": paymentID})
			if err != nil || res.status != 200 {
				t.Errorf("concurrent payment: %d %s %v", res.status, res.raw, err)
			}
		})
	}
	readers.Wait()
	requireStatus(t, api, 409, "POST", "/api/v1/payments", stu.Token, "payment-1", map[string]string{"feeId": other})
	requireStatus(t, api, 409, "POST", "/api/payments", stu.Token, "", map[string]string{"feeId": paymentID})
	var count int
	if err := api.DB.QueryRow(t.Context(), "SELECT COUNT(*) FROM payments WHERE fee_id=$1", paymentID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("payments count %d: %v", count, err)
	}

	type mutationResult struct {
		Results []struct {
			Status   string
			Snapshot json.RawMessage `json:"serverPayload"`
		}
	}
	var progressID int64
	for _, lectures := range boot.LecturesBySubject {
		if len(lectures) > 0 {
			progressID = lectures[0].ID
			break
		}
	}
	raw := map[string]any{"mutations": []any{
		map[string]any{"idempotencyKey": "p-1", "entityType": "profile", "entityId": stu.User.ID, "baseVersion": 1, "payload": map[string]string{"name": "Offline Student"}},
		map[string]any{"idempotencyKey": "p-2", "entityType": "profile", "entityId": stu.User.ID, "baseVersion": 1, "payload": map[string]string{"name": "Stale name"}},
		map[string]any{"idempotencyKey": "p-3", "entityType": "lecture_progress", "entityId": fmt.Sprint(progressID), "baseVersion": 0, "payload": map[string]bool{"complete": true}},
		map[string]any{"idempotencyKey": "p-4", "entityType": "fee_invoice", "entityId": paymentID, "baseVersion": 0, "payload": map[string]bool{"paid": true}},
	}}
	res := requireStatus(t, api, 200, "POST", "/api/v1/sync/push", stu.Token, "batch-1", raw)
	outcomes := decode[mutationResult](t, res).Results
	for i, expected := range []string{"applied", "conflict", "applied", "rejected"} {
		if outcomes[i].Status != expected {
			t.Fatalf("mutation %d got %s want %s: %s", i, outcomes[i].Status, expected, res.raw)
		}
	}
	replay := requireStatus(t, api, 200, "POST", "/api/v1/sync/push", stu.Token, "batch-1", raw)
	var first, repeated any
	first = decode[any](t, res)
	repeated = decode[any](t, replay)
	if !bytes.Equal(mustJSON(t, first), mustJSON(t, repeated)) {
		t.Fatalf("replay changed result: %s", replay.raw)
	}
	type changes struct {
		Cursor  int64
		HasMore bool
		Entries []struct{ EntityType, Scope string }
	}
	cursor := decode[changes](t, requireStatus(t, api, 200, "GET", "/api/v1/sync/pull?limit=1", stu.Token, "", nil))
	if !cursor.HasMore || len(cursor.Entries) != 1 {
		t.Fatal("expected paginated changelog")
	}
	studentChanges := decode[changes](t, requireStatus(t, api, 200, "GET", fmt.Sprintf("/api/v1/sync/pull?cursor=%d", cursor.Cursor), stu.Token, "", nil))
	found := map[string]bool{}
	for _, entry := range studentChanges.Entries {
		found[entry.EntityType] = true
	}
	if !found["profile"] || !found["lecture_progress"] {
		t.Fatalf("missing private mutations: %v", found)
	}
	staffChanges := decode[changes](t, requireStatus(t, api, 200, "GET", "/api/v1/sync/pull", teacher.Token, "", nil))
	for _, entry := range staffChanges.Entries {
		if entry.Scope != "global" {
			t.Fatalf("private entry leaked to staff: %#v", entry)
		}
	}

	stale := map[string]any{"mutations": []any{map[string]any{
		"idempotencyKey": "p-5", "entityType": "lecture_progress", "entityId": fmt.Sprint(progressID),
		"baseVersion": 0, "payload": map[string]any{"complete": false, "clientTimestamp": 1000},
	}}}
	staleResult := decode[mutationResult](t, requireStatus(t, api, 200, "POST", "/api/v1/sync/push", stu.Token, "", stale))
	if !bytes.Contains(staleResult.Results[0].Snapshot, []byte(`"complete":true`)) {
		t.Fatal("stale offline progress overwrote newer state")
	}

	sessions := redisadapter.NewSessions(api.Redis, 2*time.Hour)
	if _, err := sessions.Refresh(t.Context(), stu.Token); err != nil {
		t.Fatal(err)
	}
	check := requireStatus(t, api, 200, "GET", "/api/v1/bootstrap", stu.Token, "", nil)
	if !bytes.Contains(check.raw, []byte("Offline Student")) {
		t.Fatal("stale personalized bootstrap")
	}
	second := loginAs(t, api, "student")
	requireStatus(t, api, 200, "POST", "/api/logout", stu.Token, "", nil)
	requireStatus(t, api, 401, "GET", "/api/bootstrap", second.Token, "", nil)

	var limited int
	var accepted int
	var mu sync.Mutex
	for range 35 {
		readers.Go(func() {
			res, err := api.call(t.Context(), "POST", "/api/login", "", "", map[string]string{})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				t.Error(err)
			case res.status == 400:
				accepted++
			case res.status == 429 && res.header.Get("Retry-After") != "":
				limited++
			default:
				t.Errorf("unexpected throttling result: %d %s", res.status, res.raw)
			}
		})
	}
	readers.Wait()
	if accepted != 26 || limited != 9 {
		t.Fatalf("rate limit allowed %d and rejected %d", accepted, limited)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestWorkerSSEAndGradingQueue(t *testing.T) {
	api := startAPI(t)
	student := loginAs(t, api, "student")
	teacher := loginAs(t, api, "teacher")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", api.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+student.Token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" || res.Header.Get("Cache-Control") != "no-cache" {
		t.Fatal("unexpected SSE handshake")
	}
	reader := bufio.NewReader(res.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != ": connected\n" {
		t.Fatalf("SSE handshake %q: %v", line, err)
	}
	requireStatus(t, api, 201, "POST", "/api/staff/notices", teacher.Token, "", map[string]string{"title": "Live notice", "desc": "Event"})
	n, err := api.RelayOutbox(ctx)
	if err != nil || n != 1 {
		t.Fatalf("relay count %d: %v", n, err)
	}
	if n, err := api.RelayOutbox(ctx); err != nil || n != 0 {
		t.Fatalf("repeated relay count %d: %v", n, err)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "event: notice.published\n" {
			break
		}
	}
	runRequest := map[string]string{"assignmentId": "coding-two-sum", "language": "python", "source": "class Solution:\n    def twoSum(self, nums, target):\n        return [0, 1]", "action": "submit"}
	unlock, ok, err := redisadapter.LockJudge(ctx, api.Redis, student.User.ID)
	if err != nil || !ok {
		t.Fatalf("judge lock: %t %v", ok, err)
	}
	requireStatus(t, api, 429, "POST", "/api/coding/evaluate", student.Token, "", runRequest)
	unlock()
	req, err = http.NewRequestWithContext(ctx, "POST", api.URL+"/api/v1/coding/evaluate", bytes.NewReader(mustJSON(t, runRequest)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+student.Token)
	req.Header.Set("Prefer", "respond-async")
	queued, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	queued.Body.Close()
	if queued.StatusCode != 202 || queued.Header.Get("Location") == "" {
		t.Fatalf("async submit: %d", queued.StatusCode)
	}
	path := queued.Header.Get("Location")
	requireStatus(t, api, 401, "GET", path, "", "", nil)
	requireStatus(t, api, 403, "GET", path, teacher.Token, "", nil)
	registered := decode[identity](t, requireStatus(t, api, 200, "POST", "/api/register", "", "", map[string]string{"name": "Another Student", "email": "another@vedam.local", "password": "AnotherPassword123!"}))
	requireStatus(t, api, 404, "GET", path, registered.Token, "", nil)
	if err := api.Queue.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	message, err := api.Queue.Next(ctx, "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := api.processJob(ctx, message); err != nil {
		t.Fatal(err)
	}
	finished := decode[redisadapter.JobState](t, requireStatus(t, api, 200, "GET", path, student.Token, "", nil))
	if finished.Status != "complete" || !bytes.Contains(finished.Result, []byte(`"verdict":"Wrong Answer"`)) {
		t.Fatalf("unexpected grading result: %+v", finished)
	}
	if err := api.processJob(ctx, message); err != nil {
		t.Fatal(err)
	}
	history := decode[[]json.RawMessage](t, requireStatus(t, api, 200, "GET", "/api/coding/history?assignmentId=coding-two-sum", student.Token, "", nil))
	if len(history) != 1 {
		t.Fatalf("grading retry wrote %d attempts", len(history))
	}
}
