import { useState } from "react";
import { RotateCcw, ArrowUpRight, ChevronDown } from "lucide-react";
import { useLMS } from "../../context/useLMS";
import { contestSummary } from "../../data/lmsData";
import { attendance, semesterStats } from "../../utils/academic";
import type { ContestAttempt } from "../../types";
import { PageHeader, SectionHeader, Metric } from "../ui/Workspace";

function buildSmoothCubicPath(pts: { x: number; y: number }[]) {
  if (pts.length === 0) return "";
  if (pts.length === 1) return `M ${pts[0]!.x.toFixed(1)} ${pts[0]!.y.toFixed(1)}`;
  let d = `M ${pts[0]!.x.toFixed(1)} ${pts[0]!.y.toFixed(1)}`;
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[i]!;
    const p1 = pts[i + 1]!;
    const dx = (p1.x - p0.x) * 0.44;
    const cp1x = p0.x + dx;
    const cp1y = p0.y;
    const cp2x = p1.x - dx;
    const cp2y = p1.y;
    d += ` C ${cp1x.toFixed(1)} ${cp1y.toFixed(1)}, ${cp2x.toFixed(1)} ${cp2y.toFixed(1)}, ${p1.x.toFixed(1)} ${p1.y.toFixed(1)}`;
  }
  return d;
}

