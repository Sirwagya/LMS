import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { api } from "../api";
import { contestTime } from "./types";
import "./contestAttempt.css";

type ReviewStatus = "pending" | "under_review" | "cleared" | "confirmed_violation";
type AppealStatus = "open" | "in_review" | "resolved" | "dismissed";

export type ReviewSignal = {
  kind: string;
  summary: string;
  severity: string;
  occurredAt?: string;
};

export type IntegrityReviewSummary = {
  attemptId: string;
  studentId: string;
  studentName?: string;
  studentEmail?: string;
  status: ReviewStatus | string;
  similarityMax?: number;
  maxSimilarity?: number;
  signals?: ReviewSignal[];
  reason?: string;
  signalCount?: number;
  pasteCount?: number;
  focusLossCount?: number;
  gapCount?: number;
  missingBatchCount?: number;
  reviewerId?: string;
  reviewedBy?: string;
  reviewedAt?: string;
  retainUntil?: string;
  openAppeals?: number;
  updatedAt?: string;
  submittedAt?: string;
};

export type SimilarityPair = {
  id?: string;
  assessmentId?: string;
  attemptAId?: string;
  attemptA?: string;
  attemptBId?: string;
  attemptB?: string;
  studentAId?: string;
  studentBId?: string;
  studentAName?: string;
  studentBName?: string;
  questionIndex?: number;
  language?: string;
  score?: number;
  similarity?: number;
  matchedTokens?: number | string[];
  snippetPreview?: string;
  normalizedPreview?: string;
  snippetA?: string;
  snippetB?: string;
  computedAt?: string;
};

export type StaffAppeal = {
  id: string;
  attemptId?: string;
  studentId: string;
  studentName?: string;
  kind: "concern" | "appeal";
  message: string;
  status: AppealStatus;
  reviewerId?: string;
  resolution?: string;
  resolvedBy?: string;
  resolvedAt?: string;
  createdAt?: string;
};

export type AssignedReviewer = {
  reviewerId?: string;
  reviewerName?: string;
  userId?: string;
  id?: string;
  name?: string;
  email?: string;
  assignedBy?: string;
  assignedAt?: string;
};

export type AccessLogEntry = {
  id?: string | number;
  actorId?: string;
  userId?: string;
  actorName?: string;
  action: string;
  detail?: string;
  attemptId?: string;
  occurredAt?: string;
  createdAt?: string;
};

export type IntegrityOverviewResponse = {
  assessmentId: string;
  reviews: IntegrityReviewSummary[];
  similarityPairs: SimilarityPair[];
  appeals: StaffAppeal[];
  reviewers: AssignedReviewer[];
  accessLog: AccessLogEntry[];
};

type MissingInterval = {
  fromSeq?: number;
  toSeq?: number;
  startSeq?: number;
  endSeq?: number;
  from?: number;
  to?: number;
};

type ReplayEvent = {
  id?: string;
  sourceKind?: "server" | "client" | string;
  trust?: "server" | "client" | string;
  source?: "server" | "client" | string;
  authoritative?: boolean;
  eventType?: string;
  type?: string;
  seq?: number;
  clientAt?: string;
  occurredAt?: string;
  serverReceivedAt?: string;
  recordedAt?: string;
  questionIndex?: number;
  payload?: Record<string, unknown>;
  data?: Record<string, unknown>;
  late?: boolean;
};

type ReplayResponse = {
  review?: IntegrityReviewSummary;
  attemptId?: string;
  studentId?: string;
  studentName?: string;
  status?: string;
  reason?: string;
  events: ReplayEvent[];
  missingIntervals?: MissingInterval[];
  similarityPairs?: SimilarityPair[];
  similarityMatches?: SimilarityPair[];
  appeals?: StaffAppeal[];
};

type DiffLine = { kind: "same" | "add" | "remove"; text: string };

const reviewDecisionLabels: Record<string, string> = {
  pending: "Pending",
  under_review: "Under review",
  cleared: "Cleared",
  confirmed_violation: "Confirmed violation",
};

