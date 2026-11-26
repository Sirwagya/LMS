import { useState } from "react";
import { transport } from "../api";
import { useSession } from "../session";
import { CodeEditor } from "../coding/CodeEditor";
import type { Language } from "../coding/types";
import { contestTime } from "./types";
import { useContestAttempt, type Answer, type Question } from "./useContestAttempt";
import "./contestAttempt.css";

export function ContestAttempt({ id }: { id: string }) {
  const { user } = useSession();
  const [selected, setSelected] = useState(0);
  const [flags, setFlags] = useState<Set<number>>(() => new Set());
  const [review, setReview] = useState(false);
  const { view, answers, dirty, saving, busy, localDurable, error, sizeError, remaining, open, flush, edit } = useContestAttempt(id, user.id);
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
        <div className="contest-banner">
          <h2>Submission received</h2>
          <p>{view.finishReason === "expired" ? "The deadline passed and the server submitted your last saved answers." : "Your answers were submitted."}</p>
          {view.submittedAt && <p>Submitted {contestTime(view.submittedAt)} · {Object.keys(view.answers ?? {}).length} answered</p>}
          <p className="contest-receipt">Receipt {view.attemptId}</p>
        </div>
      )}
      {view?.state === "active" && question && (
        <>
          <div className="contest-status">
            <span role="timer" aria-label="Time remaining">{timer}</span>
            <span role="status">{remaining === 0 ? "Confirming submission…" : saving ? "Saving…" : dirty ? localDurable ? "Changes saved on this device" : "Changes still unsaved" : "Saved to server"}</span>
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
                  <button className="btn btn-primary" disabled={locked || saving || !!sizeError} onClick={() => void flush(true)}>Submit attempt</button>
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
                          <input type="radio" name={`contest-question-${selected}`} checked={!!answer && "choice" in answer && answer.choice === index} onChange={() => edit(selected, { choice: index })} />
                          {choice}
                        </label>
                      ))}
                    </fieldset>
                  ) : (
                    <CodingQuestion key={selected} question={question} answer={answer} locked={locked} edit={(next) => edit(selected, next)} />
                  )}
                  <button className="btn btn-secondary" aria-pressed={flags.has(selected)} onClick={() => setFlags((previous) => {
                    const next = new Set(previous);
                    if (next.has(selected)) next.delete(selected);
                    else next.add(selected);
                    return next;
                  })}>Flag for review</button>
                </>
              )}
              {dirty && !locked && <button className="btn btn-secondary" disabled={saving || !!sizeError} onClick={() => void flush(false)}>Save now</button>}
            </div>
          </div>
        </>
      )}
    </section>
  );
}

function CodingQuestion({ question, answer, locked, edit }: {
  question: Question;
  answer?: Answer;
  locked: boolean;
  edit: (answer: Answer) => void;
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
        <CodeEditor source={draft.source} language={draft.language} onChange={(source) => edit({ ...draft, source })} />
      )}
    </div>
  );
}
