import { useEffect, useState } from "react";
import { Badge, SectionHeader } from "../../components/ui/Workspace";
import { api, transport } from "../api";
import { contestTime, type ContestPage } from "./types";
import { useContestFeatures } from "./useContestsEnabled";

export type StudentResultSummary = {
  assessmentId: string;
  title: string;
  version: number;
  status: "published" | "withheld" | string;
  rank?: number | null;
  points?: number | null;
  possiblePoints: number;
  publishedAt: string;
};

type StudentResultsResponse = {
  items: StudentResultSummary[];
};

export function StudentContests() {
  const { enabled, attemptsEnabled, resultsEnabled } = useContestFeatures();
  const [page, setPage] = useState<ContestPage | null>(null);
  const [index, setIndex] = useState(0);
  const [error, setError] = useState("");
  const [results, setResults] = useState<StudentResultSummary[] | null>(null);
  const [resultsError, setResultsError] = useState("");

  useEffect(() => {
    if (!enabled) return;
    let current = true;
    void api<ContestPage>(`/contests?page=${index}`)
      .then((result) => { if (current) { setPage(result); setError(""); } })
      .catch((e: Error) => { if (current) setError(e.message); });
    return () => { current = false; };
  }, [enabled, index]);

  useEffect(() => {
    if (!resultsEnabled) return;
    let current = true;
    void api<StudentResultsResponse>("/contests/results")
      .then((data) => {
        if (current) {
          setResults(data.items ?? []);
          setResultsError("");
        }
      })
      .catch((e: Error) => {
        if (current) setResultsError(e.message);
      });
    return () => { current = false; };
  }, [resultsEnabled]);

  if (!enabled && !resultsEnabled) return null;
  function changePage(next: number) {
    setPage(null);
    setIndex(next);
  }
  return (
    <>
      {enabled && (
        <section>
          <SectionHeader title="Your contests" />
          <div className="panel panel-padding">
            {error && <p role="alert">{error}</p>}
            {!page && !error && <p role="status">Loading contests…</p>}
            {page?.items.length === 0 && <p className="muted">No scheduled contests.</p>}
            {page && (
              <>
                <ul className="space-y-3">
                  {page.items.map((contest) => (
                    <li key={contest.id}>
                      <strong>{contest.title}</strong>
                      {contest.status === "cancelled" ? <p>Cancelled</p> : (
                        <p className="muted">{contestTime(contest.opensAt)} – {contestTime(contest.closesAt)} · {contest.durationMinutes} min</p>
                      )}
                      {attemptsEnabled && (contest.status !== "cancelled" || contest.attemptState === "submitted") &&
                        (transport.isDesktop ? (
                          <a className="btn btn-secondary" href={`#/contests/${contest.id}`}>
                            {contest.attemptState === "submitted" ? "View receipt" : contest.attemptState === "active" ? "Resume contest" : "Open contest"}
                          </a>
                        ) : (
                          <p className="muted">
                            {contest.attemptState === "submitted" ? "Submitted · Receipt available in the LMS macOS app."
                              : contest.attemptState === "active" ? "In progress · Resume in the LMS macOS app."
                                : "Take this contest in the LMS macOS app."}
                          </p>
                        ))}
                    </li>
                  ))}
                </ul>
                {(index > 0 || page.hasMore) && (
                  <div className="inline-actions mt-4">
                    <button type="button" className="btn btn-secondary" disabled={index === 0} onClick={() => changePage(index - 1)}>Previous</button>
                    <button type="button" className="btn btn-secondary" disabled={!page.hasMore || index >= 10000} onClick={() => changePage(index + 1)}>Next</button>
                  </div>
                )}
              </>
            )}
          </div>
        </section>
      )}

      {resultsEnabled && (
        <section aria-label="Published contest results">
          <SectionHeader title="Contest results" />
          <div className="panel panel-padding">
            {resultsError && <p role="alert">{resultsError}</p>}
            {!results && !resultsError && <p role="status">Loading contest results…</p>}
            {results && results.length === 0 && (
              <p className="muted">No published contest results yet.</p>
            )}
            {results && results.length > 0 && (
              <ul className="space-y-3">
                {results.map((item) => (
                  <li key={`${item.assessmentId}-${item.version}`} className="contest-result-summary-row">
                    <div>
                      <div className="contest-appeal-meta">
                        <strong>{item.title}</strong>
                        <Badge tone={item.status === "withheld" ? "warning" : "success"}>
                          {item.status === "withheld" ? "Under review" : `v${item.version}`}
                        </Badge>
                      </div>
                      <p className="muted">
                        {item.status === "withheld" ? (
                          "Result withheld pending review"
                        ) : (
                          <>
                            {item.points !== undefined && item.points !== null
                              ? `${item.points} / ${item.possiblePoints} pts`
                              : `${item.possiblePoints} pts possible`}
                            {typeof item.rank === "number" && ` · Rank #${item.rank}`}
                          </>
                        )}
                        {item.publishedAt && ` · Published ${contestTime(item.publishedAt)}`}
                      </p>
                    </div>
                    <a className="btn btn-secondary" href={`#/contests/${item.assessmentId}/results`}>
                      {item.status === "withheld" ? "View status" : "View results"}
                    </a>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </section>
      )}
    </>
  );
}
