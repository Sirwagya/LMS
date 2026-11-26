package contest

import (
	"errors"
	"strings"
	"testing"
	"time"

	"vedam/lms/internal/domain/errs"
)

func TestValidateEventBatch(t *testing.T) {
	const (
		attemptID = "11111111-1111-4111-8111-111111111111"
		deviceID  = "22222222-2222-4222-8222-222222222222"
		batchID   = "33333333-3333-4333-8333-333333333333"
	)
	qIdx := 0
	now := time.Now().UTC()

	validBatch := EventBatchInput{
		AttemptID: attemptID,
		DeviceID:  deviceID,
		BatchID:   batchID,
		Events: []EventInput{
			{Seq: 1, EventType: "nav.question", QuestionIndex: &qIdx, ClientAt: &now, Payload: map[string]any{"to": 0}},
			{Seq: 2, EventType: "editor.paste", QuestionIndex: &qIdx, ClientAt: &now, Payload: map[string]any{"charCount": 120}},
			{Seq: 3, EventType: "window.blur", Payload: map[string]any{"durationMs": 500}},
		},
	}
	if err := ValidateEventBatch(validBatch); err != nil {
		t.Fatalf("expected valid batch to pass, got %v", err)
	}

	t.Run("rejects invalid UUIDs", func(t *testing.T) {
		bad := validBatch
		bad.AttemptID = "not-a-uuid"
		if err := ValidateEventBatch(bad); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("rejects empty and oversized batches", func(t *testing.T) {
		empty := validBatch
		empty.Events = nil
		if err := ValidateEventBatch(empty); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("expected ErrValidation for empty batch, got %v", err)
		}

		oversized := validBatch
		oversized.Events = make([]EventInput, MaxEventBatchSize+1)
		for i := range oversized.Events {
			oversized.Events[i] = EventInput{Seq: int64(i + 1), EventType: "window.focus", Payload: map[string]any{}}
		}
		if err := ValidateEventBatch(oversized); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("expected ErrValidation for oversized batch, got %v", err)
		}
	})

	t.Run("rejects duplicate and non-positive sequence numbers", func(t *testing.T) {
		dup := validBatch
		dup.Events = []EventInput{
			{Seq: 5, EventType: "window.focus", Payload: map[string]any{}},
			{Seq: 5, EventType: "window.blur", Payload: map[string]any{}},
		}
		if err := ValidateEventBatch(dup); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("expected ErrValidation for duplicate seq, got %v", err)
		}

		zero := validBatch
		zero.Events = []EventInput{{Seq: 0, EventType: "window.focus", Payload: map[string]any{}}}
		if err := ValidateEventBatch(zero); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("expected ErrValidation for seq 0, got %v", err)
		}
	})

	t.Run("rejects server event types injected by client", func(t *testing.T) {
		for _, serverType := range []string{
			"attempt.started", "attempt.takeover", "attempt.saved",
			"attempt.submitted", "attempt.expired", "attempt.code_submitted",
			"unknown.event",
		} {
			bad := validBatch
			bad.Events = []EventInput{{Seq: 1, EventType: serverType, Payload: map[string]any{}}}
			if err := ValidateEventBatch(bad); !errors.Is(err, errs.ErrValidation) {
				t.Fatalf("expected ErrValidation for %q, got %v", serverType, err)
			}
		}
	})

	t.Run("rejects prohibited privacy keys recursively", func(t *testing.T) {
		prohibited := []string{
			"clipboard", "clipboardText", "clipboard_text", "screen",
			"webcam", "microphone", "globalKeystrokes", "global_keystrokes",
			"unrelatedApp", "unrelated_app", "keystrokes",
		}
		for _, key := range prohibited {
			top := validBatch
			top.Events = []EventInput{{Seq: 1, EventType: "code.delta", Payload: map[string]any{key: "blocked"}}}
			if err := ValidateEventBatch(top); !errors.Is(err, errs.ErrValidation) {
				t.Fatalf("expected ErrValidation for top-level key %q, got %v", key, err)
			}

			nested := validBatch
			nested.Events = []EventInput{{
				Seq:       1,
				EventType: "code.delta",
				Payload: map[string]any{
					"meta": []any{map[string]any{key: true}},
				},
			}}
			if err := ValidateEventBatch(nested); !errors.Is(err, errs.ErrValidation) {
				t.Fatalf("expected ErrValidation for nested key %q, got %v", key, err)
			}
		}
	})

	t.Run("enforces editor.paste rules", func(t *testing.T) {
		cases := []map[string]any{
			{},
			{"charCount": -1},
			{"charCount": "400"},
			{"charCount": 120, "text": "pasted snippet"},
			{"charCount": 120, "nested": []any{"secret"}},
		}
		for i, payload := range cases {
			bad := validBatch
			bad.Events = []EventInput{{Seq: 1, EventType: "editor.paste", Payload: payload}}
			if err := ValidateEventBatch(bad); !errors.Is(err, errs.ErrValidation) {
				t.Fatalf("case %d: expected ErrValidation for paste payload %#v, got %v", i, payload, err)
			}
		}

		zeroPaste := validBatch
		zeroPaste.Events = []EventInput{{Seq: 1, EventType: "editor.paste", Payload: map[string]any{"charCount": float64(0)}}}
		if err := ValidateEventBatch(zeroPaste); err != nil {
			t.Fatalf("expected charCount 0 paste to pass, got %v", err)
		}
	})

	t.Run("rejects oversized payload and invalid question index", func(t *testing.T) {
		big := validBatch
		big.Events = []EventInput{{Seq: 1, EventType: "code.snapshot", Payload: map[string]any{"source": strings.Repeat("a", 8200)}}}
		if err := ValidateEventBatch(big); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("expected ErrValidation for oversized payload, got %v", err)
		}

		badIdx := 50
		badQ := validBatch
		badQ.Events = []EventInput{{Seq: 1, EventType: "nav.question", QuestionIndex: &badIdx, Payload: map[string]any{}}}
		if err := ValidateEventBatch(badQ); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("expected ErrValidation for questionIndex=50, got %v", err)
		}
	})
}