const appealStatusLabels: Record<AppealStatus, string> = {
  open: "Open",
  in_review: "In review",
  resolved: "Resolved",
  dismissed: "Dismissed",
};

function eventTypeName(event: ReplayEvent): string {
  return event.eventType ?? event.type ?? "event";
}

function isServerEvent(event: ReplayEvent): boolean {
  if (typeof event.authoritative === "boolean") return event.authoritative;
  const origin = (event.sourceKind ?? event.trust ?? event.source ?? "").toLowerCase();
  if (origin === "server") return true;
  if (origin === "client") return false;
  return event.seq === undefined || event.seq === 0;
}

function eventPayload(event: ReplayEvent): Record<string, unknown> {
  return event.payload ?? event.data ?? {};
}

function eventTimestamp(event: ReplayEvent): string {
  return event.clientAt ?? event.occurredAt ?? event.serverReceivedAt ?? event.recordedAt ?? "";
}

function eventQuestionIndex(event: ReplayEvent): number | undefined {
  if (typeof event.questionIndex === "number") return event.questionIndex;
  const payload = eventPayload(event);
  return typeof payload.questionIndex === "number" ? payload.questionIndex : undefined;
}

function formatMissingInterval(interval: MissingInterval): string {
  const from = interval.fromSeq ?? interval.startSeq ?? interval.from ?? 0;
  const to = interval.toSeq ?? interval.endSeq ?? interval.to ?? from;
  return from === to
    ? `Missing client batch seq ${from}`
    : `Missing client batch seq ${from}–${to}`;
}

function formatSimilarityPercent(pair: SimilarityPair): string {
  const raw = pair.score ?? pair.similarity ?? 0;
  const pct = raw <= 1 ? raw * 100 : raw;
  return `${pct.toFixed(1)}%`;
}

function formatMatchedTokens(tokens: SimilarityPair["matchedTokens"]): string {
  if (Array.isArray(tokens)) return tokens.join(", ");
  if (typeof tokens === "number") return `${tokens} matched tokens`;
  return "—";
}

function summarizeEvent(event: ReplayEvent): string {
  const kind = eventTypeName(event);
  const payload = eventPayload(event);
  const q = eventQuestionIndex(event);
  const prefix = q !== undefined ? `Q${q + 1} · ` : "";
  if (kind === "mcq.selected" && typeof payload.choice === "number") {
    return `${prefix}Choice ${payload.choice + 1}`;
  }
  if (kind === "editor.paste" && typeof payload.charCount === "number") {
    return `${prefix}Paste ${payload.charCount} chars`;
  }
  if ((kind === "code.snapshot" || kind === "code.delta") && typeof payload.charCount === "number") {
    const lang = typeof payload.language === "string" ? `${payload.language} · ` : "";
    return `${prefix}${lang}${payload.charCount} chars`;
  }
  if (kind === "heartbeat.gap" && typeof payload.gapSeconds === "number") {
    return `Gap ${payload.gapSeconds}s`;
  }
  if (kind === "nav.question" && payload.review === true) {
    return "Review screen";
  }
  return prefix ? prefix.slice(0, -3) : "Recorded";
}

