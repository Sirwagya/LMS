//go:build integration

package runtime

import (
	"bytes"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"vedam/lms/internal/adapters/postgres"
	"vedam/lms/internal/adapters/storage"
	"vedam/lms/internal/domain/contest"
	"vedam/lms/internal/domain/errs"
)

func TestContestProctoringWorkflow(t *testing.T) {
	api := startConfiguredAPI(t, func(cfg *Config) {
		cfg.ContestAuthoring = true
		cfg.ContestAttempts = true
		cfg.ContestIntegrity = true
	})

	teacher := loginAs(t, api, "teacher")
	student := loginAs(t, api, "student")
	admin := loginAs(t, api, "admin")
	studentB := contestTestUser(t, api, admin.Token, "student", "proctor-b")
	unrelatedTeacher := contestTestUser(t, api, admin.Token, "teacher", "proctor-unrelated")

	now := time.Now().UTC()
	chosen := 0
	exIn, exOut := "2", "4"
	hidIn, hidOut := "3", "9"
	draft := contest.Draft{
		Title:           "Proctored Media & Lockdown Contest",
		Instructions:    "Keep camera, mic, and screen share active.",
		OpensAt:         now.Add(time.Hour),
		ClosesAt:        now.Add(3 * time.Hour),
		DurationMinutes: 45,
		StudentIDs:      []string{student.User.ID, studentB.User.ID},
		Questions: []contest.Question{
			{Kind: "mcq", Prompt: "2+2?", Points: 5, Choices: []string{"4", "5"}, CorrectChoice: &chosen},
			{
				Kind:      "coding",
				Prompt:    "Square integer",
				Points:    15,
				TestCases: []contest.TestCase{{Input: &exIn, Expected: &exOut}, {Input: &hidIn, Expected: &hidOut, Hidden: true}},
			},
		},
	}

	created := decode[contest.Record](t, requireStatus(t, api, http.StatusCreated, http.MethodPost, "/api/staff/contests", teacher.Token, "", map[string]any{"draft": draft}))
	requireStatus(t, api, http.StatusOK, http.MethodPost, "/api/staff/contests/"+created.ID+"/publish", teacher.Token, "", map[string]int{"revision": 1})
	if _, err := api.DB.Exec(t.Context(), `UPDATE assessments SET opens_at=$2, closes_at=$3 WHERE id=$1`, created.ID, now.Add(-time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	nativeHeaders := map[string]string{"X-LMS-Contest-Client": "macos"}
	startAttempt := func(tok, dev string) contest.AttemptView {
		t.Helper()
		res, err := api.callWithHeaders(t.Context(), http.MethodPost, "/api/v1/contests/"+created.ID+"/attempt", tok, "", map[string]any{
			"action":   "start",
			"deviceId": dev,
		}, nativeHeaders)
		if err != nil || res.status != http.StatusOK {
			t.Fatalf("start attempt failed: err=%v status=%d raw=%s", err, res.status, res.raw)
		}
		return decode[contest.AttemptView](t, res)
	}

	devA, devB := uuid.NewString(), uuid.NewString()
	viewA := startAttempt(student.Token, devA)
	viewB := startAttempt(studentB.Token, devB)

	webmPayload := []byte("\x1a\x45\xdf\xa3mock-webm-camera-audio-chunk-0001")
	jpegPayload := []byte("\xff\xd8\xff\xe0mock-jpeg-screen-snapshot-0001\xff\xd9")

	// 1. Browser / spoofed requests are rejected with 403 on POST /api/v1/contests/{id}/media.
	noHeaderRes, err := api.callWithHeaders(t.Context(), http.MethodPost, "/api/v1/contests/"+created.ID+"/media", student.Token, "", map[string]any{
		"attemptId":   viewA.AttemptID,
		"deviceId":    devA,
		"kind":        "camera_audio",
		"seq":         1,
		"contentType": "video/webm;codecs=vp8,opus",
		"durationMs":  15000,
		"capturedAt":  now.Format(time.RFC3339),
		"dataBase64":  base64.StdEncoding.EncodeToString(webmPayload),
	}, nil)
	if err != nil || noHeaderRes.status != http.StatusForbidden {
		t.Fatalf("expected 403 without macos header, got status=%d err=%v", noHeaderRes.status, err)
	}

	spoofRes, err := api.callWithHeaders(t.Context(), http.MethodPost, "/api/v1/contests/"+created.ID+"/media", student.Token, "", map[string]any{
		"attemptId":   viewA.AttemptID,
		"deviceId":    devA,
		"kind":        "camera_audio",
		"seq":         1,
		"contentType": "video/webm",
		"durationMs":  15000,
		"capturedAt":  now.Format(time.RFC3339),
		"dataBase64":  base64.StdEncoding.EncodeToString(webmPayload),
	}, map[string]string{
		"X-LMS-Contest-Client": "macos",
		"Sec-Fetch-Site":       "same-origin",
	})
	if err != nil || spoofRes.status != http.StatusForbidden {
		t.Fatalf("expected 403 for browser Sec-Fetch-Site header, got status=%d err=%v", spoofRes.status, err)
	}

	// 2. Native macOS media uploads (camera_audio webm & screen_snapshot jpeg) + duplicate seq rejection.
	camRes, err := api.callWithHeaders(t.Context(), http.MethodPost, "/api/v1/contests/"+created.ID+"/media", student.Token, "", map[string]any{
		"attemptId":   viewA.AttemptID,
		"deviceId":    devA,
		"kind":        "camera_audio",
		"seq":         1,
		"contentType": "video/webm;codecs=vp8,opus",
		"durationMs":  15000,
		"capturedAt":  now.Format(time.RFC3339),
		"dataBase64":  base64.StdEncoding.EncodeToString(webmPayload),
	}, nativeHeaders)
	if err != nil || camRes.status != http.StatusCreated {
		t.Fatalf("camera_audio upload failed: status=%d err=%v raw=%s", camRes.status, err, camRes.raw)
	}
	camItem := decode[contest.ProctorMediaItem](t, camRes)
	if camItem.Kind != "camera_audio" || camItem.Seq != 1 || camItem.ContentType != "video/webm" || camItem.ByteSize != len(webmPayload) {
		t.Fatalf("unexpected camera_audio item: %#v", camItem)
	}

	snapRes, err := api.callWithHeaders(t.Context(), http.MethodPost, "/api/v1/contests/"+created.ID+"/media", student.Token, "", map[string]any{
		"attemptId":   viewA.AttemptID,
		"deviceId":    devA,
		"kind":        "screen_snapshot",
		"seq":         1,
		"contentType": "image/jpeg",
		"capturedAt":  now.Add(5 * time.Second).Format(time.RFC3339),
		"dataBase64":  base64.StdEncoding.EncodeToString(jpegPayload),
	}, nativeHeaders)
	if err != nil || snapRes.status != http.StatusCreated {
		t.Fatalf("screen_snapshot upload failed: status=%d err=%v raw=%s", snapRes.status, err, snapRes.raw)
	}
	snapItem := decode[contest.ProctorMediaItem](t, snapRes)
	if snapItem.Kind != "screen_snapshot" || snapItem.Seq != 1 || snapItem.ContentType != "image/jpeg" || snapItem.ByteSize != len(jpegPayload) {
		t.Fatalf("unexpected screen_snapshot item: %#v", snapItem)
	}

	// Duplicate (attemptId, kind, seq) returns 409 Conflict.
	dupRes, err := api.callWithHeaders(t.Context(), http.MethodPost, "/api/v1/contests/"+created.ID+"/media", student.Token, "", map[string]any{
		"attemptId":   viewA.AttemptID,
		"deviceId":    devA,
		"kind":        "camera_audio",
		"seq":         1,
		"contentType": "video/webm",
		"durationMs":  15000,
		"capturedAt":  now.Format(time.RFC3339),
		"dataBase64":  base64.StdEncoding.EncodeToString(webmPayload),
	}, nativeHeaders)
	if err != nil || dupRes.status != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate seq upload, got status=%d err=%v raw=%s", dupRes.status, err, dupRes.raw)
	}

	// Upload one media chunk for studentB so we can verify retention cleanup vs open appeal protection later.
	snapBRes, err := api.callWithHeaders(t.Context(), http.MethodPost, "/api/v1/contests/"+created.ID+"/media", studentB.Token, "", map[string]any{
		"attemptId":   viewB.AttemptID,
		"deviceId":    devB,
		"kind":        "screen_snapshot",
		"seq":         1,
		"contentType": "image/jpeg",
		"capturedAt":  now.Add(6 * time.Second).Format(time.RFC3339),
		"dataBase64":  base64.StdEncoding.EncodeToString(jpegPayload),
	}, nativeHeaders)
	if err != nil || snapBRes.status != http.StatusCreated {
		t.Fatalf("studentB screen_snapshot upload failed: status=%d err=%v raw=%s", snapBRes.status, err, snapBRes.raw)
	}
	snapBItem := decode[contest.ProctorMediaItem](t, snapBRes)

	// 3. Proctoring & lockdown telemetry events trigger high-severity review queue flags.
	evRes, err := api.callWithHeaders(t.Context(), http.MethodPost, "/api/v1/contests/"+created.ID+"/events", student.Token, "", map[string]any{
		"attemptId": viewA.AttemptID,
		"deviceId":  devA,
		"batchId":   uuid.NewString(),
		"events": []map[string]any{
			{
				"seq":       1,
				"eventType": "app.opened",
				"clientAt":  now.Format(time.RFC3339),
				"payload": map[string]any{
					"apps": []map[string]any{{"name": "Discord", "bundleId": "com.hnc.Discord", "pid": 9001}},
				},
			},
			{"seq": 2, "eventType": "proctor.camera_lost", "clientAt": now.Add(time.Second).Format(time.RFC3339), "payload": map[string]any{}},
			{"seq": 3, "eventType": "proctor.mic_lost", "clientAt": now.Add(2 * time.Second).Format(time.RFC3339), "payload": map[string]any{}},
			{"seq": 4, "eventType": "proctor.screen_lost", "clientAt": now.Add(3 * time.Second).Format(time.RFC3339), "payload": map[string]any{}},
		},
	}, nativeHeaders)
	if err != nil || evRes.status != http.StatusOK {
		t.Fatalf("proctor events ingest failed: status=%d err=%v raw=%s", evRes.status, err, evRes.raw)
	}

	attemptPath := "/api/v1/contests/" + created.ID + "/attempt"
	saved, err := api.callWithHeaders(t.Context(), http.MethodPost, attemptPath, student.Token, "", map[string]any{
		"action": "save", "deviceId": devA, "lease": viewA.Lease, "revision": viewA.Revision,
		"answers": map[int]contest.Answer{0: {Choice: &chosen}},
	}, nativeHeaders)
	if err != nil || saved.status != http.StatusOK {
		t.Fatalf("save before warnings: %d %s (%v)", saved.status, saved.raw, err)
	}
	warnings := func(batchID, device, reason string, seq int64, status int) contest.EventBatchResult {
		t.Helper()
		res, err := api.callWithHeaders(t.Context(), http.MethodPost, "/api/v1/contests/"+created.ID+"/events", student.Token, "",
			contest.EventBatchInput{AttemptID: viewA.AttemptID, DeviceID: device, BatchID: batchID, Events: []contest.EventInput{{
				Seq: seq, EventType: "lockdown.warning", Payload: map[string]any{"reason": reason},
			}}}, nativeHeaders)
		if err != nil || res.status != status {
			t.Fatalf("warning: status=%d body=%s (%v)", res.status, res.raw, err)
		}
		if status != http.StatusOK {
			return contest.EventBatchResult{}
		}
		return decode[contest.EventBatchResult](t, res)
	}
	batch := uuid.NewString()
	warnings(uuid.NewString(), devA, "invalid", 5, http.StatusBadRequest)
	warnings(uuid.NewString(), devB, "paste_blocked", 5, http.StatusConflict)
	if first := warnings(batch, devA, "left_contest", 5, http.StatusOK); first.LockdownWarnings != 1 || first.LockdownSubmitted {
		t.Fatalf("first warning: %#v", first)
	}
	if duplicate := warnings(batch, devA, "left_contest", 5, http.StatusOK); duplicate.LockdownWarnings != 1 || !duplicate.Duplicate {
		t.Fatalf("duplicate warning: %#v", duplicate)
	}
	if overlap := warnings(uuid.NewString(), devA, "left_contest", 5, http.StatusOK); overlap.LockdownWarnings != 1 || overlap.Accepted != 0 {
		t.Fatalf("overlapping warning: %#v", overlap)
	}
	if second := warnings(uuid.NewString(), devA, "paste_blocked", 6, http.StatusOK); second.LockdownWarnings != 2 || second.LockdownSubmitted {
		t.Fatalf("second warning: %#v", second)
	}
	thirdBatch := uuid.NewString()
	for range 2 {
		third := warnings(thirdBatch, devA, "left_contest", 7, http.StatusOK)
		if !third.LockdownSubmitted || third.LockdownWarnings != 3 {
			t.Fatalf("third warning: %#v", third)
		}
	}
	if later := warnings(uuid.NewString(), devA, "paste_blocked", 8, http.StatusOK); later.LockdownWarnings != 3 || !later.LockdownSubmitted {
		t.Fatalf("late warning: %#v", later)
	}
	after, err := api.callWithHeaders(t.Context(), http.MethodPost, attemptPath, student.Token, "", map[string]any{
		"action": "save", "deviceId": devA, "lease": viewA.Lease, "revision": 1, "answers": map[int]contest.Answer{},
	}, nativeHeaders)
	if err != nil || after.status != http.StatusOK {
		t.Fatalf("closed attempt: %d %s (%v)", after.status, after.raw, err)
	}
	receipt := decode[contest.AttemptView](t, after)
	if receipt.State != "submitted" || receipt.FinishReason != "lockdown" || receipt.LockdownWarnings != 3 ||
		receipt.Answers[0].Choice == nil || *receipt.Answers[0].Choice != chosen || receipt.Revision != 1 {
		t.Fatalf("the third warning did not freeze saved answers: %#v", receipt)
	}

	queue := decode[contest.ReviewQueueView](t, requireStatus(t, api, http.StatusOK, http.MethodGet, "/api/staff/contests/"+created.ID+"/integrity", teacher.Token, "", nil))
	var reviewA contest.ReviewSummary
	for _, rv := range queue.Reviews {
		if rv.AttemptID == viewA.AttemptID {
			reviewA = rv
		}
	}
	if reviewA.Status != "flagged" {
		t.Fatalf("expected attempt A to be flagged after lockdown/proctor events, got %#v", reviewA)
	}
	seenSignals := make(map[string]string)
	for _, sig := range reviewA.Signals {
		seenSignals[sig.Kind] = sig.Severity
	}
	for _, kind := range []string{"app_opened", "proctor_camera_lost", "proctor_mic_lost", "proctor_screen_lost", "lockdown_auto_submit"} {
		if seenSignals[kind] != "high" {
			t.Fatalf("expected high-severity signal %q, got signals=%+v", kind, reviewA.Signals)
		}
	}

	// 4. Staff listing, byte-exact playback, range requests, unauthorized denial, and access log entries.
	requireStatus(t, api, http.StatusForbidden, http.MethodGet, "/api/staff/contests/"+created.ID+"/attempts/"+viewA.AttemptID+"/media", student.Token, "", nil)
	requireStatus(t, api, http.StatusForbidden, http.MethodGet, "/api/staff/contests/"+created.ID+"/media/"+camItem.ID, student.Token, "", nil)
	requireStatus(t, api, http.StatusNotFound, http.MethodGet, "/api/staff/contests/"+created.ID+"/attempts/"+viewA.AttemptID+"/media", unrelatedTeacher.Token, "", nil)
	requireStatus(t, api, http.StatusNotFound, http.MethodGet, "/api/staff/contests/"+created.ID+"/media/"+camItem.ID, unrelatedTeacher.Token, "", nil)

	listRes := requireStatus(t, api, http.StatusOK, http.MethodGet, "/api/staff/contests/"+created.ID+"/attempts/"+viewA.AttemptID+"/media", teacher.Token, "", nil)
	listBody := decode[struct {
		Items []contest.ProctorMediaItem `json:"items"`
	}](t, listRes)
	if len(listBody.Items) != 2 || listBody.Items[0].ID != camItem.ID || listBody.Items[1].ID != snapItem.ID {
		t.Fatalf("unexpected listed media items: %#v", listBody.Items)
	}

	camPlayRes := requireStatus(t, api, http.StatusOK, http.MethodGet, "/api/staff/contests/"+created.ID+"/media/"+camItem.ID, teacher.Token, "", nil)
	if !bytes.Equal(camPlayRes.raw, webmPayload) {
		t.Fatalf("camera_audio bytes mismatch: got %x want %x", camPlayRes.raw, webmPayload)
	}
	if ct := camPlayRes.header.Get("Content-Type"); ct != "video/webm" {
		t.Fatalf("Content-Type = %q, want video/webm", ct)
	}
	if cd := camPlayRes.header.Get("Content-Disposition"); !strings.HasPrefix(cd, "inline;") {
		t.Fatalf("Content-Disposition = %q, want inline", cd)
	}
	if nosniff := camPlayRes.header.Get("X-Content-Type-Options"); nosniff != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", nosniff)
	}
	if cc := camPlayRes.header.Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", cc)
	}

	snapPlayRes := requireStatus(t, api, http.StatusOK, http.MethodGet, "/api/staff/contests/"+created.ID+"/media/"+snapItem.ID, teacher.Token, "", nil)
	if !bytes.Equal(snapPlayRes.raw, jpegPayload) {
		t.Fatalf("screen_snapshot bytes mismatch")
	}

	rangeRes, err := api.callWithHeaders(t.Context(), http.MethodGet, "/api/staff/contests/"+created.ID+"/media/"+camItem.ID, teacher.Token, "", nil, map[string]string{
		"Range": "bytes=0-3",
	})
	if err != nil || rangeRes.status != http.StatusPartialContent || !bytes.Equal(rangeRes.raw, webmPayload[:4]) {
		t.Fatalf("range request failed: status=%d err=%v bytes=%x", rangeRes.status, err, rangeRes.raw)
	}

	var listAuditCount, viewAuditCount int
	if err := api.DB.QueryRow(t.Context(), `
		SELECT
			count(*) FILTER (WHERE action = 'proctor_media.list' AND actor_id = $2 AND attempt_id = $3::uuid),
			count(*) FILTER (WHERE action = 'proctor_media.view' AND actor_id = $2 AND attempt_id = $3::uuid)
		FROM assessment_evidence_access_log
		WHERE assessment_id = $1::uuid
	`, created.ID, teacher.User.ID, viewA.AttemptID).Scan(&listAuditCount, &viewAuditCount); err != nil {
		t.Fatal(err)
	}
	if listAuditCount < 1 || viewAuditCount < 2 {
		t.Fatalf("expected proctor_media audit entries, got list=%d view=%d", listAuditCount, viewAuditCount)
	}

	// 5. Retention sweep removes expired media files & DB rows for studentB while preserving studentA's open appeal.
	requireStatus(t, api, http.StatusCreated, http.MethodPost, "/api/contests/"+created.ID+"/appeal", student.Token, "", map[string]string{
		"kind":    "appeal",
		"message": "Camera briefly disconnected when USB hub reset.",
	})
	if _, err := api.DB.Exec(t.Context(), `
		INSERT INTO assessment_reviews (attempt_id, retain_until)
		VALUES ($1::uuid, clock_timestamp() - interval '1 day')
		ON CONFLICT (attempt_id) DO UPDATE SET retain_until = EXCLUDED.retain_until
	`, viewB.AttemptID); err != nil {
		t.Fatal(err)
	}
	if _, err := api.DB.Exec(t.Context(), `
		UPDATE assessment_reviews SET retain_until = clock_timestamp() - interval '1 day'
		WHERE attempt_id = $1::uuid
	`, viewA.AttemptID); err != nil {
		t.Fatal(err)
	}

	var storageKeyB string
	if err := api.DB.QueryRow(t.Context(), `SELECT storage_key FROM assessment_proctor_media WHERE id = $1::uuid`, snapBItem.ID).Scan(&storageKeyB); err != nil {
		t.Fatal(err)
	}

	assets, err := storage.NewLocalAssets(api.Config.Assets)
	if err != nil {
		t.Fatal(err)
	}
	defer assets.Close()

	repo := postgres.NewContestIntegrityRepo(api.Tx)
	purged, err := repo.PurgeExpiredProctorMedia(t.Context(), assets.Remove)
	if err != nil {
		t.Fatal(err)
	}
	if purged != 1 {
		t.Fatalf("expected 1 expired media item purged (studentB only), got %d", purged)
	}
	if _, err := assets.Open(t.Context(), storageKeyB); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("expected purged storage key %q to be removed from disk, got %v", storageKeyB, err)
	}
	// StudentA's media must still exist and remain playable.
	requireStatus(t, api, http.StatusOK, http.MethodGet, "/api/staff/contests/"+created.ID+"/media/"+camItem.ID, teacher.Token, "", nil)
}
