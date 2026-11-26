import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { Badge, Metric, PageHeader, SectionHeader } from "../../components/ui/Workspace";
import { api } from "../api";
import { contestTime } from "./types";
import "./contestAttempt.css";

export type LeaderboardRow = {
  rank: number;
  displayName: string;
  alias?: string;
  points: number;
  possiblePoints: number;
  elapsedMs: number;
  isSelf?: boolean;
  attemptId?: string;
  studentId?: string;
  withheld?: boolean;
};

export type LeaderboardResponse = {
  items: LeaderboardRow[];
  hasMore: boolean;
  page?: number;
  totalRanked?: number;
  selfRow?: LeaderboardRow | null;
  alias?: string;
};

export type ReportQuestion = {
  index?: number;
  questionIndex?: number;
  kind: "mcq" | "coding";
  prompt: string;
  points?: number;
  earnedPoints?: number;
  score?: number;
  possiblePoints?: number;
  maxPoints?: number;
  choices?: string[];
  studentChoice?: number | null;
  selectedChoice?: number | null;
  choice?: number | null;
  correctChoice?: number | null;
  correct?: boolean;
  isCorrect?: boolean;
  verdict?: string;
  status?: string;
  explanation?: string;
  language?: string;
  submittedCode?: string;
  source?: string;
  code?: string;
  submittedAt?: string;
};

export type StudentReport = {
  assessmentId: string;
  title?: string;
  version: number;
  status: "published" | "withheld" | string;
  publishedAt?: string;
  reviewNote?: string;
  rank?: number | null;
  totalRanked?: number;
  points?: number | null;
  possiblePoints: number;
  mcqPoints?: number;
  mcqPossiblePoints?: number;
  codingPoints?: number;
  codingPossiblePoints?: number;
  elapsedMs?: number | null;
  releaseExplanations?: boolean;
  breakdown?: ReportQuestion[];
  questions?: ReportQuestion[];
};

const codingVerdictLabel: Record<string, string> = {
  accepted: "Accepted",
  rejected: "Not accepted",
  failed: "Judge error",
  needs_review: "Needs review",
  not_submitted: "Not submitted",
};

/** Formats tie-break elapsed milliseconds into a concise human-readable duration. */
function formatElapsed(elapsedMs?: number | null): string {
  if (elapsedMs === undefined || elapsedMs === null || !Number.isFinite(elapsedMs) || elapsedMs < 0) {
    return "—";
  }
  const totalSeconds = Math.floor(elapsedMs / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) {
    return `${hours}h ${String(minutes).padStart(2, "0")}m ${String(seconds).padStart(2, "0")}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${String(seconds).padStart(2, "0")}s`;
  }
  return `${seconds}s`;
}

function questionPoints(q: ReportQuestion): { earned: number; possible: number } {
  if (typeof q.earnedPoints === "number") {
    return { earned: q.earnedPoints, possible: q.possiblePoints ?? q.maxPoints ?? q.points ?? 0 };
  }
  if (typeof q.possiblePoints === "number" || typeof q.maxPoints === "number") {
    return { earned: q.score ?? q.points ?? 0, possible: q.possiblePoints ?? q.maxPoints ?? 0 };
  }
  return { earned: q.score ?? q.points ?? 0, possible: q.points ?? 0 };
}

function reportQuestions(report: StudentReport): ReportQuestion[] {
  return report.breakdown ?? report.questions ?? [];
}

function questionBreakdownTotals(report: StudentReport) {
  const questions = reportQuestions(report);
  let mcqEarned = 0;
  let mcqPossible = 0;
  let codingEarned = 0;
  let codingPossible = 0;
  let mcqCount = 0;
  let codingCount = 0;

  for (const q of questions) {
    const { earned, possible } = questionPoints(q);
    if (q.kind === "mcq") {
      mcqCount += 1;
      mcqEarned += earned;
      mcqPossible += possible;
    } else {
      codingCount += 1;
      codingEarned += earned;
      codingPossible += possible;
    }
  }

  return {
    mcqEarned: report.mcqPoints ?? mcqEarned,
    mcqPossible: report.mcqPossiblePoints ?? mcqPossible,
    codingEarned: report.codingPoints ?? codingEarned,
    codingPossible: report.codingPossiblePoints ?? codingPossible,
    mcqCount,
    codingCount,
  };
}

