import { useEffect, useState } from "react";
import { SectionHeader } from "../../components/ui/Workspace";
import { api, transport } from "../api";
import { contestTime, type ContestPage } from "./types";
import { useContestFeatures } from "./useContestsEnabled";

export function StudentContests() {
  const { enabled, attemptsEnabled } = useContestFeatures();
  const [page, setPage] = useState<ContestPage | null>(null);
  const [index, setIndex] = useState(0);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!enabled) return;
    let current = true;
    void api<ContestPage>(`/contests?page=${index}`)
      .then((result) => { if (current) { setPage(result); setError(""); } })
      .catch((e: Error) => { if (current) setError(e.message); });
    return () => { current = false; };
  }, [enabled, index]);

  if (!enabled) return null;
  function changePage(next: number) {
    setPage(null);
    setIndex(next);
  }
  return (
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
  );
}