function ContestLineChart({
  code,
  average,
  history,
  activeIndex,
  onSelectIndex,
}: {
  code: string;
  average: number;
  history: ContestAttempt[];
  activeIndex: number;
  onSelectIndex: (idx: number) => void;
}) {
  const width = 500;
  const height = 188;
  const padLeft = 40;
  const padRight = 18;
  const padTop = 16;
  const padBottom = 32;
  const plotW = width - padLeft - padRight;
  const plotH = height - padTop - padBottom;

  const yMin = 55;
  const yMax = 100;
  const yTicks = [55, 70, 85, 100];

  const yForPct = (pct: number) => {
    const clamped = Math.max(yMin, Math.min(yMax, pct));
    return padTop + plotH - ((clamped - yMin) / (yMax - yMin)) * plotH;
  };

  const points = history.map((item, idx) => {
    const x =
      history.length === 1
        ? padLeft + plotW / 2
        : padLeft + (idx / (history.length - 1)) * plotW;
    const y = yForPct(item.percent);
    return { x, y, item, idx };
  });

  const smoothLine = buildSmoothCubicPath(points);
  const baselineY = padTop + plotH;
  const areaPath =
    points.length > 1
      ? `${smoothLine} L ${points[points.length - 1]!.x.toFixed(1)} ${baselineY} L ${points[0]!.x.toFixed(1)} ${baselineY} Z`
      : "";

  const avgY = yForPct(average);
  const safeIdx = Math.max(0, Math.min(points.length - 1, activeIndex));
  const activePt = points[safeIdx]!;
  const diffFromAvg = activePt.item.percent - average;
  const gradId = `minimal-grad-${code}`;

  // Position floating tooltip so it stays inside the chart bounds
  const tooltipLeftPct = ((activePt.x - padLeft) / plotW) * 100;
  const tooltipAlignRight = tooltipLeftPct > 58;

  return (
    <div className="minimal-chart-card">
      <div className="minimal-chart-header">
        <span className="minimal-chart-title">Contest performance</span>
        <div className="minimal-chart-legend" aria-hidden="true">
          <span className="minimal-legend-item">
            <i className="minimal-legend-line is-primary" />
            Marks
          </span>
          <span className="minimal-legend-item">
            <i className="minimal-legend-line is-secondary" />
            Avg ({average}%)
          </span>
        </div>
      </div>

      <div className="minimal-chart-stage">
        <svg
          viewBox={`0 0 ${width} ${height}`}
          className="minimal-line-svg"
          role="img"
          aria-label={`Contest performance chart for ${code}, average ${average}%`}
        >
          <defs>
            <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--accent)" stopOpacity="0.16" />
              <stop offset="100%" stopColor="var(--accent)" stopOpacity="0.0" />
            </linearGradient>
          </defs>

          {/* Subtle solid horizontal grid lines */}
          {yTicks.map((tick) => {
            const y = yForPct(tick);
            return (
              <g key={tick}>
                <line
                  x1={padLeft}
                  y1={y}
                  x2={width - padRight}
                  y2={y}
                  className="minimal-grid-line"
                />
                <text
                  x={padLeft - 10}
                  y={y + 3.5}
                  textAnchor="end"
                  className="minimal-axis-label"
                >
                  {tick}%
                </text>
              </g>
            );
          })}

          {/* Soft gradient fill under primary curve */}
          {areaPath && <path d={areaPath} fill={`url(#${gradId})`} />}

          {/* Secondary baseline curve (course average) */}
          <line
            x1={points[0]?.x ?? padLeft}
            y1={avgY}
            x2={points[points.length - 1]?.x ?? width - padRight}
            y2={avgY}
            className="minimal-secondary-line"
          />

          {/* Smooth cubic-bezier primary score curve */}
          {points.length > 1 && (
            <path d={smoothLine} fill="none" className="minimal-primary-line" />
          )}

          {/* Active vertical dashed cursor line */}
          <line
            x1={activePt.x}
            y1={padTop}
            x2={activePt.x}
            y2={baselineY}
            className="minimal-cursor-line"
          />

          {/* Secondary node on average line */}
          <circle
            cx={activePt.x}
            cy={avgY}
            r={3.5}
            className="minimal-node-secondary"
          />

          {/* Primary ringed node on active point only */}
          <circle
            cx={activePt.x}
            cy={activePt.y}
            r={4.5}
            className="minimal-node-primary"
          />

          {/* Clean X-axis labels */}
          {points.map(({ x, item, idx }) => (
            <text
              key={item.label}
              x={x}
              y={baselineY + 20}
              textAnchor="middle"
              className={`minimal-axis-label ${idx === safeIdx ? "is-active" : ""}`}
            >
              {item.date.replace(/\s+\d{4}$/, "")}
            </text>
          ))}

          {/* Invisible hover/click hit columns */}
          {points.map(({ x, item, idx }) => {
            const colW = plotW / Math.max(1, points.length);
            return (
              <rect
                key={`hit-${item.label}`}
                x={x - colW / 2}
                y={padTop}
                width={colW}
                height={plotH}
                fill="transparent"
                className="minimal-hit-zone"
                onMouseEnter={() => onSelectIndex(idx)}
                onClick={() => onSelectIndex(idx)}
              />
            );
          })}
        </svg>

        {/* Floating minimal tooltip card anchored near active node */}
        <div
          className={`minimal-chart-tooltip ${tooltipAlignRight ? "align-left" : "align-right"}`}
          style={{
            left: `${(activePt.x / width) * 100}%`,
            top: `${Math.max(10, Math.min(52, (activePt.y / height) * 100 - 8))}%`,
          }}
        >
          <p className="minimal-tooltip-date">{activePt.item.date}</p>
          <div className="minimal-tooltip-main">
            <strong>{activePt.item.percent}%</strong>
            <span
              className={`minimal-tooltip-delta ${diffFromAvg >= 0 ? "is-pos" : "is-neg"}`}
            >
              {diffFromAvg >= 0 ? `+${diffFromAvg}%` : `${diffFromAvg}%`}
            </span>
          </div>
          <div className="minimal-tooltip-rows">
            <div>
              <span className="minimal-dot is-primary" />
              <span>Score</span>
              <strong>{activePt.item.score}</strong>
            </div>
            <div>
              <span className="minimal-dot is-secondary" />
              <span>Rank</span>
              <strong>{activePt.item.rank.split("/")[0]?.trim()}</strong>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

export function OverviewScreen() {
  const { currentSemester, allSubjects } = useLMS();
  const [expandedCode, setExpandedCode] = useState<string | null>(null);
  const [hoveredContestIdx, setHoveredContestIdx] = useState<
    Record<string, number>
  >({});
  const [subjectCode, setSubjectCode] = useState("");
  const [attend, setAttend] = useState(0);
  const [miss, setMiss] = useState(0);

  const selected =
    allSubjects.find((s) => s.code === subjectCode) || allSubjects[0];
  const stats = semesterStats(allSubjects);
  const evaluatedCount = allSubjects.filter(
    (s) => contestSummary[s.code],
  ).length;
  const totalSafeBuffer = allSubjects.reduce((sum, s) => {
    const a = attendance(s);
    return sum + Math.max(0, Math.floor(a.attended / 0.75 - a.total));
  }, 0);

  const activeExpandedCode =
    expandedCode && allSubjects.some((s) => s.code === expandedCode)
      ? expandedCode
      : null;

  const base = selected
    ? attendance(selected)
    : { attended: 0, total: 0, percent: 0 };
  const simTotal = base.total + attend + miss;
  const simPercent = simTotal ? ((base.attended + attend) / simTotal) * 100 : 0;
  const simDelta = simPercent - base.percent;
  const buffer = Math.max(
    0,
    Math.floor((base.attended + attend) / 0.75 - simTotal),
  );
  const recovery = Math.max(
    0,
    Math.ceil((0.75 * simTotal - base.attended - attend) / 0.25),
  );
  const reset = () => {
    setAttend(0);
    setMiss(0);
  };

  return (
    <section className="screen">
      <PageHeader
        eyebrow={`Semester ${currentSemester}${currentSemester < 3 ? " · Archived" : ""}`}
        title="Attendance & results"
        description="Overall standing and subject-by-subject attendance and contest marks."
      />

      {/* 1. Overall Attendance & Marks Summary */}
      <dl className="metrics">
        <Metric
          label="Overall attendance"
          value={`${stats.percent.toFixed(1)}%`}
          detail={`${stats.attended} of ${stats.total} classes attended · 75% minimum required`}
        />
        <Metric
          label="Overall contest marks"
          value={
            stats.contestAverage !== null ? `${stats.contestAverage}%` : "—"
          }
          detail={`Average across ${evaluatedCount} evaluated ${evaluatedCount === 1 ? "course" : "courses"}`}
        />
        <Metric
          label="Attendance buffer"
          value={`${totalSafeBuffer} ${totalSafeBuffer === 1 ? "skip" : "skips"}`}
          detail={`${allSubjects.filter((s) => attendance(s).percent >= 75).length} of ${allSubjects.length} courses above 75% requirement`}
        />
      </dl>

      {/* 2. Subject-wise Attendance & Marks Cards */}
      <div>
        <SectionHeader
          title="Subject-wise attendance & marks"
          detail="Select any subject card to view its contest performance curve."
        />
        <div className="subject-overview-list">
          {allSubjects.map((s) => {
            const a = attendance(s);
            const contest = contestSummary[s.code];
            const missed = Math.max(0, a.total - a.attended);
            const minRequired = Math.ceil(a.total * 0.75);
            const isExpanded = activeExpandedCode === s.code;
            const historyLen = contest?.history?.length ?? 0;
            const defaultIdx = Math.max(0, historyLen - 2);
            const activeContestIdx = hoveredContestIdx[s.code] ?? defaultIdx;

            return (
              <article
                key={s.code}
                className={`panel subject-overview-card ${isExpanded ? "is-expanded" : ""}`}
              >
                <button
                  type="button"
                  className="subject-overview-trigger"
                  aria-expanded={isExpanded}
                  onClick={() =>
                    setExpandedCode(isExpanded ? null : s.code)
                  }
                >
                  <div className="subject-overview-main">
                    <h3>{s.name}</h3>
                  </div>

                  <div className="subject-overview-figures">
                    <div className="subject-stat-block">
                      <span className="subject-stat-label">Attendance</span>
                      <strong
                        className={`subject-stat-value ${a.percent < 75 ? "is-warning" : ""}`}
                      >
                        {a.percent.toFixed(1)}%
                      </strong>
                      <span className="subject-stat-sub">
                        {a.attended}/{a.total} classes
                      </span>
                    </div>

                    <div className="subject-stat-block">
                      <span className="subject-stat-label">Contest marks</span>
                      {contest ? (
                        <>
                          <strong className="subject-stat-value">
                            {contest.percent}%
                          </strong>
                          <span className="subject-stat-sub">
                            {contest.completed}/{contest.total} completed ·{" "}
                            {contest.bestRank?.split("/")[0]?.trim()}
                          </span>
                        </>
                      ) : (
                        <>
                          <strong className="subject-stat-value is-muted">
                            —
                          </strong>
                          <span className="subject-stat-sub">No contests</span>
                        </>
                      )}
                    </div>

                    <span
                      className={`subject-expand-icon ${isExpanded ? "is-open" : ""}`}
                      aria-hidden="true"
                    >
                      <ChevronDown size={18} />
                    </span>
                  </div>
                </button>

                {isExpanded && (
                  <div className="subject-overview-expanded">
                    {contest && contest.history && contest.history.length > 0 ? (
                      <div className="subject-expanded-grid">
                        <ContestLineChart
                          code={s.code}
                          average={contest.percent}
                          history={contest.history}
                          activeIndex={activeContestIdx}
                          onSelectIndex={(idx) =>
                            setHoveredContestIdx((prev) => ({
                              ...prev,
                              [s.code]: idx,
                            }))
                          }
                        />

                        <div className="subject-expanded-side">
                          <div className="subject-expanded-header">
                            <div>
                              <h4>Evaluated contests</h4>
                              <p className="muted text-xs">
                                {contest.detail}
                              </p>
                            </div>
                            <a
                              href={`#/subjects/${s.code}`}
                              className="text-link"
                            >
                              Course notes <ArrowUpRight size={14} />
                            </a>
                          </div>

                          <div className="contest-attempt-list">
                            {contest.history.map((item, idx) => {
                              const isCurrent = idx === activeContestIdx;
                              return (
                                <button
                                  type="button"
                                  key={item.label}
                                  className={`contest-attempt-row ${isCurrent ? "is-active" : ""}`}
                                  onMouseEnter={() =>
                                    setHoveredContestIdx((prev) => ({
                                      ...prev,
                                      [s.code]: idx,
                                    }))
                                  }
                                  onClick={() =>
                                    setHoveredContestIdx((prev) => ({
                                      ...prev,
                                      [s.code]: idx,
                                    }))
                                  }
                                >
                                  <div className="contest-attempt-title">
                                    <span className="record-id">
                                      {item.label}
                                    </span>
                                    <strong>
                                      {item.title.replace(
                                        /^(Contest|Practicum|Sprint|Review|Challenge)\s*#\d+\s*—\s*/i,
                                        "",
                                      )}
                                    </strong>
                                  </div>
                                  <div className="contest-attempt-meta">
                                    <span>{item.rank.split("/")[0]?.trim()}</span>
                                    <strong className="tabular-nums">
                                      {item.percent}%
                                    </strong>
                                  </div>
                                </button>
                              );
                            })}
                          </div>
                        </div>
                      </div>
                    ) : (
                      <div className="subject-expanded-empty">
                        <p className="muted text-xs">
                          No coding contests are scheduled for{" "}
                          <strong>{s.name}</strong> ({a.attended} attended,{" "}
                          {missed} missed · {minRequired} required for 75%).
                        </p>
                        <a href={`#/subjects/${s.code}`} className="text-link">
                          Open course notes <ArrowUpRight size={14} />
                        </a>
                      </div>
                    )}
                  </div>
                )}
              </article>
            );
          })}
        </div>
      </div>

      {/* 3. Attendance Planner (Semester 3) */}
      {currentSemester === 3 ? (
        <div className="panel panel-padding">
          <SectionHeader
            title="Plan your attendance"
            detail="Simulate how future classes affect your attendance percentage in any course."
            action={
              (attend > 0 || miss > 0) && (
                <button
                  className="icon-button"
                  onClick={reset}
                  aria-label="Reset attendance calculator"
                >
                  <RotateCcw />
                </button>
              )
            }
          />
          <div className="planner-grid">
            <div>
              <label
                htmlFor="calculator-course"
                className="block text-xs muted mb-2"
              >
                Course
              </label>
              <select
                id="calculator-course"
                className="semester-select w-full"
                value={selected?.code}
                onChange={(e) => {
                  setSubjectCode(e.target.value);
                  reset();
                }}
              >
                {allSubjects.map((s) => (
                  <option key={s.code} value={s.code}>
                    {s.name} ({s.code})
                  </option>
                ))}
              </select>

              <div className="range-field">
                <label htmlFor="attend">
                  Future classes attended <output>+{attend}</output>
                </label>
                <input
                  id="attend"
                  type="range"
                  min={0}
                  max={25}
                  value={attend}
                  onChange={(e) => setAttend(Number(e.target.value))}
                />
              </div>
              <div className="range-field">
                <label htmlFor="miss">
                  Future classes missed <output>+{miss}</output>
                </label>
                <input
                  id="miss"
                  type="range"
                  min={0}
                  max={25}
                  value={miss}
                  onChange={(e) => setMiss(Number(e.target.value))}
                />
              </div>
            </div>

            <div
              className={`sim-result ${simPercent < 75 ? "warning" : ""}`}
              role="status"
              aria-live="polite"
            >
              <span className="eyebrow">Projected attendance</span>
              <strong>{simPercent.toFixed(1)}%</strong>
              <p className="mb-1">
                {base.attended + attend} of {simTotal} classes attended
                {(attend > 0 || miss > 0) &&
                  ` (${simDelta >= 0 ? "+" : ""}${simDelta.toFixed(1)}%)`}
              </p>
              <p>
                {simPercent >= 75
                  ? `You can miss ${buffer} more ${buffer === 1 ? "class" : "classes"} and stay at 75% or above.`
                  : `Attend the next ${recovery} consecutive ${recovery === 1 ? "class" : "classes"} to reach 75%.`}
              </p>
            </div>
          </div>
        </div>
      ) : (
        <div className="inline-note">
          This semester is archived. Attendance and contest records are
          available above for reference; attendance planning applies to the
          active semester.
        </div>
      )}
    </section>
  );
}