func TestCompareSolutions(t *testing.T) {
	t.Run("Python renamed identifiers and comments match", func(t *testing.T) {
		starter := "def solve(nums):\n    # TODO: implement\n    pass\n"
		codeA := `def solve(nums):
    # sum positive elements
    total = 0
    for val in nums:
        if val > 0:
            total += val
    return total
`
		codeB := `def solve(arr):
    # totally different comment # still ignored
    acc = 0
    for item in arr:
        if item > 0:
            acc += item
    return acc
`
		sim, matched, snipA, snipB := CompareSolutions("python", starter, codeA, codeB)
		if sim < 0.95 {
			t.Fatalf("expected similarity >= 0.95 for renamed copy, got %v", sim)
		}
		if matched == 0 || snipA == "" || snipB == "" {
			t.Fatalf("expected matched tokens and snippets, got matched=%d snipA=%q snipB=%q", matched, snipA, snipB)
		}
	})

	t.Run("Java block and line comments stripped while starter subtracted", func(t *testing.T) {
		starter := `public class Solution {
    public static int solve(int[] a) {
        return 0;
    }
}`
		codeA := `public class Solution {
    /* compute max prefix */
    public static int solve(int[] nums) {
        int best = -1000000;
        int cur = 0;
        for (int x : nums) {
            cur = Math.max(x, cur + x);
            best = Math.max(best, cur);
        }
        return best;
    }
}`
		codeB := `public class Solution {
    public static int solve(int[] values) {
        // Kadane's algorithm
        int ans = -1000000;
        int running = 0;
        for (int v : values) {
            running = Math.max(v, running + v);
            ans = Math.max(ans, running);
        }
        return ans;
    }
}`
		sim, matched, snipA, snipB := CompareSolutions("java", starter, codeA, codeB)
		if sim < SimilarityFlagThreshold {
			t.Fatalf("expected similarity >= %.2f, got %v", SimilarityFlagThreshold, sim)
		}
		if matched == 0 || snipA == "" || snipB == "" {
			t.Fatalf("expected matched tokens and snippets, got matched=%d", matched)
		}
	})

	t.Run("starter-only solutions yield zero similarity", func(t *testing.T) {
		starter := "def solve(nums):\n    return 0\n"
		sim, matched, snipA, snipB := CompareSolutions("python", starter, starter, starter)
		if sim != 0 || matched != 0 || snipA != "" || snipB != "" {
			t.Fatalf("expected zero match for starter-only code, got sim=%v matched=%d snipA=%q snipB=%q", sim, matched, snipA, snipB)
		}
	})

	t.Run("unrelated solutions stay below threshold", func(t *testing.T) {
		starter := "def solve(n):\n    pass\n"
		codeA := `def solve(n):
    memo = {0: 0, 1: 1}
    for i in range(2, n + 1):
        memo[i] = memo[i - 1] + memo[i - 2]
    return memo[n]
`
		codeB := `def solve(n):
    if n <= 1:
        return n
    while n > 10:
        n //= 2
    return n * 3 + 7
`
		sim, _, _, _ := CompareSolutions("python", starter, codeA, codeB)
		if sim >= SimilarityFlagThreshold {
			t.Fatalf("expected distinct implementations below %.2f, got %v", SimilarityFlagThreshold, sim)
		}
	})

	t.Run("short solutions handled cleanly", func(t *testing.T) {
		sim, matched, _, _ := CompareSolutions("python", "", "print(1)", "print(2)")
		if sim != 1.0 || matched == 0 {
			t.Fatalf("expected normalized short literal comparison to match, got sim=%v matched=%d", sim, matched)
		}
	})
}

