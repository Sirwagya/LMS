import { useCallback, useEffect, useState, type ClipboardEvent, type FormEvent } from "react";
import { api, transport } from "../api";
import { useSession } from "../session";
import { CodeEditor } from "../coding/CodeEditor";
import type { Language } from "../coding/types";
import { contestTime } from "./types";
import { useContestAttempt, type Answer, type CodingSubmission, type Question } from "./useContestAttempt";
import { useContestIntegrity } from "./useContestIntegrity";
import { useContestFeatures } from "./useContestsEnabled";
import "./contestAttempt.css";

const gradingMessage = {
  grading: "Grading in progress.",
  awaiting_release: "Grading complete. Results are awaiting release.",
  needs_review: "A judge error needs review before results can be released.",
};
const codingStatus: Record<CodingSubmission["status"], string> = {
  queued: "Queued",
  judging: "Judging",
  accepted: "Accepted",
  rejected: "Not accepted",
  failed: "Judge error",
};

type StudentAppeal = {
  id: string;
  kind: "concern" | "appeal";
  message: string;
  status: "open" | "in_review" | "resolved" | "dismissed";
  resolution?: string;
  createdAt?: string;
  resolvedAt?: string;
  updatedAt?: string;
};

type StudentReviewNote = {
  status: string;
  reason?: string;
  note?: string;
  updatedAt?: string;
};

type StudentAppealPayload = {
  review?: StudentReviewNote | null;
  reviewStatus?: string;
  reviewNote?: string;
  reviewReason?: string;
  appeals?: StudentAppeal[];
  appeal?: StudentAppeal | null;
};

const appealStatusLabel: Record<StudentAppeal["status"], string> = {
  open: "Open",
  in_review: "In review",
  resolved: "Resolved",
  dismissed: "Dismissed",
};

const reviewStatusLabel: Record<string, string> = {
  under_review: "Under review",
  confirmed_violation: "Confirmed violation",
  cleared: "Cleared",
};