function mcqChoiceIndex(q: ReportQuestion): number | null {
  const value = q.studentChoice ?? q.selectedChoice ?? q.choice;
  return typeof value === "number" ? value : null;
}

function questionVerdict(q: ReportQuestion, earned: number, possible: number): { label: string; tone: "success" | "warning" | "neutral" } {
  if (q.kind === "mcq") {
    const selected = mcqChoiceIndex(q);
    if (q.verdict) {
      const normalized = q.verdict.toLowerCase();
      if (normalized === "correct") return { label: "Correct", tone: "success" };
      if (normalized === "incorrect") return { label: "Incorrect", tone: "warning" };
      return { label: q.verdict, tone: "neutral" };
    }
    if (selected === null) return { label: "Unanswered", tone: "neutral" };
    const isCorrect = q.correct ?? q.isCorrect ?? (possible > 0 && earned >= possible);
    return isCorrect ? { label: "Correct", tone: "success" } : { label: "Incorrect", tone: "warning" };
  }

  const raw = (q.verdict ?? q.status ?? "").toLowerCase();
  if (raw === "accepted") return { label: codingVerdictLabel.accepted, tone: "success" };
  if (raw in codingVerdictLabel) return { label: codingVerdictLabel[raw], tone: "warning" };
  if (raw) return { label: q.verdict ?? q.status ?? "Pending", tone: "neutral" };
  if (possible > 0 && earned >= possible) return { label: "Accepted", tone: "success" };
  return { label: "Not accepted", tone: "neutral" };
}