func TestMissingSeqsAndIntervals(t *testing.T) {
	highest, missing := MissingSeqs(nil)
	if highest != 0 || len(missing) != 0 {
		t.Fatalf("expected empty result for nil seqs, got highest=%d missing=%v", highest, missing)
	}
	if gaps := MissingIntervals(nil); len(gaps) != 0 {
		t.Fatalf("expected empty gaps for nil seqs, got %v", gaps)
	}

	seqs := []int64{9, 2, 6, 3}
	highest, missing = MissingSeqs(seqs)
	if highest != 9 {
		t.Fatalf("expected highest=9, got %d", highest)
	}
	wantMissing := []int64{1, 4, 5, 7, 8}
	if len(missing) != len(wantMissing) {
		t.Fatalf("expected missing=%v, got %v", wantMissing, missing)
	}
	for i := range wantMissing {
		if missing[i] != wantMissing[i] {
			t.Fatalf("expected missing=%v, got %v", wantMissing, missing)
		}
	}

	gaps := MissingIntervals(seqs)
	wantGaps := []SequenceGap{
		{FromSeq: 1, ToSeq: 1},
		{FromSeq: 4, ToSeq: 5},
		{FromSeq: 7, ToSeq: 8},
	}
	if len(gaps) != len(wantGaps) {
		t.Fatalf("expected gaps=%v, got %v", wantGaps, gaps)
	}
	for i := range wantGaps {
		if gaps[i] != wantGaps[i] {
			t.Fatalf("expected gaps=%v, got %v", wantGaps, gaps)
		}
	}
}

func TestDetectBatchSignals(t *testing.T) {
	q := 1
	signals := DetectBatchSignals([]EventInput{
		{Seq: 1, EventType: "editor.paste", QuestionIndex: &q, Payload: map[string]any{"charCount": MaxPasteAutoFlagChars - 1}},
		{Seq: 2, EventType: "editor.paste", QuestionIndex: &q, Payload: map[string]any{"charCount": MaxPasteAutoFlagChars}},
		{Seq: 3, EventType: "fullscreen.exit", Payload: map[string]any{}},
	})
	if len(signals) != 2 {
		t.Fatalf("expected 2 signals, got %v", signals)
	}
	if signals[0].Kind != "large_paste" || signals[1].Kind != "fullscreen_exit" {
		t.Fatalf("unexpected signal kinds: %+v", signals)
	}
}

