package contest

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"mime"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MaxEventBatchSize              = 50
	MaxAttemptClientEvents         = 2000
	MaxPasteAutoFlagChars          = 400
	SimilarityFlagThreshold        = 0.70
	MaxProctorMediaBytes           = 5 * 1024 * 1024
	MaxProctorMediaItemsPerAttempt = 300

	maxEventPayloadBytes = 8192
	maxReasonRunes       = 2000
	maxSnippetBytes      = 600
	defaultNGramSize     = 5
)

type EventInput struct {
	Seq           int64          `json:"seq"`
	EventType     string         `json:"eventType"`
	QuestionIndex *int           `json:"questionIndex,omitempty"`
	ClientAt      *time.Time     `json:"clientAt,omitempty"`
	Payload       map[string]any `json:"payload"`
}

type EventBatchInput struct {
	AttemptID string       `json:"attemptId"`
	DeviceID  string       `json:"deviceId"`
	BatchID   string       `json:"batchId"`
	Events    []EventInput `json:"events"`
}

const MaxLockdownWarnings = 3

type EventBatchResult struct {
	LockdownWarnings  int     `json:"lockdownWarnings"`
	LockdownSubmitted bool    `json:"lockdownSubmitted"`
	Accepted          int     `json:"accepted"`
	Duplicate         bool    `json:"duplicate"`
	HighestSeq        int64   `json:"highestSeq"`
	MissingSeqs       []int64 `json:"missingSeqs"`
}

type ReplayEvent struct {
	Seq              int64          `json:"seq"`
	SourceKind       string         `json:"sourceKind"`
	EventType        string         `json:"eventType"`
	QuestionIndex    *int           `json:"questionIndex,omitempty"`
	ClientAt         *time.Time     `json:"clientAt,omitempty"`
	ServerReceivedAt time.Time      `json:"serverReceivedAt"`
	Payload          map[string]any `json:"payload"`
}

type SequenceGap struct {
	FromSeq int64 `json:"fromSeq"`
	ToSeq   int64 `json:"toSeq"`
}

type ReviewSignal struct {
	Kind       string     `json:"kind"`
	Summary    string     `json:"summary"`
	Severity   string     `json:"severity"`
	OccurredAt *time.Time `json:"occurredAt,omitempty"`
}

type ReviewSummary struct {
	AttemptID     string         `json:"attemptId"`
	StudentID     string         `json:"studentId"`
	StudentName   string         `json:"studentName"`
	Status        string         `json:"status"`
	SimilarityMax float64        `json:"similarityMax"`
	Signals       []ReviewSignal `json:"signals"`
	Reason        string         `json:"reason"`
	ReviewerID    string         `json:"reviewerId,omitempty"`
	ReviewedAt    *time.Time     `json:"reviewedAt,omitempty"`
	RetainUntil   time.Time      `json:"retainUntil"`
	OpenAppeals   int            `json:"openAppeals"`
}

type SimilarityPair struct {
	AssessmentID  string    `json:"assessmentId"`
	QuestionIndex int       `json:"questionIndex"`
	AttemptA      string    `json:"attemptA"`
	AttemptB      string    `json:"attemptB"`
	Similarity    float64   `json:"similarity"`
	MatchedTokens int       `json:"matchedTokens"`
	SnippetA      string    `json:"snippetA"`
	SnippetB      string    `json:"snippetB"`
	ComputedAt    time.Time `json:"computedAt"`
}