function computeLineDiff(before: string, after: string): DiffLine[] {
  const a = before ? before.split("\n") : [];
  const b = after ? after.split("\n") : [];
  if (a.length * b.length > 40_000) {
    return [
      ...a.map((text): DiffLine => ({ kind: "remove", text })),
      ...b.map((text): DiffLine => ({ kind: "add", text })),
    ];
  }
  const rows = a.length + 1;
  const cols = b.length + 1;
  const dp = Array.from({ length: rows }, () => new Uint16Array(cols));
  for (let i = a.length - 1; i >= 0; i -= 1) {
    for (let j = b.length - 1; j >= 0; j -= 1) {
      dp[i][j] = a[i] === b[j]
        ? dp[i + 1][j + 1] + 1
        : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }
  const result: DiffLine[] = [];
  let i = 0;
  let j = 0;
  while (i < a.length && j < b.length) {
    if (a[i] === b[j]) {
      result.push({ kind: "same", text: a[i] });
      i += 1;
      j += 1;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      result.push({ kind: "remove", text: a[i] });
      i += 1;
    } else {
      result.push({ kind: "add", text: b[j] });
      j += 1;
    }
  }
  while (i < a.length) {
    result.push({ kind: "remove", text: a[i] });
    i += 1;
  }
  while (j < b.length) {
    result.push({ kind: "add", text: b[j] });
    j += 1;
  }
  return result;
}

export function ContestIntegrityReview({ contestId }: { contestId: string }) {
  const [overview, setOverview] = useState<IntegrityOverviewResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const [reviewerId, setReviewerId] = useState("");
  const [selectedAttemptId, setSelectedAttemptId] = useState<string | null>(null);
  const [replay, setReplay] = useState<ReplayResponse | null>(null);
  const [replayLoading, setReplayLoading] = useState(false);
  const [cursor, setCursor] = useState(0);

  const [decisionStatus, setDecisionStatus] = useState<"under_review" | "cleared" | "confirmed_violation">("under_review");
  const [decisionReason, setDecisionReason] = useState("");

  const [appealDrafts, setAppealDrafts] = useState<Record<string, { status: "in_review" | "resolved" | "dismissed"; resolution: string }>>({});

  const loadOverview = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api<IntegrityOverviewResponse>(`/staff/contests/${contestId}/integrity`);
      setOverview(data);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load integrity review.");
    } finally {
      setLoading(false);
    }
  }, [contestId]);

  useEffect(() => {
    let active = true;
    void api<IntegrityOverviewResponse>(`/staff/contests/${contestId}/integrity`)
      .then((data) => {
        if (!active) return;
        setOverview(data);
        setError("");
        setLoading(false);
      })
      .catch((e: Error) => {
        if (!active) return;
        setError(e.message || "Failed to load integrity review.");
        setLoading(false);
      });
    return () => { active = false; };
  }, [contestId]);

  const openReplay = useCallback(async (attemptId: string) => {
    setSelectedAttemptId(attemptId);
    setReplayLoading(true);
    setError("");
    try {
      const data = await api<ReplayResponse>(`/staff/contests/${contestId}/attempts/${attemptId}/replay`);
      setReplay(data);
      setCursor(Math.max(0, (data.events?.length ?? 1) - 1));
      const summary = overview?.reviews.find((r) => r.attemptId === attemptId);
      const currentStatus = data.review?.status ?? data.status ?? summary?.status;
      if (currentStatus === "cleared" || currentStatus === "confirmed_violation" || currentStatus === "under_review") {
        setDecisionStatus(currentStatus);
      } else {
        setDecisionStatus("under_review");
      }
      setDecisionReason(data.review?.reason ?? data.reason ?? summary?.reason ?? "");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load attempt replay.");
      setReplay(null);
    } finally {
      setReplayLoading(false);
    }
  }, [contestId, overview]);

  async function runSimilarityCheck() {
    if (busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await api(`/staff/contests/${contestId}/similarity`, "POST");
      setNotice("Similarity check completed.");
      await loadOverview();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Similarity check failed.");
    } finally {
      setBusy(false);
    }
  }

  async function assignReviewer(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = reviewerId.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await api(`/staff/contests/${contestId}/reviewers`, "POST", { reviewerId: trimmed });
      setReviewerId("");
      setNotice("Reviewer assigned.");
      await loadOverview();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to assign reviewer.");
    } finally {
      setBusy(false);
    }
  }

  async function submitReviewDecision(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedAttemptId || !decisionReason.trim() || busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await api(`/staff/contests/${contestId}/attempts/${selectedAttemptId}/review`, "POST", {
        status: decisionStatus,
        reason: decisionReason.trim(),
      });
      setNotice("Review decision saved.");
      await loadOverview();
      await openReplay(selectedAttemptId);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save review decision.");
    } finally {
      setBusy(false);
    }
  }

  async function resolveAppeal(appealId: string, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const draft = appealDrafts[appealId] ?? { status: "resolved", resolution: "" };
    if (!draft.resolution.trim() || busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await api(`/staff/contests/${contestId}/appeals/${appealId}/resolve`, "POST", {
        status: draft.status,
        resolution: draft.resolution.trim(),
      });
      setNotice("Appeal updated.");
      await loadOverview();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to update appeal.");
    } finally {
      setBusy(false);
    }
  }

  const timeline = useMemo(() => replay?.events ?? [], [replay]);
  const clampedCursor = Math.min(cursor, Math.max(0, timeline.length - 1));
  const activeEvent = timeline[clampedCursor];

  const snapshotAtCursor = useMemo(() => {
    const codeState = new Map<number, { current: string; previous: string; language?: string }>();
    const mcqState = new Map<number, number>();
    let lastQuestionIndex: number | undefined;

    for (let idx = 0; idx <= clampedCursor && idx < timeline.length; idx += 1) {
      const ev = timeline[idx];
      const payload = eventPayload(ev);
      const qIdx = eventQuestionIndex(ev);
      if (qIdx !== undefined) lastQuestionIndex = qIdx;

      if (eventTypeName(ev) === "mcq.selected" && qIdx !== undefined && typeof payload.choice === "number") {
        mcqState.set(qIdx, payload.choice);
      }

      const preview =
        typeof payload.preview === "string"
          ? payload.preview
          : typeof payload.source === "string"
            ? payload.source
            : undefined;
      if (qIdx !== undefined && preview !== undefined) {
        const prev = codeState.get(qIdx);
        codeState.set(qIdx, {
          previous: prev?.current ?? "",
          current: preview,
          language: typeof payload.language === "string" ? payload.language : prev?.language,
        });
      }
    }

    const focusQuestion =
      (activeEvent ? eventQuestionIndex(activeEvent) : undefined) ?? lastQuestionIndex;
    const code = focusQuestion !== undefined ? codeState.get(focusQuestion) : undefined;
    const mcqChoice = focusQuestion !== undefined ? mcqState.get(focusQuestion) : undefined;
    const diff = code ? computeLineDiff(code.previous, code.current) : [];

    return { focusQuestion, code, mcqChoice, diff };
  }, [activeEvent, clampedCursor, timeline]);

  const attemptSimilarityMatches = useMemo(() => {
    if (!selectedAttemptId) return [];
    if (replay?.similarityMatches?.length) return replay.similarityMatches;
    if (replay?.similarityPairs?.length) return replay.similarityPairs;
    return (overview?.similarityPairs ?? []).filter((pair) => {
      const a = pair.attemptAId ?? pair.attemptA;
      const b = pair.attemptBId ?? pair.attemptB;
      return a === selectedAttemptId || b === selectedAttemptId;
    });
  }, [overview, replay, selectedAttemptId]);

  return (
    <section className="contest-integrity-panel mt-4" aria-label="Integrity and replay review">
      <div className="contest-integrity-header">
        <div>
          <h3>Integrity &amp; Replay</h3>
          <p className="muted">
            Automated signals queue human review only. Review full timelines before taking action.
          </p>
        </div>
        <div className="inline-actions">
          <button type="button" className="btn btn-secondary" disabled={busy || loading} onClick={() => void loadOverview()}>
            Refresh
          </button>
          <button type="button" className="btn btn-primary" disabled={busy || loading} onClick={() => void runSimilarityCheck()}>
            Run similarity check
          </button>
        </div>
      </div>

      {error && <p role="alert">{error}</p>}
      {notice && <p role="status">{notice}</p>}
      {loading && !overview && <p role="status">Loading integrity review…</p>}

      {overview && (
        <div className="contest-integrity-sections">
          <section className="contest-integrity-card" aria-label="Assigned reviewers">
            <h4>Assigned reviewers</h4>
            {overview.reviewers.length === 0 ? (
              <p className="muted">No additional reviewers assigned yet.</p>
            ) : (
              <ul className="contest-pill-list">
                {overview.reviewers.map((reviewer, idx) => {
                  const id = reviewer.reviewerId ?? reviewer.userId ?? reviewer.id ?? String(idx);
                  const label = reviewer.reviewerName ?? reviewer.name;
                  return (
                    <li key={id} className="contest-badge">
                      {label ? `${label} (${id})` : id}
                    </li>
                  );
                })}
              </ul>
            )}
            <form className="contest-inline-form" onSubmit={(e) => { void assignReviewer(e); }}>
              <label>
                Reviewer user ID
                <input
                  required
                  value={reviewerId}
                  disabled={busy}
                  onChange={(e) => setReviewerId(e.target.value)}
                  placeholder="Enter staff reviewer ID"
                />
              </label>
              <button type="submit" className="btn btn-secondary" disabled={busy || !reviewerId.trim()}>
                Assign reviewer
              </button>
            </form>
          </section>

          <section className="contest-integrity-card" aria-label="Attempt review queue">
            <h4>Attempts ({overview.reviews.length})</h4>
            {overview.reviews.length === 0 ? (
              <p className="muted">No attempts recorded for review yet.</p>
            ) : (
              <ul className="contest-review-list">
                {overview.reviews.map((item) => {
                  const sim = item.similarityMax ?? item.maxSimilarity;
                  const signals = item.signals ?? [];
                  return (
                    <li key={item.attemptId} className="contest-review-row">
                      <div>
                        <strong>{item.studentName || item.studentId}</strong>{" "}
                        <span className="contest-badge">{reviewDecisionLabels[item.status] ?? item.status}</span>
                        <p className="muted">
                          Attempt {item.attemptId}
                          {signals.length > 0 && ` · ${signals.length} signals`}
                          {typeof item.signalCount === "number" && signals.length === 0 && ` · ${item.signalCount} signals`}
                          {typeof sim === "number" && sim > 0 && ` · Max similarity ${(sim <= 1 ? sim * 100 : sim).toFixed(1)}%`}
                          {typeof item.openAppeals === "number" && item.openAppeals > 0 && ` · ${item.openAppeals} open appeals`}
                        </p>
                        {signals.length > 0 && (
                          <ul className="contest-pill-list">
                            {signals.map((sig, idx) => (
                              <li key={`${sig.kind}-${idx}`} className="contest-badge">
                                {sig.summary || sig.kind}
                              </li>
                            ))}
                          </ul>
                        )}
                        {item.reason && <p className="contest-prompt">{item.reason}</p>}
                      </div>
                      <button
                        type="button"
                        className="btn btn-secondary"
                        disabled={replayLoading}
                        onClick={() => void openReplay(item.attemptId)}
                      >
                        {selectedAttemptId === item.attemptId ? "Reload replay" : "Open replay"}
                      </button>
                    </li>
                  );
                })}
              </ul>
            )}
          </section>

          {selectedAttemptId && (
            <section className="contest-replay-panel" aria-label="Attempt replay">
              <div className="contest-integrity-header">
                <div>
                  <h4>
                    Attempt Replay ·{" "}
                    {replay?.review?.studentName ||
                      replay?.studentName ||
                      replay?.review?.studentId ||
                      replay?.studentId ||
                      selectedAttemptId}
                  </h4>
                  <p className="muted">Attempt ID: {selectedAttemptId}</p>
                </div>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => {
                    setSelectedAttemptId(null);
                    setReplay(null);
                  }}
                >
                  Close replay
                </button>
              </div>

              {replayLoading && <p role="status">Loading replay timeline…</p>}

              {replay && (
                <>
                  {!!replay.missingIntervals?.length && (
                    <div className="contest-missing-banner" role="alert">
                      <strong>Missing telemetry intervals detected</strong>
                      <ul>
                        {replay.missingIntervals.map((interval, idx) => (
                          <li key={idx}>{formatMissingInterval(interval)}</li>
                        ))}
                      </ul>
                    </div>
                  )}

                  {timeline.length === 0 ? (
                    <p className="muted">No timeline events recorded for this attempt.</p>
                  ) : (
                    <div className="contest-replay-grid">
                      <div className="contest-timeline-column">
                        <div className="contest-scrubber">
                          <label htmlFor="contest-replay-scrubber">
                            Timeline scrubber ({clampedCursor + 1} / {timeline.length})
                          </label>
                          <input
                            id="contest-replay-scrubber"
                            type="range"
                            min={0}
                            max={Math.max(0, timeline.length - 1)}
                            value={clampedCursor}
                            onChange={(e) => setCursor(Number(e.target.value))}
                          />
                          <div className="inline-actions">
                            <button
                              type="button"
                              className="btn btn-secondary"
                              disabled={clampedCursor <= 0}
                              onClick={() => setCursor((c) => Math.max(0, c - 1))}
                            >
                              Previous
                            </button>
                            <button
                              type="button"
                              className="btn btn-secondary"
                              disabled={clampedCursor >= timeline.length - 1}
                              onClick={() => setCursor((c) => Math.min(timeline.length - 1, c + 1))}
                            >
                              Next
                            </button>
                          </div>
                        </div>

                        <ol className="contest-step-list" aria-label="Replay events">
                          {timeline.map((ev, index) => {
                            const authoritative = isServerEvent(ev);
                            const ts = eventTimestamp(ev);
                            const evName = eventTypeName(ev);
                            return (
                              <li key={ev.id ?? `${evName}-${ev.seq ?? index}-${index}`}>
                                <button
                                  type="button"
                                  className="contest-step-btn"
                                  aria-current={index === clampedCursor ? "step" : undefined}
                                  onClick={() => setCursor(index)}
                                >
                                  <span className={authoritative ? "contest-trust-server" : "contest-trust-client"}>
                                    {authoritative ? "[SERVER]" : "[CLIENT]"}
                                  </span>
                                  <span className="contest-step-type">{evName}</span>
                                  <span className="muted">{summarizeEvent(ev)}</span>
                                  {ev.seq !== undefined && ev.seq > 0 && <span className="muted">seq {ev.seq}</span>}
                                  {ev.late && <span className="contest-badge">Late</span>}
                                  {ts && <span className="muted">{contestTime(ts)}</span>}
                                </button>
                              </li>
                            );
                          })}
                        </ol>
                      </div>

                      <div className="contest-snapshot-column">
                        {activeEvent && (
                          <div className="contest-snapshot-card">
                            <h5>
                              {isServerEvent(activeEvent) ? "[SERVER] Authoritative event" : "[CLIENT] Observed event"} · {eventTypeName(activeEvent)}
                            </h5>
                            <p className="muted">
                              {eventTimestamp(activeEvent) ? contestTime(eventTimestamp(activeEvent)) : "Timestamp unavailable"}
                              {activeEvent.seq !== undefined && activeEvent.seq > 0 && ` · seq ${activeEvent.seq}`}
                            </p>
                            {snapshotAtCursor.focusQuestion !== undefined && (
                              <p>Question {snapshotAtCursor.focusQuestion + 1}</p>
                            )}
                            {snapshotAtCursor.mcqChoice !== undefined && (
                              <p>MCQ selection at step: Choice {snapshotAtCursor.mcqChoice + 1}</p>
                            )}
                            {snapshotAtCursor.code ? (
                              <>
                                <h6>
                                  Code snapshot &amp; diff
                                  {snapshotAtCursor.code.language ? ` (${snapshotAtCursor.code.language})` : ""}
                                </h6>
                                <div className="contest-diff-view" role="region" aria-label="Code diff at scrubber position">
                                  {snapshotAtCursor.diff.map((line, i) => (
                                    <div key={i} className={`contest-diff-line contest-diff-${line.kind}`}>
                                      <span>{line.kind === "add" ? "+" : line.kind === "remove" ? "-" : " "}</span>
                                      <code>{line.text || " "}</code>
                                    </div>
                                  ))}
                                </div>
                              </>
                            ) : (
                              <pre className="contest-payload-preview">
                                {JSON.stringify(eventPayload(activeEvent), null, 2)}
                              </pre>
                            )}
                          </div>
                        )}

                        <div className="contest-snapshot-card">
                          <h5>Similarity matches ({attemptSimilarityMatches.length})</h5>
                          {attemptSimilarityMatches.length === 0 ? (
                            <p className="muted">No similarity matches flagged for this attempt.</p>
                          ) : (
                            <ul className="contest-similarity-list">
                              {attemptSimilarityMatches.map((pair, idx) => (
                                <li key={pair.id ?? idx} className="contest-similarity-item">
                                  <p>
                                    <strong>Attempt {pair.attemptAId ?? pair.attemptA}</strong> vs{" "}
                                    <strong>Attempt {pair.attemptBId ?? pair.attemptB}</strong> ·{" "}
                                    <span className="contest-badge">{formatSimilarityPercent(pair)}</span>
                                    {typeof pair.questionIndex === "number" && ` · Q${pair.questionIndex + 1}`}
                                  </p>
                                  <p className="muted">Matched tokens: {formatMatchedTokens(pair.matchedTokens)}</p>
                                  <SimilaritySnippets pair={pair} />
                                </li>
                              ))}
                            </ul>
                          )}
                        </div>

                        <form className="portal-form contest-snapshot-card" onSubmit={(e) => { void submitReviewDecision(e); }}>
                          <h5>Record review decision</h5>
                          <label>
                            Decision status
                            <select
                              value={decisionStatus}
                              disabled={busy}
                              onChange={(e) => setDecisionStatus(e.target.value as "under_review" | "cleared" | "confirmed_violation")}
                            >
                              <option value="under_review">Under review</option>
                              <option value="cleared">Cleared</option>
                              <option value="confirmed_violation">Confirmed violation</option>
                            </select>
                          </label>
                          <label>
                            Reason (required)
                            <textarea
                              required
                              rows={3}
                              maxLength={2000}
                              disabled={busy}
                              value={decisionReason}
                              onChange={(e) => setDecisionReason(e.target.value)}
                              placeholder="Summarize supporting timeline or similarity evidence."
                            />
                          </label>
                          <button type="submit" className="btn btn-primary" disabled={busy || !decisionReason.trim()}>
                            Save decision
                          </button>
                        </form>
                      </div>
                    </div>
                  )}
                </>
              )}
            </section>
          )}

          <section className="contest-integrity-card" aria-label="Similarity pairs">
            <h4>Similarity pairs ({overview.similarityPairs.length})</h4>
            {overview.similarityPairs.length === 0 ? (
              <p className="muted">No similarity pairs recorded. Run similarity check after submissions arrive.</p>
            ) : (
              <ul className="contest-similarity-list">
                {overview.similarityPairs.map((pair, idx) => {
                  const aId = pair.attemptAId ?? pair.attemptA ?? "";
                  const bId = pair.attemptBId ?? pair.attemptB ?? "";
                  return (
                    <li key={pair.id ?? idx} className="contest-similarity-item">
                      <div className="contest-integrity-header">
                        <p>
                          <strong>Attempt {aId}</strong>
                          {pair.studentAName ? ` (${pair.studentAName})` : ""} vs{" "}
                          <strong>Attempt {bId}</strong>
                          {pair.studentBName ? ` (${pair.studentBName})` : ""} ·{" "}
                          <span className="contest-badge">{formatSimilarityPercent(pair)}</span>
                          {typeof pair.questionIndex === "number" && ` · Q${pair.questionIndex + 1}`}
                        </p>
                        <div className="inline-actions">
                          {aId && (
                            <button type="button" className="btn btn-secondary" onClick={() => void openReplay(aId)}>
                              Replay A
                            </button>
                          )}
                          {bId && (
                            <button type="button" className="btn btn-secondary" onClick={() => void openReplay(bId)}>
                              Replay B
                            </button>
                          )}
                        </div>
                      </div>
                      <p className="muted">Matched tokens: {formatMatchedTokens(pair.matchedTokens)}</p>
                      <SimilaritySnippets pair={pair} />
                    </li>
                  );
                })}
              </ul>
            )}
          </section>

          <section className="contest-integrity-card" aria-label="Student concerns and appeals">
            <h4>Concerns &amp; Appeals ({overview.appeals.length})</h4>
            {overview.appeals.length === 0 ? (
              <p className="muted">No student concerns or appeals submitted.</p>
            ) : (
              <ul className="contest-appeals-list">
                {overview.appeals.map((appeal) => {
                  const draft = appealDrafts[appeal.id] ?? {
                    status: appeal.status === "dismissed" ? "dismissed" : appeal.status === "resolved" ? "resolved" : "in_review",
                    resolution: appeal.resolution ?? "",
                  };
                  return (
                    <li key={appeal.id} className="contest-appeal-item">
                      <div className="contest-appeal-meta">
                        <strong>{appeal.studentName || appeal.studentId}</strong>
                        <span className="contest-badge">{appeal.kind === "appeal" ? "Appeal" : "Concern"}</span>
                        <span className="contest-badge">{appealStatusLabels[appeal.status] ?? appeal.status}</span>
                        {appeal.attemptId && <span className="muted">Attempt {appeal.attemptId}</span>}
                        {appeal.createdAt && <span className="muted">{contestTime(appeal.createdAt)}</span>}
                      </div>
                      <p className="contest-prompt">{appeal.message}</p>
                      {appeal.resolution && (
                        <p className="contest-appeal-resolution">
                          <strong>Current resolution:</strong> {appeal.resolution}
                        </p>
                      )}
                      <form className="portal-form mt-2" onSubmit={(e) => { void resolveAppeal(appeal.id, e); }}>
                        <label>
                          Appeal status
                          <select
                            value={draft.status}
                            disabled={busy}
                            onChange={(e) =>
                              setAppealDrafts((prev) => ({
                                ...prev,
                                [appeal.id]: {
                                  ...draft,
                                  status: e.target.value as "in_review" | "resolved" | "dismissed",
                                },
                              }))
                            }
                          >
                            <option value="in_review">In review</option>
                            <option value="resolved">Resolved</option>
                            <option value="dismissed">Dismissed</option>
                          </select>
                        </label>
                        <label>
                          Resolution (required)
                          <textarea
                            required
                            rows={2}
                            maxLength={2000}
                            disabled={busy}
                            value={draft.resolution}
                            onChange={(e) =>
                              setAppealDrafts((prev) => ({
                                ...prev,
                                [appeal.id]: { ...draft, resolution: e.target.value },
                              }))
                            }
                            placeholder="Explain the review outcome to the student."
                          />
                        </label>
                        <button type="submit" className="btn btn-secondary" disabled={busy || !draft.resolution.trim()}>
                          Update appeal
                        </button>
                      </form>
                    </li>
                  );
                })}
              </ul>
            )}
          </section>

          <details className="contest-integrity-card">
            <summary>Evidence access audit log ({overview.accessLog.length})</summary>
            {overview.accessLog.length === 0 ? (
              <p className="muted mt-2">No evidence access events recorded yet.</p>
            ) : (
              <ul className="contest-review-list mt-2">
                {overview.accessLog.map((entry, idx) => {
                  const ts = entry.occurredAt ?? entry.createdAt;
                  return (
                    <li key={entry.id ?? idx}>
                      <span>{entry.actorName || entry.actorId || entry.userId || "Staff"}</span> ·{" "}
                      <span>{entry.action}</span>
                      {entry.detail && <span className="muted"> · {entry.detail}</span>}
                      {entry.attemptId && <span className="muted"> · Attempt {entry.attemptId}</span>}
                      {ts && <span className="muted"> · {contestTime(ts)}</span>}
                    </li>
                  );
                })}
              </ul>
            )}
          </details>
        </div>
      )}
    </section>
  );
}

function SimilaritySnippets({ pair }: { pair: SimilarityPair }) {
  const preview = pair.normalizedPreview ?? pair.snippetPreview;
  if (!preview && !pair.snippetA && !pair.snippetB) return null;
  return (
    <>
      {preview && <pre className="contest-payload-preview">{preview}</pre>}
      {!preview && pair.snippetA && (
        <pre className="contest-payload-preview">{`Attempt A:\n${pair.snippetA}`}</pre>
      )}
      {!preview && pair.snippetB && (
        <pre className="contest-payload-preview">{`Attempt B:\n${pair.snippetB}`}</pre>
      )}
    </>
  );
}
