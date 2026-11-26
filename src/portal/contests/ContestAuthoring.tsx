import { useEffect, useState, type FormEvent } from "react";
import { api } from "../api";
import { ContestIntegrityReview } from "./ContestIntegrityReview";
import { ContestQuestionEditor } from "./ContestQuestions";
import { RosterPicker } from "./RosterPicker";
import { contestTime, newQuestion, type ContestDraft, type ContestPage, type ContestRecord } from "./types";
import { useContestFeatures } from "./useContestsEnabled";

type Editor = { draft: ContestDraft; record?: ContestRecord };

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
  const { integrityEnabled } = useContestFeatures();

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
          </div>
          {integrityOpen && record && (record.status === "published" || record.status === "cancelled") && integrityEnabled && (
            <ContestIntegrityReview key={record.id} contestId={record.id} />
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
