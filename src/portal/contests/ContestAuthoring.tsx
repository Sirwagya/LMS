import { lazy, Suspense, useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { api } from "../api";
import { ContestIntegrityReview } from "./ContestIntegrityReview";
import { ContestQuestionEditor } from "./ContestQuestions";
import type { LeaderboardResponse } from "./ContestResults";
import { RosterPicker } from "./RosterPicker";

const LazyLeaderboardTable = lazy(() =>
  import("./ContestResults").then((m) => ({ default: m.LeaderboardTable })),
);
import { contestTime, newQuestion, type ContestDraft, type ContestPage, type ContestRecord } from "./types";
import { useContestFeatures } from "./useContestsEnabled";

type Editor = { draft: ContestDraft; record?: ContestRecord };

type ResultRelease = {
  version: number;
  reason?: string;
  publishedAt: string;
  publishedBy?: string;
  publishedByName?: string;
  releaseExplanations?: boolean;
  withheldCount?: number;
  withholdAttemptIds?: string[];
};

type StaffResultAttempt = {
  attemptId: string;
  studentId?: string;
  studentName?: string;
  displayName?: string;
  status?: string;
  withheld?: boolean;
};

type StaffResultsResponse = {
  assessmentId: string;
  latestVersion: number;
  releaseExplanations: boolean;
  releases: ResultRelease[];
  leaderboard: LeaderboardResponse;
  attempts?: StaffResultAttempt[];
};

function parseAttemptIds(raw: string): string[] {
  return Array.from(
    new Set(
      raw
        .split(/[\s,]+/)
        .map((value) => value.trim())
        .filter(Boolean),
    ),
  );
}

function localTime(value: string) {
  if (!value) return "";
  const time = new Date(value);
  return new Date(time.getTime() - time.getTimezoneOffset() * 60_000).toISOString().slice(0, 19);
}

function editorFor(record?: ContestRecord): Editor {
  return {
    record,
    draft: record
      ? { ...record, opensAt: localTime(record.opensAt), closesAt: localTime(record.closesAt) }
      : { title: "", instructions: "", opensAt: "", closesAt: "", durationMinutes: 60, studentIds: [], questions: [] },
  };
}

export function ContestAuthoring() {
  const [editor, setEditor] = useState<Editor | null>(null);
  const [page, setPage] = useState<ContestPage | null>(null);
  const [index, setIndex] = useState(0);
  const [version, setVersion] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [reason, setReason] = useState("");
  const [preview, setPreview] = useState(false);
  const [integrityOpen, setIntegrityOpen] = useState(false);
  const [resultsOpen, setResultsOpen] = useState(false);
  const { integrityEnabled, resultsEnabled } = useContestFeatures();

  useEffect(() => {
    let current = true;
    void api<ContestPage>(`/staff/contests?page=${index}`)
      .then((result) => { if (current) setPage(result); })
      .catch((e: Error) => { if (current) setError(e.message); });
    return () => { current = false; };
  }, [index, version]);

  async function perform(operation: () => Promise<void>) {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await operation();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Contest request failed.");
    } finally {
      setBusy(false);
    }
  }

  function patch(values: Partial<ContestDraft>) {
    setEditor((current) => current && { ...current, draft: { ...current.draft, ...values } });
  }

  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!editor || busy) return;
    await perform(async () => {
      const { record, draft } = editor;
      const saved = await api<ContestRecord>(record ? `/staff/contests/${record.id}` : "/staff/contests",
        record ? "PUT" : "POST", {
          revision: record?.revision,
          draft: { ...draft, opensAt: new Date(draft.opensAt).toISOString(), closesAt: new Date(draft.closesAt).toISOString() },
        });
      setEditor(editorFor(saved));
      setMessage("Draft saved.");
      setVersion((value) => value + 1);
    });
  }

  async function transition(action: "publish" | "cancel", record: ContestRecord) {
    if (busy) return;
    await perform(async () => {
      const path = `/staff/contests/${record.id}`;
      await api(`${path}/${action}`, "POST", { revision: record.revision, reason });
      setVersion((value) => value + 1);
      setEditor(null);
      setIntegrityOpen(false);
      setResultsOpen(false);
      setReason("");
      setMessage(action === "publish" ? "Contest published." : "Contest cancelled.");
    });
  }

  const draft = editor?.draft;
  const record = editor?.record;
  const dirty = editor && JSON.stringify(editor.draft) !== JSON.stringify(editorFor(record).draft);
  const readOnly = record !== undefined && record.status !== "draft";

  return (
    <section className="screen">
      <div className="panel panel-padding">
        <div className="inline-actions">
          <button className="btn btn-primary" type="button" disabled={busy} onClick={() => {
            if (dirty && !window.confirm("Discard unsaved contest edits?")) return;
            setEditor(editorFor());
            setPreview(false);
            setIntegrityOpen(false);
            setResultsOpen(false);
            setError(""); setMessage(""); setReason("");
          }}>New contest</button>
        </div>
        {error && <p role="alert" className="mt-3">{error}</p>}
        {message && <p role="status" className="mt-3">{message}</p>}
        {!page && !error && <p role="status">Loading contests…</p>}
        {page?.items.length === 0 && <p className="muted mt-3">No contests on this page.</p>}
        <ul className="space-y-2 mt-3">
          {page?.items.map((contest) => (
            <li key={contest.id}>
              <button className="text-link" type="button" disabled={busy} onClick={() => {
                if (dirty && !window.confirm("Discard unsaved contest edits?")) return;
                void perform(async () => {
                  const detail = await api<ContestRecord>(`/staff/contests/${contest.id}`);
                  setEditor(editorFor(detail));
                  setPreview(false);
                  setIntegrityOpen(false);
                  setResultsOpen(false);
                  setReason("");
                });
              }}>{contest.title}</button>
              <p className="muted">{contest.status} · {contestTime(contest.opensAt)}</p>
            </li>
          ))}
        </ul>
        {(index > 0 || page?.hasMore) && (
          <div className="inline-actions mt-4">
            <button type="button" className="btn btn-secondary" disabled={index === 0} onClick={() => { setPage(null); setIndex(index - 1); }}>Previous contests</button>
            <button type="button" className="btn btn-secondary" disabled={!page?.hasMore || index >= 10000} onClick={() => { setPage(null); setIndex(index + 1); }}>Next contests</button>
          </div>
        )}
      </div>

      {draft && (
        <section className="panel panel-padding">
          <h2>{record?.title || "New contest"}</h2>
          {record && (
            <p className="muted">{record.status} · Revision {record.revision}
              {record.cancellationReason && <> · {record.cancellationReason}</>}
            </p>
          )}
          <div className="inline-actions mt-3">
            <button className="btn btn-secondary" type="button" aria-expanded={preview}
              onClick={() => setPreview(!preview)}>{preview ? "Hide student preview" : "Preview student view"}</button>
            {record && (record.status === "published" || record.status === "cancelled") && integrityEnabled && (
              <button
                className="btn btn-secondary"
                type="button"
                aria-expanded={integrityOpen}
                onClick={() => setIntegrityOpen((open) => !open)}
              >
                {integrityOpen ? "Hide integrity & replay" : "Integrity & Replay"}
              </button>
            )}
            {record && record.status === "published" && resultsEnabled && (
              <button
                className="btn btn-secondary"
                type="button"
                aria-expanded={resultsOpen}
                onClick={() => setResultsOpen((open) => !open)}
              >
                {resultsOpen ? "Hide results & leaderboard" : "Results & Leaderboard"}
              </button>
            )}
          </div>
          {integrityOpen && record && (record.status === "published" || record.status === "cancelled") && integrityEnabled && (
            <ContestIntegrityReview key={record.id} contestId={record.id} />
          )}
          {resultsOpen && record && record.status === "published" && resultsEnabled && (
            <StaffContestResults key={record.id} contestId={record.id} />
          )}
          {preview && (
            <section className="mt-4 space-y-3" aria-label="Student preview">
              <p className="whitespace-pre-wrap">{draft.instructions}</p>
              {draft.questions.length === 0 && <p className="muted">Add questions to preview them here.</p>}
              <ol className="space-y-4">
                {draft.questions.map((question, i) => (
                  <li key={i}>
                    <h3>Question {i + 1} · {question.points} points</h3>
                    <p className="whitespace-pre-wrap">{question.prompt}</p>
                    {question.kind === "mcq" ? (
                      <ul className="list-disc pl-5">{question.choices.map((choice, index) => <li key={index}>{choice}</li>)}</ul>
                    ) : question.testCases.filter((test) => !test.hidden).map((test, index) => (
                      <div key={index}>
                        <p>Example {index + 1}</p>
                        <p>Input</p><pre className="whitespace-pre-wrap">{test.input}</pre>
                        <p>Expected output</p><pre className="whitespace-pre-wrap">{test.expected}</pre>
                      </div>
                    ))}
                  </li>
                ))}
              </ol>
            </section>
          )}
          <form className="portal-form" onSubmit={(e) => { void save(e); }}>
            <fieldset className="portal-form" disabled={busy || readOnly}>
              <label>
                Title
                <input required maxLength={240} value={draft.title} onChange={(e) => patch({ title: e.target.value })} />
              </label>
              <label>
                Instructions and allowed resources
                <textarea rows={4} maxLength={8000} value={draft.instructions} onChange={(e) => patch({ instructions: e.target.value })} />
              </label>
              <p className="muted">Times shown in {Intl.DateTimeFormat().resolvedOptions().timeZone}.</p>
              <label>Opens
                <input type="datetime-local" step={1} required value={draft.opensAt} onChange={(e) => patch({ opensAt: e.target.value })} />
              </label>
              <label>Closes
                <input type="datetime-local" step={1} required value={draft.closesAt} onChange={(e) => patch({ closesAt: e.target.value })} />
              </label>
              <label>Attempt duration (minutes)
                <input type="number" required min={1} max={720} value={draft.durationMinutes} onChange={(e) => patch({ durationMinutes: Number(e.target.value) })} />
              </label>
              {readOnly ? <p>{draft.studentIds.length} eligible students</p> : (
                <RosterPicker studentIds={draft.studentIds} onChange={(studentIds) => patch({ studentIds })} />
              )}
              {draft.questions.map((question, index) => (
                <fieldset className="portal-form" key={index}>
                  <legend>Question {index + 1}</legend>
                  <ContestQuestionEditor question={question} onChange={(value) => patch({ questions: draft.questions.map((current, i) => i === index ? value : current) })} />
                  {!readOnly && <button type="button" className="text-link" onClick={() => patch({ questions: draft.questions.filter((_, i) => i !== index) })}>Remove question {index + 1}</button>}
                </fieldset>
              ))}
              {!readOnly && (
                <div className="inline-actions">
                  <button type="button" className="btn btn-secondary" disabled={draft.questions.length >= 50} onClick={() => patch({ questions: [...draft.questions, newQuestion("mcq")] })}>Add MCQ</button>
                  <button type="button" className="btn btn-secondary" disabled={draft.questions.length >= 50} onClick={() => patch({ questions: [...draft.questions, newQuestion("coding")] })}>Add coding question</button>
                  <button type="submit" className="btn btn-primary">Save draft</button>
                </div>
              )}
            </fieldset>
          </form>
          {record?.status === "draft" && (
            <div className="mt-4">
              {dirty && <p className="muted">Save changes before publishing.</p>}
              <button className="btn btn-secondary" type="button" disabled={busy || !!dirty || draft.questions.length === 0 || draft.studentIds.length === 0} onClick={() => {
                if (window.confirm("Publish this contest? The saved questions and roster will be locked.")) void transition("publish", record);
              }}>Publish contest</button>
            </div>
          )}
          {record && record.status !== "cancelled" && (
            <form className="portal-form mt-4" onSubmit={(e) => {
              e.preventDefault();
              if (window.confirm("Cancel this contest?")) void transition("cancel", record);
            }}>
              <label>Cancellation reason
                <input required maxLength={1000} value={reason} onChange={(e) => setReason(e.target.value)} />
              </label>
              <button className="btn btn-secondary" disabled={busy || !reason.trim()}>Cancel contest</button>
            </form>
          )}
        </section>
      )}
    </section>
  );
}

