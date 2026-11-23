import { assignmentLocation } from "../utils/learningRoutes";
import { lecturesBySubject } from "../data/lmsData";
import { useEffect, useState, type FormEvent } from "react";
import type { Assignment } from "../types";
import { api, fileBase64 } from "./api";
import { useSession, type Discussion } from "./session";
export function AssignmentSubmission({
  assignment,
  expanded = false,
}: {
  assignment: Assignment;
  expanded?: boolean;
}) {
  const { submissions, refresh } = useSession(),
    saved = submissions.find((s) => s.assignment_id === String(assignment.id));
  const [answer, setAnswer] = useState(saved?.answer || ""),
    [busy, setBusy] = useState(false),
    [message, setMessage] = useState("");
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    setMessage("");
    const form = e.currentTarget;
    try {
      const f = (form.elements.namedItem("attachment") as HTMLInputElement)
        .files?.[0];
      await api("/submissions", "POST", {
        assignmentId: assignment.id,
        answer,
        ...(f ? { filename: f.name, file: await fileBase64(f) } : {}),
      });
      await refresh();
      setMessage("Submission saved. Your teacher can now review it.");
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  if (assignment.kind === "coding")
    return (
      <a
        className="btn btn-primary mt-4"
        href={`#${assignmentLocation(assignment.id, lecturesBySubject)?.path || "/assignments"}`}
      >
        {saved?.score
          ? `View solution · ${saved.score}`
          : "Solve coding challenge"}
      </a>
    );
  return (
    <details className="demo-access" open={expanded || undefined}>
      <summary>
        {saved?.score
          ? `Graded · ${saved.score}`
          : saved
            ? "View / update submission"
            : "Submit assignment"}
      </summary>
      {saved && (
        <div className="inline-note mt-3">
          <p>Submitted {new Date(saved.created).toLocaleString()}</p>
          {saved.filename && (
            <a className="text-link" href={`/api/submission-file/${saved.id}`}>
              Download {saved.filename}
            </a>
          )}
          {saved.score && (
            <p>
              Score: {saved.score} · {saved.feedback || "No feedback added."}
            </p>
          )}
        </div>
      )}
      {saved?.score ? (
        <p className="submission-answer">{saved.answer}</p>
      ) : (
        <form className="portal-form" onSubmit={submit}>
          <label>
            Your answer / project link
            <textarea
              required
              rows={4}
              maxLength={20000}
              value={answer}
              onChange={(e) => setAnswer(e.target.value)}
            />
          </label>
          <label>
            Attachment (optional, up to 10 MB)
            <input name="attachment" type="file" />
          </label>
          {saved?.filename && (
            <p className="muted">
              Updating without an attachment removes the old attachment.
            </p>
          )}
          <button className="btn btn-primary" disabled={busy}>
            {busy
              ? "Submitting…"
              : saved
                ? "Update submission"
                : "Submit assignment"}
          </button>
        </form>
      )}
      <p role="status">{message}</p>
    </details>
  );
}
export function LearningTools({ lectureId }: { lectureId: number }) {
  const { progress, refresh } = useSession(),
    [messages, setMessages] = useState<Discussion[]>([]),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const complete = progress.includes(lectureId);
  const load = () =>
    api<Discussion[]>(`/discussions?lecture=${lectureId}`).then(setMessages);
  useEffect(() => {
    let active = true;
    api<Discussion[]>(`/discussions?lecture=${lectureId}`)
      .then((d) => {
        if (active) setMessages(d);
      })
      .catch((e) => {
        if (active) setError(e.message);
      });
    return () => {
      active = false;
    };
  }, [lectureId]);
  async function post(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const body = new FormData(form).get("body");
    setBusy(true);
    setError("");
    try {
      await api("/discussions", "POST", { lectureId, body });
      await load();
      form.reset();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="reader-section">
      <h3>Your learning</h3>
      <button
        className={`btn ${complete ? "btn-secondary" : "btn-primary"} mt-3`}
        disabled={busy}
        onClick={async () => {
          setBusy(true);
          try {
            await api("/progress", "POST", { lectureId, complete: !complete });
            await refresh();
          } catch (e) {
            setError((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        {complete ? "Completed · Mark incomplete" : "Mark lecture complete"}
      </button>
      <h3 className="mt-6">Course discussion</h3>
      {messages.length ? (
        messages.map((m) => (
          <article className="discussion-message" key={m.id}>
            <strong>{m.name}</strong>
            <small className="muted">
              {" "}
              · {new Date(m.created).toLocaleString()}
            </small>
            <p>{m.body}</p>
          </article>
        ))
      ) : (
        <p className="muted mt-3">
          No messages yet. Ask a question to start the discussion.
        </p>
      )}
      <form className="portal-form" onSubmit={post}>
        <label>
          Message
          <textarea name="body" required maxLength={2000} rows={3} />
        </label>
        <button className="btn btn-secondary" disabled={busy}>
          Post message
        </button>
      </form>
      {error && <p role="alert">{error}</p>}
    </section>
  );
}
