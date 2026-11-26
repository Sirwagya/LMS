import { StudentContests } from "../../portal/contests/StudentContests";
import { useState } from "react";
import {
  ArrowUpRight,
  ArrowRight,
  Search,
  Bell,
  BookOpen,
  Clock,
  MapPin,
  Code2,
  Database,
  Network,
  BrainCircuit,
  Globe,
  Lightbulb,
} from "lucide-react";
import { useLMS } from "../../context/useLMS";
import {
  contestSummary,
  demoWeek,
  drawerNotices,
  lecturesBySubject,
} from "../../data/lmsData";
import {
  attendance,
  formatDate,
  localISO,
  semesterStats,
} from "../../utils/academic";
import { PageHeader, SectionHeader, Badge, EmptyState } from "../ui/Workspace";

const courseIcons = [Code2, BrainCircuit, Globe, Lightbulb, Database, Network];

function toMinutes(hhmm: string) {
  const [h, m] = hhmm.split(":").map(Number);
  return (h || 0) * 60 + (m || 0);
}

function getNextOrRunningSession() {
  const now = new Date(),
    today = localISO(now),
    minutes = now.getHours() * 60 + now.getMinutes();
  for (const day of Object.keys(demoWeek.days).sort()) {
    if (day < today) continue;
    for (const session of demoWeek.days[day] || []) {
      if (day === today && toMinutes(session.to) <= minutes) continue;
      return {
        status:
          day === today && toMinutes(session.from) <= minutes
            ? ("running" as const)
            : ("upcoming" as const),
        dateKey: day,
        dayLabel:
          day === today ? "Today" : formatDate(new Date(`${day}T12:00:00`)),
        session,
      };
    }
  }
  return null;
}

import { useSession } from "../../portal/session";