func TestProctoringEventsAndSignals(t *testing.T) {
	const (
		attemptID = "11111111-1111-4111-8111-111111111111"
		deviceID  = "22222222-2222-4222-8222-222222222222"
		batchID   = "33333333-3333-4333-8333-333333333333"
	)
	now := time.Now().UTC()
	events := []EventInput{
		{
			Seq:       1,
			EventType: "app.opened",
			ClientAt:  &now,
			Payload:   map[string]any{"apps": []any{map[string]any{"name": "Safari", "bundleId": "com.apple.Safari", "pid": 42}}},
		},
		{Seq: 2, EventType: "proctor.camera_lost", ClientAt: &now, Payload: map[string]any{}},
		{Seq: 3, EventType: "proctor.mic_lost", ClientAt: &now, Payload: map[string]any{}},
		{Seq: 4, EventType: "proctor.screen_lost", ClientAt: &now, Payload: map[string]any{}},
	}
	if err := ValidateEventBatch(EventBatchInput{
		AttemptID: attemptID,
		DeviceID:  deviceID,
		BatchID:   batchID,
		Events:    events,
	}); err != nil {
		t.Fatalf("expected proctoring events batch to validate, got %v", err)
	}

	signals := DetectBatchSignals(events)
	if len(signals) != 4 {
		t.Fatalf("expected 4 high-severity signals, got %+v", signals)
	}
	wantKinds := []string{"app_opened", "proctor_camera_lost", "proctor_mic_lost", "proctor_screen_lost"}
	for i, kind := range wantKinds {
		if signals[i].Kind != kind || signals[i].Severity != "high" {
			t.Fatalf("signal %d = %+v, want kind=%q severity=high", i, signals[i], kind)
		}
	}
	if !strings.Contains(signals[0].Summary, "Safari") {
		t.Fatalf("expected app name in summary, got %q", signals[0].Summary)
	}
}

