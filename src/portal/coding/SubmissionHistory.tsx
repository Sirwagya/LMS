import { useState } from "react";
import { Clock3, Code2, CheckCheck } from "lucide-react";
import type { CodingAttempt } from "./types";
import { TestResults } from "./TestResults";
export function SubmissionHistory({
  attempts,
  onRestore,
  entrypoint,
  busy,
}: {
  attempts: CodingAttempt[];
  onRestore: (attempt: CodingAttempt) => void;
  entrypoint?: string;
  busy: boolean;
}) {
  const [selectedId, setSelectedId] = useState<string | null>(null),
    [filter, setFilter] = useState("all"),
    [copied, setCopied] = useState(false);
  const filtered = attempts.filter(
      (a) => filter === "all" || a.language === filter,
    ),
    selected = filtered.find((a) => a.id === selectedId) || filtered[0];
  const isMethod =
    selected &&
    (!entrypoint ||
      (selected.language === "python"
        ? new RegExp(`def\\s+${entrypoint}\\s*\\(`)
        : new RegExp(`\\b${entrypoint}\\s*\\(`)
      ).test(selected.source));
  return (
    <section className="submission-history" aria-label="Submission history">
      <div className="history-toolbar">
        <div>
          <h2>Submission history</h2>
          <p className="muted">
            Your latest {attempts.length} attempts · saved across sessions
          </p>
        </div>
        <select
          aria-label="Filter history by language"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        >
          <option value="all">All languages</option>
          <option value="python">Python</option>
          <option value="java">Java</option>
        </select>
      </div>
      {filtered.length ? (
        <>
          <div
            className="history-list"
            aria-label="Previous submissions"
          >
            {filtered.map((a) => (
              <button
                key={a.id}
                className="history-row"
                aria-pressed={selected?.id === a.id}
                onClick={() => {
                  setSelectedId(a.id);
                  setCopied(false);
                }}
              >
                <span
                  className="history-verdict"
                  data-accepted={a.verdict === "Accepted"}
                >
                  {a.verdict}
                </span>
                <span>
                  {a.language === "java" ? "Java" : "Python"} · {a.passed}/
                  {a.total} tests
                </span>
                <time dateTime={a.created}>
                  {new Date(a.created).toLocaleString()}
                </time>
              </button>
            ))}
          </div>
          {selected && (
            <article className="history-detail">
              <div className="history-detail-title">
                <h3>Submission details</h3>
                <span className="record-id">#{selected.id.slice(0, 8)}</span>
              </div>
              <div className="history-metrics">
                <span>
                  <CheckCheck size={14} />
                  {selected.passed}/{selected.total} passed
                </span>
                <span>
                  <Clock3 size={14} />
                  {selected.result.results.reduce(
                    (total, t) => total + t.durationMs,
                    0,
                  )}{" "}
                  ms total
                </span>
                <span>
                  <Code2 size={14} />
                  {selected.language === "java" ? "Java" : "Python"}
                </span>
              </div>
              <div className="history-code-actions">
                <h3>Submitted code</h3>
                <div className="inline-actions">
                  <button
                    className="text-link"
                    onClick={() =>
                      navigator.clipboard
                        .writeText(selected.source)
                        .then(() => setCopied(true))
                        .catch(() => setCopied(false))
                    }
                  >
                    {copied ? "Copied" : "Copy code"}
                  </button>
                  {isMethod && (
                    <button
                      className="btn btn-secondary"
                      disabled={busy}
                      onClick={() => onRestore(selected)}
                    >
                      Load in editor
                    </button>
                  )}
                </div>
              </div>
              {!isMethod && (
                <p className="muted text-xs">
                  This older attempt used the previous full-program format. It
                  is preserved here for reference.
                </p>
              )}
              <pre className="code-output history-source">
                {selected.source}
              </pre>
              <TestResults key={selected.id} result={selected.result} />
            </article>
          )}
        </>
      ) : (
        <div className="history-empty">
          <Clock3 size={24} />
          <h3>No submissions yet</h3>
          <p>
            Run the examples, then Submit. Each attempt will appear here with
            its verdict, code and test results.
          </p>
        </div>
      )}
    </section>
  );
}