/** Shared cohort leaderboard table with optional pinned self row and pagination. */
export function LeaderboardTable({
  leaderboard,
  page,
  loading,
  onPageChange,
}: {
  leaderboard: LeaderboardResponse;
  page: number;
  loading?: boolean;
  onPageChange: (nextPage: number) => void;
}) {
  const selfOnPage = leaderboard.items.some((row) => row.isSelf);
  const pinnedSelf = leaderboard.selfRow && !selfOnPage ? leaderboard.selfRow : null;

  return (
    <div className="contest-leaderboard-block">
      {pinnedSelf && (
        <div className="contest-self-pin" aria-label="Your leaderboard position">
          <div>
            <strong>Your position: #{pinnedSelf.rank}</strong> · {pinnedSelf.displayName}{" "}
            <span className="contest-badge">You</span>
          </div>
          <div className="contest-tabular">
            <strong>{pinnedSelf.points} / {pinnedSelf.possiblePoints}</strong> · {formatElapsed(pinnedSelf.elapsedMs)}
          </div>
        </div>
      )}

      {leaderboard.items.length === 0 ? (
        <p className="muted">
          {page > 0 ? "No ranked participants on this page." : "No ranked participants yet."}
        </p>
      ) : (
        <div className="table-scroll">
          <table className="data-table contest-leaderboard-table">
            <thead>
              <tr>
                <th scope="col">Rank</th>
                <th scope="col">Participant</th>
                <th scope="col" className="amount">Score</th>
                <th scope="col" className="amount">Tie-break time</th>
              </tr>
            </thead>
            <tbody>
              {leaderboard.items.map((row) => (
                <tr
                  key={`${row.rank}-${row.attemptId ?? row.studentId ?? row.displayName}`}
                  className={row.isSelf ? "contest-leaderboard-self" : undefined}
                >
                  <td className="contest-tabular">#{row.rank}</td>
                  <td>
                    <span>{row.displayName}</span>{" "}
                    {row.isSelf && <span className="contest-badge">You</span>}
                    {row.withheld && <span className="contest-badge">Withheld</span>}
                  </td>
                  <td className="amount">
                    {row.points} / {row.possiblePoints}
                  </td>
                  <td className="amount">{formatElapsed(row.elapsedMs)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {(page > 0 || leaderboard.hasMore) && (
        <div className="inline-actions">
          <button
            type="button"
            className="btn btn-secondary"
            disabled={loading || page === 0}
            onClick={() => onPageChange(Math.max(0, page - 1))}
          >
            Previous
          </button>
          <span className="muted">Page {page + 1}</span>
          <button
            type="button"
            className="btn btn-secondary"
            disabled={loading || !leaderboard.hasMore || page >= 10000}
            onClick={() => onPageChange(page + 1)}
          >
            Next
          </button>
        </div>
      )}
    </div>
  );
}

/** Student contest results report, withheld review status, and cohort leaderboard. */
export function ContestResults({ id }: { id: string }) {
  const [report, setReport] = useState<StudentReport | null>(null);
  const [reportLoading, setReportLoading] = useState(true);
  const [reportError, setReportError] = useState("");

  const [leaderboardPage, setLeaderboardPage] = useState(0);
  const [leaderboard, setLeaderboard] = useState<LeaderboardResponse | null>(null);
  const [leaderboardLoading, setLeaderboardLoading] = useState(true);
  const [leaderboardError, setLeaderboardError] = useState("");

  const [aliasInput, setAliasInput] = useState("");
  const aliasDirtyRef = useRef(false);
  const [aliasBusy, setAliasBusy] = useState(false);
  const [aliasError, setAliasError] = useState("");
  const [aliasNotice, setAliasNotice] = useState("");

  useEffect(() => {
    let active = true;
    void api<StudentReport>(`/contests/${id}/report`)
      .then((data) => {
        if (!active) return;
        setReport(data);
        setReportError("");
        setReportLoading(false);
      })
      .catch((e: Error) => {
        if (!active) return;
        setReport(null);
        setReportError(e.message || "Contest results are not published yet.");
        setReportLoading(false);
      });
    return () => {
      active = false;
    };
  }, [id]);

  const fetchLeaderboard = useCallback(
    async (page: number) => {
      try {
        const data = await api<LeaderboardResponse>(`/contests/${id}/leaderboard?page=${page}`);
        setLeaderboard(data);
        setLeaderboardError("");
        const existingAlias = data.selfRow?.alias ?? data.alias;
        if (!aliasDirtyRef.current && typeof existingAlias === "string") {
          setAliasInput(existingAlias);
        }
      } catch (e) {
        setLeaderboard(null);
        setLeaderboardError(e instanceof Error ? e.message : "Cohort leaderboard is not available.");
      } finally {
        setLeaderboardLoading(false);
      }
    },
    [id],
  );

  useEffect(() => {
    let active = true;
    void api<LeaderboardResponse>(`/contests/${id}/leaderboard?page=${leaderboardPage}`)
      .then((data) => {
        if (!active) return;
        setLeaderboard(data);
        setLeaderboardError("");
        setLeaderboardLoading(false);
        const existingAlias = data.selfRow?.alias ?? data.alias;
        if (!aliasDirtyRef.current && typeof existingAlias === "string") {
          setAliasInput(existingAlias);
        }
      })
      .catch((e: Error) => {
        if (!active) return;
        setLeaderboard(null);
        setLeaderboardError(e.message || "Cohort leaderboard is not available.");
        setLeaderboardLoading(false);
      });
    return () => {
      active = false;
    };
  }, [id, leaderboardPage]);

  async function saveAlias(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (aliasBusy) return;
    setAliasBusy(true);
    setAliasError("");
    setAliasNotice("");
    try {
      const trimmed = aliasInput.trim();
      const updated = await api<{ alias?: string }>(`/contests/${id}/alias`, "PUT", { alias: trimmed });
      setAliasInput(updated.alias ?? trimmed);
      aliasDirtyRef.current = false;
      setAliasNotice(trimmed ? "Display alias updated." : "Display alias cleared.");
      setLeaderboardLoading(true);
      await fetchLeaderboard(leaderboardPage);
    } catch (e) {
      setAliasError(e instanceof Error ? e.message : "Could not update display alias.");
    } finally {
      setAliasBusy(false);
    }
  }

  const breakdown = report ? questionBreakdownTotals(report) : null;
  const isWithheld = report?.status === "withheld";

  return (
    <section className="screen contest-results-screen">
      <div>
        <a className="text-link" href="#/dashboard">
          ← Back to dashboard
        </a>
      </div>

      <PageHeader
        eyebrow="Contest results"
        title={report?.title ?? "Contest report"}
        description={
          report?.publishedAt
            ? `Published ${contestTime(report.publishedAt)} · Release v${report.version}`
            : "Review your graded report, question breakdown, and cohort ranking."
        }
        actions={
          <a className="btn btn-secondary" href={`#/contests/${id}`}>
            Contest workspace
          </a>
        }
      />

      {reportLoading && !report && !reportError && (
        <div className="panel panel-padding">
          <p role="status">Loading contest report…</p>
        </div>
      )}

      {reportError && (
        <section className="panel panel-padding" aria-label="Report status">
          <h2>Results not yet released</h2>
          <p className="muted mt-2">{reportError}</p>
          <div className="inline-actions mt-4">
            <a className="btn btn-secondary" href={`#/contests/${id}`}>
              Open contest workspace
            </a>
            <a className="btn btn-quiet" href="#/dashboard">
              Return to dashboard
            </a>
          </div>
        </section>
      )}

      {report && isWithheld && (
        <section className="panel panel-padding" aria-label="Withheld result notice">
          <div className="contest-review-note" role="status">
            <div className="contest-appeal-meta">
              <Badge tone="warning">Under review</Badge>
              {report.version > 0 && <span className="muted">Release v{report.version}</span>}
            </div>
            <p>
              Your results for this contest are currently withheld while staff complete a review.
            </p>
            {report.reviewNote && (
              <p className="contest-prompt">
                <strong>Review note:</strong> {report.reviewNote}
              </p>
            )}
          </div>
          <div className="inline-actions mt-4">
            <a className="btn btn-primary" href={`#/contests/${id}`}>
              Open contest workspace &amp; appeals
            </a>
            <a className="btn btn-secondary" href="#/dashboard">
              Back to dashboard
            </a>
          </div>
        </section>
      )}

      {report && !isWithheld && (
        <>
          <section className="panel panel-padding" aria-label="Score summary">
            <SectionHeader title="Your performance" />
            <dl className="metrics">
              <Metric
                label="Score"
                value={
                  report.points !== undefined && report.points !== null
                    ? `${report.points} / ${report.possiblePoints}`
                    : `— / ${report.possiblePoints}`
                }
                detail={
                  report.points !== undefined && report.points !== null && report.possiblePoints > 0
                    ? `${Math.round((report.points / report.possiblePoints) * 100)}%`
                    : undefined
                }
              />
              <Metric
                label="Rank"
                value={report.rank !== undefined && report.rank !== null ? `#${report.rank}` : "Unranked"}
                detail={
                  typeof report.totalRanked === "number"
                    ? `of ${report.totalRanked} ranked`
                    : undefined
                }
              />
              <Metric
                label="Elapsed time"
                value={formatElapsed(report.elapsedMs)}
                detail="Tie-break time"
              />
            </dl>

            {breakdown && (breakdown.mcqPossible > 0 || breakdown.codingPossible > 0 || breakdown.mcqCount > 0 || breakdown.codingCount > 0) && (
              <div className="contest-breakdown-summary">
                <div className="contest-breakdown-pill">
                  <span className="muted">Multiple choice</span>
                  <strong className="contest-tabular">
                    {breakdown.mcqEarned} / {breakdown.mcqPossible} pts
                  </strong>
                </div>
                <div className="contest-breakdown-pill">
                  <span className="muted">Coding</span>
                  <strong className="contest-tabular">
                    {breakdown.codingEarned} / {breakdown.codingPossible} pts
                  </strong>
                </div>
              </div>
            )}
          </section>

          <section aria-label="Question breakdown">
            <SectionHeader
              title="Question breakdown"
              detail={
                report.releaseExplanations || reportQuestions(report).some((q) => Boolean(q.explanation))
                  ? "Explanations are enabled for this release."
                  : undefined
              }
            />
            {reportQuestions(report).length === 0 ? (
              <div className="panel panel-padding">
                <p className="muted">No question-level details are attached to this report.</p>
              </div>
            ) : (
              <ol className="contest-report-questions">
                {reportQuestions(report).map((question, idx) => {
                  const number = (question.questionIndex ?? question.index ?? idx) + 1;
                  const { earned, possible } = questionPoints(question);
                  const verdict = questionVerdict(question, earned, possible);
                  const choiceIdx = mcqChoiceIndex(question);
                  const submittedCode = question.submittedCode ?? question.source ?? question.code;

                  return (
                    <li key={number} className="panel panel-padding contest-report-question">
                      <div className="contest-integrity-header">
                        <div>
                          <h3>
                            Question {number} · {question.kind === "mcq" ? "Multiple choice" : "Coding"}
                          </h3>
                          <p className="muted contest-tabular">
                            {earned} / {possible} pts
                          </p>
                        </div>
                        <Badge tone={verdict.tone}>{verdict.label}</Badge>
                      </div>

                      <p className="contest-prompt">{question.prompt}</p>

                      {question.kind === "mcq" ? (
                        <div className="contest-mcq-review">
                          {question.choices && question.choices.length > 0 ? (
                            <ul className="contest-choice-review">
                              {question.choices.map((choiceText, cIdx) => {
                                const isSelected = choiceIdx === cIdx;
                                const isCorrect = question.correctChoice === cIdx;
                                return (
                                  <li
                                    key={cIdx}
                                    className={[
                                      "contest-choice-row",
                                      isSelected ? "is-selected" : "",
                                      isCorrect ? "is-correct" : "",
                                    ]
                                      .filter(Boolean)
                                      .join(" ")}
                                  >
                                    <span>{choiceText}</span>
                                    <span className="contest-pill-list">
                                      {isSelected && <span className="contest-badge">Your choice</span>}
                                      {isCorrect && <span className="contest-badge">Correct answer</span>}
                                    </span>
                                  </li>
                                );
                              })}
                            </ul>
                          ) : (
                            <p className="muted">
                              Your choice: {choiceIdx !== null ? `Option ${choiceIdx + 1}` : "No answer selected"}
                              {typeof question.correctChoice === "number" &&
                                ` · Correct option: ${question.correctChoice + 1}`}
                            </p>
                          )}

                          {question.explanation && (
                            <div className="contest-explanation">
                              <strong>Explanation</strong>
                              <p className="contest-prompt">{question.explanation}</p>
                            </div>
                          )}
                        </div>
                      ) : (
                        <div className="contest-coding-review">
                          {(question.language || question.submittedAt) && (
                            <p className="muted">
                              {question.language ? `Language: ${question.language}` : ""}
                              {question.language && question.submittedAt ? " · " : ""}
                              {question.submittedAt ? `Submitted ${contestTime(question.submittedAt)}` : ""}
                            </p>
                          )}
                          {submittedCode ? (
                            <pre className="contest-code-snapshot" aria-label={`Submitted code for question ${number}`}>
                              <code>{submittedCode}</code>
                            </pre>
                          ) : (
                            <p className="muted">No official coding submission recorded.</p>
                          )}
                        </div>
                      )}
                    </li>
                  );
                })}
              </ol>
            )}
          </section>
        </>
      )}

      <section className="panel panel-padding" aria-label="Cohort leaderboard">
        <SectionHeader
          title="Cohort leaderboard"
          detail={
            typeof leaderboard?.totalRanked === "number"
              ? `${leaderboard.totalRanked} ranked participants`
              : undefined
          }
        />

        <form className="contest-inline-form contest-alias-form" onSubmit={(e) => { void saveAlias(e); }}>
          <label>
            Contest display alias
            <input
              maxLength={60}
              disabled={aliasBusy}
              value={aliasInput}
              onChange={(e) => {
                setAliasInput(e.target.value);
                aliasDirtyRef.current = true;
              }}
              placeholder="Optional public display alias"
            />
          </label>
          <button type="submit" className="btn btn-secondary" disabled={aliasBusy}>
            {aliasBusy ? "Saving…" : "Save alias"}
          </button>
          {aliasError && <p role="alert">{aliasError}</p>}
          {aliasNotice && <p role="status">{aliasNotice}</p>}
        </form>

        {leaderboardLoading && !leaderboard && !leaderboardError && (
          <p role="status" className="mt-4">Loading leaderboard…</p>
        )}
        {leaderboardError && <p role="alert" className="mt-4">{leaderboardError}</p>}

        {leaderboard && (
          <div className="mt-4">
            <LeaderboardTable
              leaderboard={leaderboard}
              page={leaderboardPage}
              loading={leaderboardLoading}
              onPageChange={(next) => {
                setLeaderboardLoading(true);
                setLeaderboardPage(next);
              }}
            />
          </div>
        )}
      </section>
    </section>
  );
}