func TestValidateProctorMediaInput(t *testing.T) {
	const (
		attemptID = "11111111-1111-4111-8111-111111111111"
		deviceID  = "22222222-2222-4222-8222-222222222222"
	)
	now := time.Now().UTC()

	cases := []struct {
		rawType  string
		wantType string
		wantExt  string
	}{
		{"video/webm;codecs=vp8,opus", "video/webm", "webm"},
		{"video/mp4", "video/mp4", "mp4"},
		{"audio/webm;codecs=opus", "audio/webm", "webm"},
		{"image/jpeg", "image/jpeg", "jpg"},
		{"image/png", "image/png", "png"},
	}
	for _, tc := range cases {
		in := ProctorMediaInput{
			AttemptID:   attemptID,
			DeviceID:    deviceID,
			Kind:        "camera_audio",
			Seq:         1,
			ContentType: tc.rawType,
			DurationMS:  15000,
			CapturedAt:  now,
			ByteSize:    1024,
		}
		ext, err := ValidateProctorMediaInput(&in)
		if err != nil {
			t.Fatalf("expected %q to validate, got %v", tc.rawType, err)
		}
		if ext != tc.wantExt || in.ContentType != tc.wantType {
			t.Fatalf("got ext=%q contentType=%q, want ext=%q contentType=%q", ext, in.ContentType, tc.wantExt, tc.wantType)
		}
	}

	invalidCases := []ProctorMediaInput{
		{AttemptID: "bad", DeviceID: deviceID, Kind: "camera_audio", Seq: 1, ContentType: "video/webm", CapturedAt: now, ByteSize: 10},
		{AttemptID: attemptID, DeviceID: deviceID, Kind: "audio_only", Seq: 1, ContentType: "video/webm", CapturedAt: now, ByteSize: 10},
		{AttemptID: attemptID, DeviceID: deviceID, Kind: "screen_snapshot", Seq: 0, ContentType: "image/jpeg", CapturedAt: now, ByteSize: 10},
		{AttemptID: attemptID, DeviceID: deviceID, Kind: "screen_snapshot", Seq: 1, ContentType: "image/svg+xml", CapturedAt: now, ByteSize: 10},
		{AttemptID: attemptID, DeviceID: deviceID, Kind: "screen_snapshot", Seq: 1, ContentType: "text/html", CapturedAt: now, ByteSize: 10},
		{AttemptID: attemptID, DeviceID: deviceID, Kind: "screen_video", Seq: 1, ContentType: "video/webm", CapturedAt: now, ByteSize: 0},
		{AttemptID: attemptID, DeviceID: deviceID, Kind: "screen_video", Seq: 1, ContentType: "video/webm", CapturedAt: now, ByteSize: MaxProctorMediaBytes + 1},
		{AttemptID: attemptID, DeviceID: deviceID, Kind: "screen_video", Seq: 1, ContentType: "video/webm", CapturedAt: time.Time{}, ByteSize: 10},
		{AttemptID: attemptID, DeviceID: deviceID, Kind: "screen_video", Seq: 1, ContentType: "video/webm", DurationMS: -1, CapturedAt: now, ByteSize: 10},
	}
	for i, bad := range invalidCases {
		if _, err := ValidateProctorMediaInput(&bad); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("case %d: expected ErrValidation for %+v, got %v", i, bad, err)
		}
	}
}

func TestValidateDecisionsAndAppeals(t *testing.T) {
	dec := ReviewDecisionInput{Status: "cleared", Reason: "  Verified legitimate work.  "}
	if err := ValidateReviewDecision(&dec); err != nil {
		t.Fatalf("expected valid decision, got %v", err)
	}
	if dec.Reason != "Verified legitimate work." {
		t.Fatalf("expected trimmed reason, got %q", dec.Reason)
	}

	for _, bad := range []ReviewDecisionInput{
		{Status: "flagged", Reason: "auto"},
		{Status: "clear", Reason: "ok"},
		{Status: "confirmed_violation", Reason: "   "},
		{Status: "cleared", Reason: strings.Repeat("x", 2001)},
	} {
		if err := ValidateReviewDecision(&bad); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("expected ErrValidation for %+v, got %v", bad, err)
		}
	}

	ap := AppealInput{Kind: "appeal", Message: "  Screen reader focus shifted. "}
	if err := ValidateAppeal(&ap); err != nil {
		t.Fatalf("expected valid appeal, got %v", err)
	}
	if err := ValidateAppeal(&AppealInput{Kind: "other", Message: "hi"}); !errors.Is(err, errs.ErrValidation) {
		t.Fatalf("expected ErrValidation for invalid appeal kind")
	}
	if err := ValidateAppeal(&AppealInput{Kind: "concern", Message: "  "}); !errors.Is(err, errs.ErrValidation) {
		t.Fatalf("expected ErrValidation for blank appeal message")
	}

	res := ResolveAppealInput{Status: "resolved", Resolution: "  Cleared after review. "}
	if err := ValidateResolveAppeal(&res); err != nil {
		t.Fatalf("expected valid resolution, got %v", err)
	}
	if err := ValidateResolveAppeal(&ResolveAppealInput{Status: "open", Resolution: "ok"}); !errors.Is(err, errs.ErrValidation) {
		t.Fatalf("expected ErrValidation for invalid resolution status")
	}
	if err := ValidateResolveAppeal(&ResolveAppealInput{Status: "dismissed", Resolution: ""}); !errors.Is(err, errs.ErrValidation) {
		t.Fatalf("expected ErrValidation for empty resolution")
	}
}