function StaffContestResults({ contestId }: { contestId: string }) {
  const [data, setData] = useState<StaffResultsResponse | null>(null);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [releaseExplanations, setReleaseExplanations] = useState(false);
  const [withholdIdsText, setWithholdIdsText] = useState("");
  const [reason, setReason] = useState("");
  const formInitializedRef = useRef(false);

  const applyResultDefaults = useCallback((result: StaffResultsResponse, forceSync: boolean) => {
    if (!forceSync && formInitializedRef.current) return;
    setReleaseExplanations(Boolean(result.releaseExplanations));
    const latestRelease = result.releases?.[0];
    if (latestRelease?.withholdAttemptIds?.length) {
      setWithholdIdsText(latestRelease.withholdAttemptIds.join("\n"));
    } else {
      const withheldFromAttempts = (result.attempts ?? [])
        .filter((attempt) => attempt.withheld)
        .map((attempt) => attempt.attemptId);
      if (withheldFromAttempts.length > 0) {
        setWithholdIdsText(withheldFromAttempts.join("\n"));
      }
    }
    formInitializedRef.current = true;
  }, []);

  const loadResults = useCallback(
    async (targetPage: number, syncDefaults: boolean) => {
      setLoading(true);
      setError("");
      try {
        const result = await api<StaffResultsResponse>(`/staff/contests/${contestId}/results?page=${targetPage}`);
        setData(result);
        applyResultDefaults(result, syncDefaults);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to load contest results.");
      } finally {
        setLoading(false);
      }
    },
    [applyResultDefaults, contestId],
  );

  useEffect(() => {
    let active = true;
    void api<StaffResultsResponse>(`/staff/contests/${contestId}/results?page=${page}`)
      .then((result) => {
        if (!active) return;
        setData(result);
        setError("");
        setLoading(false);
        applyResultDefaults(result, false);
      })
      .catch((e: Error) => {
        if (!active) return;
        setError(e.message || "Failed to load contest results.");
        setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [applyResultDefaults, contestId, page]);

  async function publishOrRegrade(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    const isRegrade = (data?.latestVersion ?? 0) > 0;
    const trimmedReason = reason.trim();
    if (isRegrade && !trimmedReason) {
      setError("A reason is required when regrading and republishing results.");
      return;
    }

    const withholdAttemptIds = parseAttemptIds(withholdIdsText);
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await api(`/staff/contests/${contestId}/results/publish`, "POST", {
        releaseExplanations,
        withholdAttemptIds,
        reason: trimmedReason,
      });
      setReason("");
      setNotice(isRegrade ? "Results regraded and republished." : "Results published.");
      await loadResults(page, true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to publish results.");
    } finally {
      setBusy(false);
    }
  }

  function toggleAttemptWithhold(attemptId: string) {
    const selected = new Set(parseAttemptIds(withholdIdsText));
    if (selected.has(attemptId)) selected.delete(attemptId);
    else selected.add(attemptId);
    setWithholdIdsText(Array.from(selected).join("\n"));
  }

  const latestVersion = data?.latestVersion ?? 0;
  const isRegrade = latestVersion > 0;
  const selectedWithheld = new Set(parseAttemptIds(withholdIdsText));
  const selectableAttempts: StaffResultAttempt[] =
    data?.attempts && data.attempts.length > 0
      ? data.attempts
      : (data?.leaderboard?.items ?? [])
          .filter((row): row is typeof row & { attemptId: string } => typeof row.attemptId === "string" && row.attemptId.length > 0)
          .map((row) => ({
            attemptId: row.attemptId,
            studentId: row.studentId,
            displayName: row.displayName,
            withheld: row.withheld,
          }));

  return (
    <section className="contest-integrity-panel mt-4" aria-label="Results and leaderboard management">
      <div className="contest-integrity-header">
        <div>
          <h3>Results &amp; Leaderboard</h3>
          <p className="muted">
            {latestVersion > 0
              ? `Published version ${latestVersion} · Explanations ${data?.releaseExplanations ? "released" : "hidden"}`
              : "Results have not been published to students yet."}
          </p>
        </div>
        <button
          type="button"
          className="btn btn-secondary"
          disabled={busy || loading}
          onClick={() => void loadResults(page, false)}
        >
          Refresh
        </button>
      </div>

      {error && <p role="alert">{error}</p>}
      {notice && <p role="status">{notice}</p>}
      {loading && !data && <p role="status">Loading results overview…</p>}

      {data && (
        <div className="contest-integrity-sections">
          <form className="portal-form contest-integrity-card" onSubmit={(e) => { void publishOrRegrade(e); }}>
            <h4>{isRegrade ? "Regrade & republish results" : "Publish results"}</h4>

            <label className="contest-roster-choice">
              <input
                type="checkbox"
                checked={releaseExplanations}
                disabled={busy}
                onChange={(e) => setReleaseExplanations(e.target.checked)}
              />
              <span>Release MCQ explanations to students</span>
            </label>

            {selectableAttempts.length > 0 && (
              <fieldset className="contest-withhold-picker" disabled={busy}>
                <legend>Quick-select attempts to withhold</legend>
                <div className="contest-withhold-list">
                  {selectableAttempts.map((attempt) => (
                    <label key={attempt.attemptId} className="contest-roster-choice">
                      <input
                        type="checkbox"
                        checked={selectedWithheld.has(attempt.attemptId)}
                        onChange={() => toggleAttemptWithhold(attempt.attemptId)}
                      />
                      <span>
                        {attempt.studentName || attempt.displayName || attempt.studentId || attempt.attemptId}{" "}
                        <span className="muted">({attempt.attemptId})</span>
                      </span>
                    </label>
                  ))}
                </div>
              </fieldset>
            )}

            <label>
              Withhold attempt IDs (comma or newline separated)
              <textarea
                rows={2}
                disabled={busy}
                value={withholdIdsText}
                onChange={(e) => setWithholdIdsText(e.target.value)}
                placeholder="Paste attempt UUIDs to keep under private review"
              />
            </label>

            <label>
              {isRegrade ? "Regrade reason (required)" : "Publication note (optional)"}
              <input
                required={isRegrade}
                maxLength={1000}
                disabled={busy}
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                placeholder={isRegrade ? "Explain why scores or withheld attempts are changing" : "Optional note for audit history"}
              />
            </label>

            <button
              type="submit"
              className="btn btn-primary"
              disabled={busy || (isRegrade && !reason.trim())}
            >
              {busy ? "Publishing…" : isRegrade ? "Regrade & publish results" : "Publish results"}
            </button>
          </form>

          <section className="contest-integrity-card" aria-label="Publication audit history">
            <h4>Release history ({data.releases.length})</h4>
            {data.releases.length === 0 ? (
              <p className="muted">No result publications recorded yet.</p>
            ) : (
              <ul className="contest-review-list">
                {data.releases.map((rel) => {
                  const withheldCount = rel.withheldCount ?? rel.withholdAttemptIds?.length ?? 0;
                  return (
                    <li key={rel.version} className="contest-appeal-item">
                      <div className="contest-appeal-meta">
                        <span className="contest-badge">v{rel.version}</span>
                        <span className="contest-badge">{withheldCount} withheld</span>
                        {typeof rel.releaseExplanations === "boolean" && (
                          <span className="contest-badge">
                            {rel.releaseExplanations ? "Explanations released" : "Explanations hidden"}
                          </span>
                        )}
                        <span className="muted">{contestTime(rel.publishedAt)}</span>
                      </div>
                      {rel.reason && <p className="contest-prompt">{rel.reason}</p>}
                    </li>
                  );
                })}
              </ul>
            )}
          </section>

          <section className="contest-integrity-card" aria-label="Staff cohort leaderboard">
            <h4>Cohort leaderboard</h4>
            <Suspense fallback={<p role="status">Loading leaderboard…</p>}>
              <LazyLeaderboardTable
                leaderboard={data.leaderboard}
                page={page}
                loading={loading}
                onPageChange={(next) => {
                  setLoading(true);
                  setPage(next);
                }}
              />
            </Suspense>
          </section>
        </div>
      )}
    </section>
  );
}