export function ContestAttempt({ id }: { id: string }) {
  const { user } = useSession();
  const [selected, setSelected] = useState(0);
  const [flags, setFlags] = useState<Set<number>>(() => new Set());
  const [review, setReview] = useState(false);
  const { integrityEnabled } = useContestFeatures();
  const { view, answers, dirty, saving, busy, localDurable, error, sizeError, remaining, open, flush, edit, submitCode, submittingCode, codeError } = useContestAttempt(id, user.id);
  const integrity = useContestIntegrity({
    contestId: id,
    userId: user.id,
    attemptId: view?.attemptId,
    enabled: integrityEnabled && view?.state === "active" && !!view.attemptId,
    selectedQuestion: selected,
    reviewMode: review,
    attemptError: error,
  });
  const question = view?.questions?.[selected];
  const answer = answers[selected];
  const locked = busy || remaining === 0 || error.startsWith("Saved answers changed") || error.startsWith("Your saved answers changed");
  const minutes = Math.floor(remaining / 60);
  const timer = `${minutes}:${String(remaining % 60).padStart(2, "0")}`;

  return (
    <section className="contest-session">
      <a href="#/dashboard" onClick={(event) => {
        if (dirty && !localDurable && !window.confirm("Leave with changes that have not been saved?")) event.preventDefault();
      }}>← Dashboard</a>
      <h1>{view?.title ?? "Contest"}</h1>
      {!transport.isDesktop && <p>Open the LMS macOS app to take this contest.</p>}
      {transport.isDesktop && !view && !error && <p role="status">Loading contest…</p>}
      {error && (
        <div className="contest-banner" role="alert">
          <p>{error}</p>
          {view?.state === "active" && error.includes("answers changed") ? (
            <button className="btn btn-secondary" disabled={busy || saving} onClick={() => {
              if (window.confirm("Replace unsynced edits on this device with the server’s saved answers?")) void open("state", true);
            }}>Reload server answers</button>
          ) : (
            <button className="btn btn-secondary" disabled={busy || saving} onClick={() => void open("state")}>Reconnect</button>
          )}
        </div>
      )}
      {view && (view.state === "upcoming" || view.state === "ready") && (
        <div className="contest-banner">
          <p>{contestTime(view.opensAt)} – {contestTime(view.closesAt)} · Up to {view.durationMinutes} minutes</p>
          {view.instructions && <p className="contest-prompt">{view.instructions}</p>}
          <p>Your timer starts when the server admits you and keeps running if you disconnect. At the deadline, the server submits your last saved answers.</p>
          {integrityEnabled && (
            <section className="contest-preflight" aria-label="Contest integrity and rules">
              <h3>Contest integrity &amp; rules</h3>
              <p>
                <strong>Allowed resources:</strong> Follow the contest instructions above. Connection status:{" "}
                {typeof navigator !== "undefined" && !navigator.onLine ? "Offline" : "Online"}.
              </p>
              <p>
                <strong>What is recorded:</strong> contest navigation, MCQ &amp; code edits, paste size, focus/fullscreen changes, reconnects.
              </p>
              <p>
                <strong>What is not collected:</strong> screen, webcam, microphone, clipboard contents, global keystrokes, and unrelated apps stay outside collection.
              </p>
              <p className="contest-fairness-note">
                Automated signals only queue human review and never auto-disqualify.
              </p>
            </section>
          )}
          {view.state === "ready" ? (
            <button className="btn btn-primary" disabled={busy} onClick={() => void open("start")}>Start attempt</button>
          ) : (
            <button className="btn btn-secondary" disabled={busy} onClick={() => void open("state")}>Check availability</button>
          )}
        </div>
      )}
      {view?.state === "closed" && <p>This contest has closed.</p>}
      {view?.state === "cancelled" && <p>This contest was cancelled.</p>}
      {view?.state === "occupied" && (
        <div className="contest-banner">
          <p>This attempt is open on another device. Moving it here keeps the original deadline and the work saved to the server.</p>
          <button className="btn btn-primary" disabled={busy} onClick={() => void open("takeover")}>Continue on this device</button>
        </div>
      )}
      {view?.state === "submitted" && (
        <>
          <div className="contest-banner">
            <h2>Submission received</h2>
            <p>{view.finishReason === "expired" ? "The deadline passed and the server submitted your last saved answers." : "Your answers were submitted."}</p>
            {view.submittedAt && <p>Submitted {contestTime(view.submittedAt)} · {Object.keys(view.answers ?? {}).length} answered</p>}
            {view.gradingStatus && <p role="status">{gradingMessage[view.gradingStatus]}</p>}
            <p className="contest-receipt">Receipt {view.attemptId}</p>
          </div>
          {integrityEnabled && <SubmittedIntegrityPanel contestId={id} />}
        </>
      )}
      {view?.state === "active" && question && (
        <>
          <div className="contest-status">
            <span role="timer" aria-label="Time remaining">{timer}</span>
            <span role="status">{remaining === 0 ? "Confirming submission…" : saving ? "Saving…" : dirty ? localDurable ? "Changes saved on this device" : "Changes still unsaved" : "Saved to server"}</span>
            {integrityEnabled && (
              <div className="contest-status-actions">
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => void (integrity.isFullscreen ? integrity.exitFullscreen() : integrity.enterFullscreen())}
                >
                  {integrity.isFullscreen ? "Exit fullscreen mode" : "Enter fullscreen mode"}
                </button>
              </div>
            )}
          </div>
          {sizeError && <p role="alert">{sizeError}</p>}
          <div className="contest-workspace">
            <nav className="contest-navigation" aria-label="Contest questions">
              {view.questions?.map((_, index) => (
                <button key={index} className="btn btn-secondary" type="button" aria-current={!review && selected === index ? "step" : undefined}
                  onClick={() => { setSelected(index); setReview(false); }}>
                  {index + 1}. {flags.has(index) ? "Flagged" : answers[index] ? "Answered" : "Unanswered"}
                </button>
              ))}
              <button className="btn btn-secondary" onClick={() => setReview(true)}>Review answers</button>
            </nav>
            <div className="contest-question">
              {review ? (
                <>
                  <h2>Review and submit</h2>
                  <p>{Object.keys(answers).length} of {view.questions?.length} answered. {flags.size} flagged for review.</p>
                  <p>Submitting ends your attempt.</p>
                  {view.officialCoding && <p>{view.codingSubmissions?.length ?? 0} official coding submissions. Only official submissions count for coding.</p>}
                  <button className="btn btn-primary" disabled={locked || saving || submittingCode || !!sizeError} onClick={() => void flush(true)}>Submit attempt</button>
                </>
              ) : (
                <>
                  <h2>Question {selected + 1} · {question.points} pts</h2>
                  <p className="contest-prompt">{question.prompt}</p>
                  {question.kind === "mcq" ? (
                    <fieldset className="contest-choices" disabled={locked}>
                      <legend>Choose one answer</legend>
                      {question.choices?.map((choice, index) => (
                        <label key={index}>
                          <input
                            type="radio"
                            name={`contest-question-${selected}`}
                            checked={!!answer && "choice" in answer && answer.choice === index}
                            onChange={() => {
                              edit(selected, { choice: index });
                              integrity.recordMcqSelect(selected, index);
                            }}
                          />
                          {choice}
                        </label>
                      ))}
                    </fieldset>
                  ) : (
                    <CodingQuestion key={selected} question={question} answer={answer} locked={locked}
                      edit={(next) => {
                        edit(selected, next);
                        if ("source" in next) {
                          integrity.recordCodeChange(selected, next.language, next.source);
                        }
                      }}
                      onPasteCapture={integrityEnabled ? integrity.handleEditorPasteCapture(selected) : undefined}
                      official={view.officialCoding}
                      submissions={view.codingSubmissions?.filter((submission) => submission.questionIndex === selected)}
                      submitting={submittingCode} submitDisabled={saving}
                      submit={() => void submitCode(selected)}
                    />
                  )}
                  <button className="btn btn-secondary" aria-pressed={flags.has(selected)} onClick={() => setFlags((previous) => {
                    const next = new Set(previous);
                    if (next.has(selected)) next.delete(selected);
                    else next.add(selected);
                    return next;
                  })}>Flag for review</button>
                </>
              )}
              {codeError && <p role="alert">{codeError}</p>}
              {dirty && !locked && <button className="btn btn-secondary" disabled={saving || submittingCode || !!sizeError} onClick={() => void flush(false)}>Save now</button>}
            </div>
          </div>
        </>
      )}
    </section>
  );
}