export function DashboardScreen() {
  const { user } = useSession();
  const {
    currentSemester,
    allSubjects,
    subjectByCode,
    setIsNoticesDrawerOpen,
  } = useLMS();
  const [query, setQuery] = useState("");
  const stats = semesterStats(allSubjects);
  const evaluatedCount = allSubjects.filter(
    (s) => contestSummary[s.code],
  ).length;
  const subjects = allSubjects.filter((s) =>
    `${s.name} ${s.code} ${s.instructor}`
      .toLowerCase()
      .includes(query.toLowerCase().trim()),
  );
  const archived = currentSemester < 3;
  const featuredSubject = allSubjects[0];
  const featuredLecture = featuredSubject
    ? lecturesBySubject[featuredSubject.code]?.[0]
    : undefined;
  const activeOrNext = !archived ? getNextOrRunningSession() : null;
  const activeSubject = activeOrNext
    ? subjectByCode(activeOrNext.session.code)
    : undefined;

  return (
    <section className="screen dashboard-screen">
      <PageHeader
        eyebrow={`B.Tech CSE (AI) · Semester ${currentSemester}`}
        title={
          archived
            ? "Your semester archive."
            : `Welcome back, ${user.name.split(" ")[0]}.`
        }
        description={
          archived
            ? "Revisit your courses and academic records from this semester."
            : "Pick up where you left off."
        }
      />
      <div className="academic-overview">
        <section
          className="attendance-feature"
          aria-labelledby="overall-attendance-title"
        >
          <div className="attendance-feature-grid">
            <div>
              <p className="eyebrow" id="overall-attendance-title">
                Overall attendance
              </p>
              <p className="attendance-value">
                {stats.percent.toFixed(1)}
                <span>%</span>
              </p>
              <p className="muted text-xs">
                {stats.attended} of {stats.total} classes attended
              </p>
            </div>
            {stats.contestAverage !== null && (
              <div className="attendance-feature-col">
                <p className="eyebrow">Contest average</p>
                <p className="attendance-value">
                  {stats.contestAverage}
                  <span>%</span>
                </p>
                <p className="muted text-xs">
                  Across {evaluatedCount} evaluated{" "}
                  {evaluatedCount === 1 ? "course" : "courses"}
                </p>
              </div>
            )}
          </div>
          <a className="btn btn-secondary mt-6" href="#/overview">
            View by course <ArrowUpRight />
          </a>
        </section>
        {activeOrNext ? (
          <article className="panel featured-lecture">
            <div className="featured-card-top">
              <div
                className={`featured-label ${activeOrNext.status === "running" ? "is-live" : ""}`}
              >
                {activeOrNext.status === "running" ? (
                  <span className="live-dot" aria-hidden="true" />
                ) : (
                  <Clock size={14} aria-hidden="true" />
                )}
                <span>
                  {activeOrNext.status === "running"
                    ? "Running lecture"
                    : "Upcoming lecture"}
                </span>
              </div>
              <Badge>{activeOrNext.session.type}</Badge>
            </div>
            <p className="featured-course">
              {activeSubject?.name || activeOrNext.session.code} ·{" "}
              <span className="record-id">{activeOrNext.session.code}</span>
            </p>
            <h2>{activeOrNext.session.topic}</h2>
            <p className="agenda-meta">
              <span>
                {activeOrNext.dayLabel} · {activeOrNext.session.from} –{" "}
                {activeOrNext.session.to}
              </span>
              <span>·</span>
              <span>
                <MapPin aria-hidden="true" /> {activeOrNext.session.room}
              </span>
            </p>
            <a
              className="btn btn-primary"
              href={`#/subjects/${activeOrNext.session.code}`}
            >
              Open course <ArrowUpRight />
            </a>
          </article>
        ) : (
          featuredSubject &&
          featuredLecture && (
            <article className="panel featured-lecture">
              <div className="featured-label">
                <BookOpen size={15} aria-hidden="true" />
                <span>Latest lecture</span>
              </div>
              <p className="featured-course">{featuredSubject.name}</p>
              <h2>{featuredLecture.title}</h2>
              <p className="muted text-xs">
                {featuredLecture.code.replace(/^LEC\s*/i, "Lecture ")} ·{" "}
                {featuredLecture.date}
              </p>
              <a
                className="btn btn-primary"
                href={`#/subjects/${featuredSubject.code}/lectures/${featuredLecture.id}`}
              >
                Open lecture notes <ArrowUpRight />
              </a>
            </article>
          )
        )}
      </div>
      <div className="semester-summary">
        <span>
          <strong>{allSubjects.length}</strong>{" "}
          {archived ? "completed courses" : "enrolled courses"}
        </span>
        <span>
          <strong>
            {allSubjects.reduce((sum, subject) => sum + subject.credits, 0)}
          </strong>{" "}
          academic credits
        </span>
      </div>
      <div>
        <SectionHeader
          title={archived ? "Archived courses" : "Your courses"}
          action={
            <div className="search-field">
              <Search aria-hidden="true" />
              <input
                type="search"
                aria-label="Search courses"
                placeholder="Find a course…"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </div>
          }
        />
        <div className="course-grid dashboard-courses">
          {subjects.map((s) => {
            const Icon =
              courseIcons[allSubjects.indexOf(s) % courseIcons.length];
            const att = attendance(s);
            return (
              <a
                key={s.code}
                href={`#/subjects/${s.code}`}
                className="panel course-card"
              >
                <div className="course-card-top">
                  <span className="course-symbol">
                    <Icon aria-hidden="true" />
                  </span>
                  <span className="course-meta">{s.credits} credits</span>
                </div>
                <div>
                  <h3>{s.name}</h3>
                  <p className="instructor">{s.instructor}</p>
                </div>
                <p className="course-topic">
                  <span>{archived ? "Last topic" : "Up next"}</span>
                  <br />
                  {s.next.replace(/^(Next:|Completed:)\s*/i, "")}
                </p>
                <div className="course-card-footer">
                  <span>
                    <strong>{att.percent.toFixed(1)}%</strong> attendance
                  </span>
                  <ArrowUpRight aria-hidden="true" />
                </div>
              </a>
            );
          })}
        </div>
        {!subjects.length && (
          <EmptyState
            title="No courses found"
            description="Try a course name or instructor."
          >
            <button className="btn btn-secondary" onClick={() => setQuery("")}>
              Clear search
            </button>
          </EmptyState>
        )}
      </div>
      {!archived && <StudentContests />}
      <div>
        <SectionHeader
          title="Notice board"
          action={
            <button
              className="text-link"
              onClick={() => setIsNoticesDrawerOpen(true)}
            >
              All notices <ArrowRight />
            </button>
          }
        />
        {drawerNotices.slice(0, 3).map((n) => (
          <button
            key={n.id}
            className="notice-row"
            onClick={() => setIsNoticesDrawerOpen(true)}
          >
            <Bell aria-hidden="true" />
            <div>
              <h3>{n.title}</h3>
              <p>{n.time}</p>
            </div>
            <Badge tone={n.tag === "URGENT" ? "warning" : "neutral"}>
              {n.tag === "URGENT"
                ? "Time sensitive"
                : n.tag === "FINANCE"
                  ? "Finance"
                  : "Academic"}
            </Badge>
          </button>
        ))}
      </div>
    </section>
  );
}