type AppealInput struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type Appeal struct {
	ID          string     `json:"id"`
	AttemptID   string     `json:"attemptId"`
	StudentID   string     `json:"studentId"`
	StudentName string     `json:"studentName,omitempty"`
	Kind        string     `json:"kind"`
	Message     string     `json:"message"`
	Status      string     `json:"status"`
	ReviewerID  string     `json:"reviewerId,omitempty"`
	Resolution  string     `json:"resolution,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	ResolvedAt  *time.Time `json:"resolvedAt,omitempty"`
}

type StudentIntegrityStatus struct {
	ReviewStatus string   `json:"reviewStatus"`
	Appeals      []Appeal `json:"appeals"`
}

type ReviewerInfo struct {
	ReviewerID   string    `json:"reviewerId"`
	ReviewerName string    `json:"reviewerName"`
	AssignedBy   string    `json:"assignedBy"`
	AssignedAt   time.Time `json:"assignedAt"`
}

type EvidenceAccessEntry struct {
	ID         int64     `json:"id"`
	AttemptID  string    `json:"attemptId,omitempty"`
	ActorID    string    `json:"actorId"`
	Action     string    `json:"action"`
	Detail     string    `json:"detail"`
	OccurredAt time.Time `json:"occurredAt"`
}

type ReviewQueueView struct {
	AssessmentID    string                `json:"assessmentId"`
	Reviews         []ReviewSummary       `json:"reviews"`
	SimilarityPairs []SimilarityPair      `json:"similarityPairs"`
	Appeals         []Appeal              `json:"appeals"`
	Reviewers       []ReviewerInfo        `json:"reviewers"`
	AccessLog       []EvidenceAccessEntry `json:"accessLog"`
}

type AttemptReplayView struct {
	Review           ReviewSummary    `json:"review"`
	Events           []ReplayEvent    `json:"events"`
	MissingIntervals []SequenceGap    `json:"missingIntervals"`
	SimilarityPairs  []SimilarityPair `json:"similarityPairs"`
	Appeals          []Appeal         `json:"appeals"`
}

type ReviewDecisionInput struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type ResolveAppealInput struct {
	Status     string `json:"status"`
	Resolution string `json:"resolution"`
}

type ProctorMediaItem struct {
	ID           string    `json:"id"`
	AttemptID    string    `json:"attemptId"`
	AssessmentID string    `json:"assessmentId"`
	Kind         string    `json:"kind"`
	Seq          int       `json:"seq"`
	ContentType  string    `json:"contentType"`
	ByteSize     int       `json:"byteSize"`
	DurationMS   int       `json:"durationMs"`
	CapturedAt   time.Time `json:"capturedAt"`
	CreatedAt    time.Time `json:"createdAt"`
	StorageKey   string    `json:"-"`
}

type ProctorMediaInput struct {
	AttemptID   string    `json:"attemptId"`
	DeviceID    string    `json:"deviceId"`
	Kind        string    `json:"kind"`
	Seq         int       `json:"seq"`
	ContentType string    `json:"contentType"`
	DurationMS  int       `json:"durationMs"`
	CapturedAt  time.Time `json:"capturedAt"`
	StorageKey  string    `json:"-"`
	ByteSize    int       `json:"-"`
}

type IntegrityRepository interface {
	IngestEvents(ctx context.Context, assessmentID, studentID string, batch EventBatchInput) (EventBatchResult, error)
	SubmitAppeal(ctx context.Context, assessmentID, studentID string, cmd AppealInput) (Appeal, error)
	ListStudentAppeals(ctx context.Context, assessmentID, studentID string) (StudentIntegrityStatus, error)
	AssignReviewer(ctx context.Context, actor Actor, assessmentID, reviewerID string) error
	ListReviewQueue(ctx context.Context, actor Actor, assessmentID string) (ReviewQueueView, error)
	GetAttemptReplay(ctx context.Context, actor Actor, assessmentID, attemptID string) (AttemptReplayView, error)
	DecideReview(ctx context.Context, actor Actor, assessmentID, attemptID string, cmd ReviewDecisionInput) (ReviewSummary, error)
	ResolveAppeal(ctx context.Context, actor Actor, assessmentID, appealID string, cmd ResolveAppealInput) (Appeal, error)
	ComputeSimilarity(ctx context.Context, actor Actor, assessmentID string) ([]SimilarityPair, error)
	PurgeExpiredEvidence(ctx context.Context) (int64, error)
	RecordProctorMedia(ctx context.Context, assessmentID, studentID string, in ProctorMediaInput) (ProctorMediaItem, error)
	ListProctorMedia(ctx context.Context, actor Actor, assessmentID, attemptID string) ([]ProctorMediaItem, error)
	GetProctorMedia(ctx context.Context, actor Actor, assessmentID, mediaID string) (ProctorMediaItem, error)
	PurgeExpiredProctorMedia(ctx context.Context, removeFile func(string) error) (int64, error)
}

var validClientEventTypes = map[string]bool{
	"nav.question":        true,
	"mcq.selected":        true,
	"code.snapshot":       true,
	"code.delta":          true,
	"code.run":            true,
	"editor.paste":        true,
	"window.focus":        true,
	"window.blur":         true,
	"fullscreen.enter":    true,
	"fullscreen.exit":     true,
	"heartbeat.gap":       true,
	"client.reconnect":    true,
	"app.opened":          true,
	"proctor.camera_lost": true,
	"proctor.mic_lost":    true,
	"proctor.screen_lost": true,
	"lockdown.warning":    true,
}

var validProctorMediaKinds = map[string]bool{
	"camera_audio":    true,
	"screen_snapshot": true,
	"screen_video":    true,
}

var proctorMediaExtensions = map[string]string{
	"video/webm":      "webm",
	"video/mp4":       "mp4",
	"video/quicktime": "mov",
	"audio/webm":      "webm",
	"audio/mp4":       "m4a",
	"image/jpeg":      "jpg",
	"image/png":       "png",
}

// NormalizeProctorMediaType validates a proctoring media MIME type against the
// allowlist (stripping optional parameters such as unquoted codecs) and returns
// the normalized base MIME type and file extension.
func NormalizeProctorMediaType(raw string) (baseType, ext string, ok bool) {
	base, _, _ := strings.Cut(strings.TrimSpace(raw), ";")
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(base))
	if err != nil {
		return "", "", false
	}
	mediaType = strings.ToLower(mediaType)
	ext, ok = proctorMediaExtensions[mediaType]
	if !ok {
		return "", "", false
	}
	return mediaType, ext, true
}

// ValidateProctorMediaInput validates a proctoring media upload payload,
// normalizing its MIME type and returning the corresponding file extension.
func ValidateProctorMediaInput(in *ProctorMediaInput) (string, error) {
	if !isUUID(in.AttemptID) || !isUUID(in.DeviceID) {
		return "", Invalid("Provide valid attempt and device identifiers.")
	}
	if !validProctorMediaKinds[in.Kind] {
		return "", Invalid("Unsupported proctoring media kind.")
	}
	if in.Seq < 1 {
		return "", Invalid("Media sequence number must be at least 1.")
	}
	if in.DurationMS < 0 {
		return "", Invalid("Media duration must be non-negative.")
	}
	if in.CapturedAt.IsZero() {
		return "", Invalid("Provide a valid media capture timestamp.")
	}
	if in.ByteSize <= 0 || in.ByteSize > MaxProctorMediaBytes {
		return "", Invalid(fmt.Sprintf("Media chunk size must be between 1 and %d bytes.", MaxProctorMediaBytes))
	}
	baseType, ext, ok := NormalizeProctorMediaType(in.ContentType)
	if !ok {
		return "", Invalid("Unsupported proctoring media content type.")
	}
	in.ContentType = baseType
	return ext, nil
}

var prohibitedPrivacyKeys = map[string]bool{
	"clipboard":        true,
	"clipboardtext":    true,
	"screen":           true,
	"webcam":           true,
	"microphone":       true,
	"globalkeystrokes": true,
	"unrelatedapp":     true,
	"keystrokes":       true,
}

var privacyKeyNormalizer = strings.NewReplacer("_", "", "-", "")

// ValidateEventBatch validates an incoming client integrity event batch.
func ValidateEventBatch(batch EventBatchInput) error {
	if !isUUID(batch.AttemptID) || !isUUID(batch.DeviceID) || !isUUID(batch.BatchID) {
		return Invalid("Provide valid attempt, device, and batch identifiers.")
	}
	if len(batch.Events) == 0 || len(batch.Events) > MaxEventBatchSize {
		return Invalid(fmt.Sprintf("Send between 1 and %d events per batch.", MaxEventBatchSize))
	}
	seenSeqs := make(map[int64]bool, len(batch.Events))
	for _, ev := range batch.Events {
		if seenSeqs[ev.Seq] {
			return Invalid("Batch event sequence numbers must be distinct.")
		}
		seenSeqs[ev.Seq] = true
		if err := ValidateEvent(ev); err != nil {
			return err
		}
	}
	return nil
}

// ValidateEvent validates a single client-reported replay event.
func ValidateEvent(ev EventInput) error {
	if ev.Seq <= 0 {
		return Invalid("Event sequence numbers must be positive.")
	}
	if !validClientEventTypes[ev.EventType] {
		return Invalid("Unsupported client event type.")
	}
	if ev.QuestionIndex != nil && (*ev.QuestionIndex < 0 || *ev.QuestionIndex > 49) {
		return Invalid("Question index must be between 0 and 49.")
	}
	payload := ev.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	if hasProhibitedPrivacyKey(payload) {
		return Invalid("Event payload contains prohibited privacy fields.")
	}
	if ev.EventType == "lockdown.warning" {
		reason, _ := payload["reason"].(string)
		if len(payload) != 1 || ev.QuestionIndex != nil || (reason != "left_contest" && reason != "paste_blocked") {
			return Invalid("Provide a supported lockdown warning reason.")
		}
	}
	if ev.EventType == "editor.paste" {
		if _, ok := ExtractCharCount(payload); !ok {
			return Invalid("Paste events must include a non-negative numeric charCount.")
		}
		if hasStringValue(payload) {
			return Invalid("Paste events may only report counts and must not include string content.")
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) > maxEventPayloadBytes {
		return Invalid("Event payload must be a JSON object of at most 8,192 bytes.")
	}
	return nil
}

// ExtractCharCount reads a non-negative integer charCount from an event payload.
func ExtractCharCount(payload map[string]any) (int, bool) {
	if payload == nil {
		return 0, false
	}
	val, exists := payload["charCount"]
	if !exists {
		return 0, false
	}
	switch v := val.(type) {
	case int:
		return v, v >= 0
	case int64:
		return int(v), v >= 0 && v <= math.MaxInt
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > float64(math.MaxInt32) || math.Trunc(v) != v {
			return 0, false
		}
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil && n >= 0 && n <= math.MaxInt32
	default:
		return 0, false
	}
}

func hasProhibitedPrivacyKey(v any) bool {
	switch node := v.(type) {
	case map[string]any:
		for k, child := range node {
			normalized := strings.ToLower(privacyKeyNormalizer.Replace(k))
			if prohibitedPrivacyKeys[normalized] {
				return true
			}
			if hasProhibitedPrivacyKey(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if hasProhibitedPrivacyKey(child) {
				return true
			}
		}
	case []map[string]any:
		for _, child := range node {
			if hasProhibitedPrivacyKey(child) {
				return true
			}
		}
	}
	return false
}

func hasStringValue(v any) bool {
	switch node := v.(type) {
	case string:
		return true
	case map[string]any:
		for _, child := range node {
			if hasStringValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if hasStringValue(child) {
				return true
			}
		}
	case []map[string]any:
		for _, child := range node {
			if hasStringValue(child) {
				return true
			}
		}
	}
	return false
}

// ValidateReviewDecision validates a human reviewer's decision input.
func ValidateReviewDecision(cmd *ReviewDecisionInput) error {
	switch cmd.Status {
	case "under_review", "cleared", "confirmed_violation":
	default:
		return Invalid("Review status must be under_review, cleared, or confirmed_violation.")
	}
	cmd.Reason = strings.TrimSpace(cmd.Reason)
	n := utf8.RuneCountInString(cmd.Reason)
	if n < 1 || n > maxReasonRunes {
		return Invalid("Provide a review reason between 1 and 2,000 characters.")
	}
	return nil
}

// ValidateAppeal validates a student's concern or appeal submission.
func ValidateAppeal(cmd *AppealInput) error {
	switch cmd.Kind {
	case "concern", "appeal":
	default:
		return Invalid("Submission kind must be concern or appeal.")
	}
	cmd.Message = strings.TrimSpace(cmd.Message)
	n := utf8.RuneCountInString(cmd.Message)
	if n < 1 || n > maxReasonRunes {
		return Invalid("Provide a message between 1 and 2,000 characters.")
	}
	return nil
}

// ValidateResolveAppeal validates a reviewer's resolution for a student appeal.
func ValidateResolveAppeal(cmd *ResolveAppealInput) error {
	switch cmd.Status {
	case "in_review", "resolved", "dismissed":
	default:
		return Invalid("Appeal status must be in_review, resolved, or dismissed.")
	}
	cmd.Resolution = strings.TrimSpace(cmd.Resolution)
	n := utf8.RuneCountInString(cmd.Resolution)
	if n < 1 || n > maxReasonRunes {
		return Invalid("Provide an appeal resolution between 1 and 2,000 characters.")
	}
	return nil
}

// DetectBatchSignals derives review signals from a validated event batch.
// Automated detection only surfaces signals for human review and never sets confirmed_violation.
func DetectBatchSignals(events []EventInput) []ReviewSignal {
	signals := make([]ReviewSignal, 0)
	for _, ev := range events {
		switch ev.EventType {
		case "editor.paste":
			if chars, ok := ExtractCharCount(ev.Payload); ok && chars >= MaxPasteAutoFlagChars {
				severity := "medium"
				if chars >= MaxPasteAutoFlagChars*2 {
					severity = "high"
				}
				summary := fmt.Sprintf("Large paste detected (%d chars)", chars)
				if ev.QuestionIndex != nil {
					summary = fmt.Sprintf("Large paste detected on Q%d (%d chars)", *ev.QuestionIndex+1, chars)
				}
				signals = append(signals, ReviewSignal{
					Kind:       "large_paste",
					Summary:    summary,
					Severity:   severity,
					OccurredAt: ev.ClientAt,
				})
			}
		case "fullscreen.exit":
			signals = append(signals, ReviewSignal{
				Kind:       "fullscreen_exit",
				Summary:    "Exited contest fullscreen mode",
				Severity:   "medium",
				OccurredAt: ev.ClientAt,
			})
		case "app.opened":
			summary := "External application opened during contest"
			if names := extractOpenedAppNames(ev.Payload); len(names) > 0 {
				summary = fmt.Sprintf("External application opened during contest (%s)", strings.Join(names, ", "))
			}
			signals = append(signals, ReviewSignal{
				Kind:       "app_opened",
				Summary:    summary,
				Severity:   "high",
				OccurredAt: ev.ClientAt,
			})
		case "proctor.camera_lost":
			signals = append(signals, ReviewSignal{
				Kind:       "proctor_camera_lost",
				Summary:    "Camera proctoring stream lost",
				Severity:   "high",
				OccurredAt: ev.ClientAt,
			})
		case "proctor.mic_lost":
			signals = append(signals, ReviewSignal{
				Kind:       "proctor_mic_lost",
				Summary:    "Microphone proctoring stream lost",
				Severity:   "high",
				OccurredAt: ev.ClientAt,
			})
		case "proctor.screen_lost":
			signals = append(signals, ReviewSignal{
				Kind:       "proctor_screen_lost",
				Summary:    "Screen proctoring stream lost",
				Severity:   "high",
				OccurredAt: ev.ClientAt,
			})
		}
	}
	return signals
}

func extractOpenedAppNames(payload map[string]any) []string {
	if payload == nil {
		return nil
	}
	rawApps, ok := payload["apps"].([]any)
	if !ok || len(rawApps) == 0 {
		return nil
	}
	names := make([]string, 0, min(len(rawApps), 3))
	for _, item := range rawApps {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			name, _ = m["bundleId"].(string)
			name = strings.TrimSpace(name)
		}
		if name != "" {
			names = append(names, name)
			if len(names) >= 3 {
				break
			}
		}
	}
	return names
}

// MissingIntervals computes contiguous missing sequence intervals starting from 1.
func MissingIntervals(seqs []int64) []SequenceGap {
	if len(seqs) == 0 {
		return []SequenceGap{}
	}
	sorted := slices.Clone(seqs)
	slices.Sort(sorted)
	gaps := make([]SequenceGap, 0)
	var expected int64 = 1
	for _, s := range sorted {
		if s < expected {
			continue
		}
		if s > expected {
			gaps = append(gaps, SequenceGap{FromSeq: expected, ToSeq: s - 1})
		}
		expected = s + 1
	}
	return gaps
}

// MissingSeqs returns the highest sequence number and individual missing sequence numbers (capped at 500).
func MissingSeqs(seqs []int64) (int64, []int64) {
	if len(seqs) == 0 {
		return 0, []int64{}
	}
	sorted := slices.Clone(seqs)
	slices.Sort(sorted)
	highest := sorted[len(sorted)-1]
	gaps := MissingIntervals(sorted)
	missing := make([]int64, 0)
	const maxReturnedMissing = 500
	for _, g := range gaps {
		for s := g.FromSeq; s <= g.ToSeq; s++ {
			missing = append(missing, s)
			if len(missing) >= maxReturnedMissing {
				return highest, missing
			}
		}
	}
	return highest, missing
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

type sourceToken struct {
	text  string
	start int
	end   int
}

var pythonKeywords = map[string]bool{
	"False": true, "None": true, "True": true, "and": true, "as": true, "assert": true,
	"async": true, "await": true, "break": true, "class": true, "continue": true, "def": true,
	"del": true, "elif": true, "else": true, "except": true, "finally": true, "for": true,
	"from": true, "global": true, "if": true, "import": true, "in": true, "is": true,
	"lambda": true, "nonlocal": true, "not": true, "or": true, "pass": true, "raise": true,
	"return": true, "try": true, "while": true, "with": true, "yield": true,
}

var javaKeywords = map[string]bool{
	"abstract": true, "assert": true, "boolean": true, "break": true, "byte": true, "case": true,
	"catch": true, "char": true, "class": true, "continue": true, "default": true, "do": true,
	"double": true, "else": true, "enum": true, "extends": true, "false": true, "final": true,
	"finally": true, "float": true, "for": true, "if": true, "implements": true, "import": true,
	"instanceof": true, "int": true, "interface": true, "long": true, "new": true, "null": true,
	"package": true, "private": true, "protected": true, "public": true, "return": true,
	"short": true, "static": true, "super": true, "switch": true, "synchronized": true,
	"this": true, "throw": true, "throws": true, "true": true, "try": true, "void": true,
	"while": true, "String": true,
}

// CompareSolutions normalizes comments, whitespace, and identifiers, subtracts starter
// scaffolding n-grams, and computes Jaccard similarity plus matched source snippets.
func CompareSolutions(language, starter, codeA, codeB string) (similarity float64, matchedTokens int, snippetA, snippetB string) {
	tokensA := tokenizeSource(language, codeA)
	tokensB := tokenizeSource(language, codeB)
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0, 0, "", ""
	}
	k := defaultNGramSize
	if minLen := min(len(tokensA), len(tokensB)); minLen < k {
		k = minLen
	}
	if k < 1 {
		return 0, 0, "", ""
	}

	tokensStarter := tokenizeSource(language, starter)
	starterGrams := buildNGramCounts(tokensStarter, k)
	gramsA, occA := buildNGramPositions(tokensA, k)
	gramsB, occB := buildNGramPositions(tokensB, k)

	remA := subtractCounts(gramsA, starterGrams)
	remB := subtractCounts(gramsB, starterGrams)
	if len(remA) == 0 || len(remB) == 0 {
		return 0, 0, "", ""
	}

	var intersection, union int
	matchedCounts := make(map[string]int)
	for g, ca := range remA {
		cb := remB[g]
		if cb > 0 {
			m := min(ca, cb)
			intersection += m
			matchedCounts[g] = m
			union += max(ca, cb)
		} else {
			union += ca
		}
	}
	for g, cb := range remB {
		if _, seen := remA[g]; !seen {
			union += cb
		}
	}
	if union == 0 || intersection == 0 {
		return 0, 0, "", ""
	}

	similarity = float64(intersection) / float64(union)
	if similarity > 1 {
		similarity = 1
	}

	markedA := markMatchedTokens(len(tokensA), k, starterGrams, matchedCounts, occA)
	markedB := markMatchedTokens(len(tokensB), k, starterGrams, matchedCounts, occB)
	countA := countMarked(markedA)
	countB := countMarked(markedB)
	matchedTokens = min(countA, countB)
	if matchedTokens == 0 {
		return 0, 0, "", ""
	}

	snippetA = extractBestSnippet(codeA, tokensA, markedA)
	snippetB = extractBestSnippet(codeB, tokensB, markedB)
	return similarity, matchedTokens, snippetA, snippetB
}

func buildNGramCounts(tokens []sourceToken, k int) map[string]int {
	if len(tokens) < k {
		return nil
	}
	counts := make(map[string]int, len(tokens)-k+1)
	for i := 0; i <= len(tokens)-k; i++ {
		counts[joinNGram(tokens[i:i+k])]++
	}
	return counts
}

func buildNGramPositions(tokens []sourceToken, k int) (map[string]int, map[string][]int) {
	counts := make(map[string]int, len(tokens)-k+1)
	positions := make(map[string][]int, len(tokens)-k+1)
	for i := 0; i <= len(tokens)-k; i++ {
		key := joinNGram(tokens[i : i+k])
		counts[key]++
		positions[key] = append(positions[key], i)
	}
	return counts, positions
}

func joinNGram(window []sourceToken) string {
	var b strings.Builder
	for i, tok := range window {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(tok.text)
	}
	return b.String()
}

func subtractCounts(grams, starter map[string]int) map[string]int {
	out := make(map[string]int, len(grams))
	for g, c := range grams {
		if rem := c - starter[g]; rem > 0 {
			out[g] = rem
		}
	}
	return out
}

func markMatchedTokens(tokenLen, k int, starter, matchedCounts map[string]int, occ map[string][]int) []bool {
	marked := make([]bool, tokenLen)
	for g, need := range matchedCounts {
		starts := occ[g]
		skip := starter[g]
		applied := 0
		for idx := skip; idx < len(starts) && applied < need; idx++ {
			pos := starts[idx]
			for offset := 0; offset < k; offset++ {
				marked[pos+offset] = true
			}
			applied++
		}
	}
	return marked
}

func countMarked(marked []bool) int {
	n := 0
	for _, m := range marked {
		if m {
			n++
		}
	}
	return n
}

func extractBestSnippet(src string, tokens []sourceToken, marked []bool) string {
	bestStart, bestEnd, bestLen := -1, -1, 0
	curStart := -1
	for i, m := range marked {
		if m {
			if curStart == -1 {
				curStart = i
			}
			if curLen := i - curStart + 1; curLen > bestLen {
				bestLen = curLen
				bestStart = curStart
				bestEnd = i
			}
		} else {
			curStart = -1
		}
	}
	if bestStart == -1 || bestEnd < bestStart {
		return ""
	}
	startByte := tokens[bestStart].start
	endByte := tokens[bestEnd].end
	if startByte < 0 || endByte > len(src) || startByte >= endByte {
		return ""
	}
	snippet := strings.TrimSpace(src[startByte:endByte])
	if len(snippet) > maxSnippetBytes {
		cut := maxSnippetBytes
		for cut > 0 && !utf8.RuneStart(snippet[cut]) {
			cut--
		}
		snippet = strings.TrimSpace(snippet[:cut]) + "..."
	}
	return snippet
}

func tokenizeSource(language, src string) []sourceToken {
	if strings.TrimSpace(src) == "" {
		return nil
	}
	lang := strings.ToLower(strings.TrimSpace(language))
	keywords := javaKeywords
	isPython := lang == "python" || lang == "py"
	if isPython {
		keywords = pythonKeywords
	}
	cleaned := stripCommentsPreservingOffsets(src, isPython)
	tokens := make([]sourceToken, 0, len(cleaned)/4)

	for i := 0; i < len(cleaned); {
		b := cleaned[i]
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			i++
			continue
		}
		if isPython && (strings.HasPrefix(cleaned[i:], `"""`) || strings.HasPrefix(cleaned[i:], `'''`)) {
			end := scanTripleString(cleaned, i)
			tokens = append(tokens, sourceToken{text: "STR", start: i, end: end})
			i = end
			continue
		}
		if b == '"' || b == '\'' {
			end := scanQuotedLiteral(cleaned, i, b)
			tokens = append(tokens, sourceToken{text: "STR", start: i, end: end})
			i = end
			continue
		}
		if b >= '0' && b <= '9' {
			end := scanNumber(cleaned, i)
			tokens = append(tokens, sourceToken{text: "NUM", start: i, end: end})
			i = end
			continue
		}
		r, width := utf8.DecodeRuneInString(cleaned[i:])
		if unicode.IsLetter(r) || r == '_' {
			end := scanIdent(cleaned, i)
			word := cleaned[i:end]
			tokText := "ID"
			if keywords[word] {
				tokText = "kw:" + word
			}
			tokens = append(tokens, sourceToken{text: tokText, start: i, end: end})
			i = end
			continue
		}
		if width > 1 {
			tokens = append(tokens, sourceToken{text: cleaned[i : i+width], start: i, end: i + width})
			i += width
			continue
		}
		opLen := matchMultiCharOp(cleaned[i:])
		tokens = append(tokens, sourceToken{text: cleaned[i : i+opLen], start: i, end: i + opLen})
		i += opLen
	}
	return tokens
}

// stripCommentsPreservingOffsets replaces comment bytes with ASCII spaces so token
// byte offsets still index directly into the original source string.
func stripCommentsPreservingOffsets(src string, isPython bool) string {
	buf := []byte(src)
	n := len(buf)
	for i := 0; i < n; {
		if isPython {
			if i+2 < n && ((buf[i] == '"' && buf[i+1] == '"' && buf[i+2] == '"') ||
				(buf[i] == '\'' && buf[i+1] == '\'' && buf[i+2] == '\'')) {
				i = scanTripleString(src, i)
				continue
			}
			if buf[i] == '"' || buf[i] == '\'' {
				i = scanQuotedLiteral(src, i, buf[i])
				continue
			}
			if buf[i] == '#' {
				for i < n && buf[i] != '\n' {
					buf[i] = ' '
					i++
				}
				continue
			}
			i++
			continue
		}

		if buf[i] == '"' || buf[i] == '\'' {
			i = scanQuotedLiteral(src, i, buf[i])
			continue
		}
		if i+1 < n && buf[i] == '/' && buf[i+1] == '/' {
			for i < n && buf[i] != '\n' {
				buf[i] = ' '
				i++
			}
			continue
		}
		if i+1 < n && buf[i] == '/' && buf[i+1] == '*' {
			buf[i], buf[i+1] = ' ', ' '
			i += 2
			for i < n {
				if i+1 < n && buf[i] == '*' && buf[i+1] == '/' {
					buf[i], buf[i+1] = ' ', ' '
					i += 2
					break
				}
				if buf[i] != '\n' {
					buf[i] = ' '
				}
				i++
			}
			continue
		}
		i++
	}
	return string(buf)
}

func scanTripleString(src string, start int) int {
	quote := src[start : start+3]
	i := start + 3
	for i < len(src) {
		if src[i] == '\\' && i+1 < len(src) {
			i += 2
			continue
		}
		if strings.HasPrefix(src[i:], quote) {
			return i + 3
		}
		i++
	}
	return len(src)
}

func scanQuotedLiteral(src string, start int, quote byte) int {
	i := start + 1
	for i < len(src) {
		if src[i] == '\\' && i+1 < len(src) {
			i += 2
			continue
		}
		if src[i] == quote {
			return i + 1
		}
		if src[i] == '\n' {
			return i
		}
		i++
	}
	return len(src)
}

func scanNumber(src string, start int) int {
	i := start
	for i < len(src) {
		b := src[i]
		if (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F') ||
			b == 'x' || b == 'X' || b == 'b' || b == 'B' || b == 'o' || b == 'O' ||
			b == '_' || b == '.' || b == 'L' || b == 'l' {
			i++
			continue
		}
		if (b == '+' || b == '-') && i > start && (src[i-1] == 'e' || src[i-1] == 'E') {
			i++
			continue
		}
		break
	}
	return i
}

func scanIdent(src string, start int) int {
	i := start
	for i < len(src) {
		r, width := utf8.DecodeRuneInString(src[i:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			i += width
			continue
		}
		break
	}
	return i
}

func matchMultiCharOp(s string) int {
	for _, op := range []string{"**=", "//=", "<<=", ">>=", ">>>", "==", "!=", "<=", ">=", "&&", "||", "++", "--", "+=", "-=", "*=", "/=", "%=", "//", "**", "->", "::", "<<", ">>", ":="} {
		if strings.HasPrefix(s, op) {
			return len(op)
		}
	}
	return 1
}