function SubmittedIntegrityPanel({ contestId }: { contestId: string }) {
  const [data, setData] = useState<StudentAppealPayload | null>(null);
  const [loadError, setLoadError] = useState("");
  const [formOpen, setFormOpen] = useState(false);
  const [kind, setKind] = useState<"concern" | "appeal">("concern");
  const [message, setMessage] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState("");
  const [submitNotice, setSubmitNotice] = useState("");

  const loadAppeals = useCallback(async () => {
    try {
      const result = await api<StudentAppealPayload>(`/contests/${contestId}/appeal`);
      setData(result);
      setLoadError("");
    } catch (e) {
      setLoadError(e instanceof Error ? e.message : "Could not load appeal status.");
    }
  }, [contestId]);

  useEffect(() => {
    let active = true;
    void api<StudentAppealPayload>(`/contests/${contestId}/appeal`)
      .then((result) => {
        if (active) {
          setData(result);
          setLoadError("");
        }
      })
      .catch((e: Error) => {
        if (active) setLoadError(e.message);
      });
    return () => { active = false; };
  }, [contestId]);

  async function submitAppeal(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = message.trim();
    if (!trimmed || submitting) return;
    setSubmitting(true);
    setSubmitError("");
    setSubmitNotice("");
    try {
      await api(`/contests/${contestId}/appeal`, "POST", { kind, message: trimmed });
      setMessage("");
      setFormOpen(false);
      setSubmitNotice("Your submission has been recorded for staff review.");
      await loadAppeals();
    } catch (e) {
      setSubmitError(e instanceof Error ? e.message : "Could not submit concern or appeal.");
    } finally {
      setSubmitting(false);
    }
  }

  const reviewStatus = data?.review?.status ?? data?.reviewStatus;
  const reviewReason = data?.review?.reason ?? data?.review?.note ?? data?.reviewReason ?? data?.reviewNote;
  const appeals: StudentAppeal[] = data?.appeals ?? (data?.appeal ? [data.appeal] : []);

  return (
    <section className="contest-banner contest-appeal-panel" aria-label="Review and appeals">
      {loadError && <p role="alert">{loadError}</p>}
      {reviewStatus && (reviewStatus === "under_review" || reviewStatus === "confirmed_violation" || !!reviewReason) && (
        <div className="contest-review-note" role="status">
          <p>
            <strong>Private review status:</strong> {reviewStatusLabel[reviewStatus] ?? reviewStatus}
          </p>
          {reviewReason ? (
            <p className="contest-prompt">{reviewReason}</p>
          ) : reviewStatus === "under_review" ? (
            <p className="contest-prompt">Your attempt is queued for human review. You can share context or report a concern below.</p>
          ) : reviewStatus === "confirmed_violation" ? (
            <p className="contest-prompt">A reviewer recorded a violation on this attempt. You may submit an appeal below.</p>
          ) : null}
        </div>
      )}
      <div>
        <button
          type="button"
          className="btn btn-secondary"
          aria-expanded={formOpen}
          onClick={() => {
            setFormOpen((open) => !open);
            setSubmitError("");
          }}
        >
          {formOpen ? "Hide concern or appeal form" : "Report a concern or appeal"}
        </button>
      </div>
      {submitNotice && <p role="status">{submitNotice}</p>}
      {formOpen && (
        <form className="portal-form contest-appeal-form" onSubmit={(e) => { void submitAppeal(e); }}>
          <label>
            Type
            <select value={kind} disabled={submitting} onChange={(e) => setKind(e.target.value as "concern" | "appeal")}>
              <option value="concern">Report a concern</option>
              <option value="appeal">Appeal a review outcome</option>
            </select>
          </label>
          <label>
            Details
            <textarea
              required
              rows={4}
              maxLength={4000}
              disabled={submitting}
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              placeholder="Describe technical issues, interruptions, or context for reviewers."
            />
          </label>
          {submitError && <p role="alert">{submitError}</p>}
          <button type="submit" className="btn btn-primary" disabled={submitting || !message.trim()}>
            {submitting ? "Submitting…" : "Submit"}
          </button>
        </form>
      )}
      {appeals.length > 0 && (
        <div className="contest-appeals-list">
          <h3>Your concerns &amp; appeals</h3>
          <ul>
            {appeals.map((appeal) => (
              <li key={appeal.id} className="contest-appeal-item">
                <div className="contest-appeal-meta">
                  <span className="contest-badge">{appeal.kind === "appeal" ? "Appeal" : "Concern"}</span>
                  <span className="contest-badge">{appealStatusLabel[appeal.status] ?? appeal.status}</span>
                  {appeal.createdAt && <span className="muted">{contestTime(appeal.createdAt)}</span>}
                </div>
                <p className="contest-prompt">{appeal.message}</p>
                {appeal.resolution && (
                  <p className="contest-appeal-resolution">
                    <strong>Resolution:</strong> {appeal.resolution}
                  </p>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

function CodingQuestion({
  question, answer, locked, edit, onPasteCapture, official, submissions, submitting, submitDisabled, submit,
}: {
  question: Question;
  answer?: Answer;
  locked: boolean;
  edit: (answer: Answer) => void;
  onPasteCapture?: (event: ClipboardEvent<HTMLElement>) => void;
  official: boolean;
  submissions?: CodingSubmission[];
  submitting: boolean;
  submitDisabled: boolean;
  submit: () => void;
}) {
  const draft = answer && "language" in answer ? answer : { language: "python" as const, source: "" };
  return (
    <div className="contest-code">
      {question.examples?.map((example, i) => (
        <details key={i}>
          <summary>Example {i + 1}</summary>
          <p>Input</p><pre>{example.input}</pre>
          <p>Expected output</p><pre>{example.expected}</pre>
        </details>
      ))}
      <label>Language <select value={draft.language} disabled={locked}
        onChange={(event) => edit({ ...draft, language: event.target.value as Language })}>
        <option value="python">Python</option>
        <option value="java">Java</option>
      </select></label>
      {locked ? <pre aria-label="Saved code">{draft.source}</pre> : (
        <div className="contest-editor-wrap" onPasteCapture={onPasteCapture}>
          <CodeEditor source={draft.source} language={draft.language} onChange={(source) => edit({ ...draft, source })} />
        </div>
      )}
      {official && (
        <>
          <p>Send this version to the judge before the deadline.</p>
          <button className="btn btn-primary" onClick={submit}
            disabled={locked || submitDisabled || submitting || !draft.source.trim() || draft.source.includes("\0") || new TextEncoder().encode(draft.source).length > 32 * 1024}>
            {submitting ? "Submitting to judge…" : "Submit code to judge"}
          </button>
        </>
      )}
      {!!submissions?.length && (
        <ul className="space-y-2" aria-label="Official coding submissions" aria-live="polite">
          {submissions.map((submission) => (
            <li key={submission.id}>{contestTime(submission.submittedAt)} · {codingStatus[submission.status]}</li>
          ))}
        </ul>
      )}
    </div>
  );
}
