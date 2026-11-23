import { subjectPath, assignmentPath } from "../../utils/learningRoutes";
import { useSession } from "../../portal/session";
import { useMemo, useState } from "react";
import { ArrowLeft, Search, FileText } from "lucide-react";
import { useLMS } from "../../context/useLMS";
import { lecturesBySubject } from "../../data/lmsData";
import { attendance } from "../../utils/academic";
import { PageHeader, Badge, EmptyState } from "../ui/Workspace";

import {
  AssignmentSubmission,
  LearningTools,
} from "../../portal/LearningTools";

export function LecturesScreen() {
  const { currentSubjectCode, subjectByCode, selectedLectureId } = useLMS();
  const { submissions } = useSession();
  const [query, setQuery] = useState("");
  const subject = subjectByCode(currentSubjectCode);
  const lectures = useMemo(
    () => lecturesBySubject[currentSubjectCode] || [],
    [currentSubjectCode],
  );
  const filtered = lectures.filter((l) =>
    `${l.title} ${l.summary} ${l.code}`
      .toLowerCase()
      .includes(query.toLowerCase().trim()),
  );
  const selected =
    lectures.find((l) => l.id === selectedLectureId) || filtered[0];
  if (!subject)
    return (
      <section className="screen">
        <PageHeader
          title="Course not found"
          description="This course is not available in your academic record."
        />
        <a href="#/dashboard" className="btn btn-primary">
          Back to subject
        </a>
      </section>
    );
  const att = attendance(subject);
  return (
    <section className="screen">
      <a
        href={`#${subjectPath(currentSubjectCode)}`}
        className="text-link self-start"
      >
        <ArrowLeft />
        Back to subject
      </a>
      <PageHeader
        eyebrow={`Semester coursework · ${subject.credits} credits`}
        title={subject.name}
        description={subject.instructor}
        actions={
          <Badge tone={att.percent >= 75 ? "success" : "warning"}>
            {att.percent.toFixed(1)}% attendance
          </Badge>
        }
      />
      <div className="lecture-layout">
        <div>
          <div className="section-header">
            <h2>Lectures</h2>
            <span className="muted text-xs">{lectures.length} available</span>
          </div>
          <div className="search-field">
            <Search aria-hidden="true" />
            <input
              type="search"
              aria-label="Search lectures"
              placeholder="Search lectures…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
          <nav
            id="lecture-list"
            tabIndex={-1}
            aria-label="Course lectures"
            className="panel lecture-list"
          >
            {filtered.map((l) => (
              <a
                className="lecture-item"
                key={l.id}
                href={`#/subjects/${currentSubjectCode}/lectures/${l.id}`}
                aria-current={selected?.id === l.id ? "true" : undefined}
                onClick={() => {
                  if (window.matchMedia("(max-width: 1023px)").matches) {
                    requestAnimationFrame(() =>
                      document.getElementById("lecture-reader")?.focus(),
                    );
                  }
                }}
              >
                <div className="lecture-meta">
                  <span>{l.code.replace(/^LEC\s*/i, "Lecture ")}</span>
                  <span>{l.date}</span>
                </div>
                <h3>{l.title}</h3>
              </a>
            ))}
            {!filtered.length && (
              <EmptyState
                title={query ? "No matching lectures" : "No lectures yet"}
                description={
                  query
                    ? "Try a different topic or lecture number."
                    : "Lecture details will appear once published."
                }
              >
                {query && (
                  <button
                    className="btn btn-secondary"
                    onClick={() => setQuery("")}
                  >
                    Clear search
                  </button>
                )}
              </EmptyState>
            )}
          </nav>
        </div>
        {selected ? (
          <article
            id="lecture-reader"
            tabIndex={-1}
            className="panel lecture-reader"
            aria-label="Lecture details"
          >
            <button
              className="text-link mb-4 lg:hidden"
              onClick={() => document.getElementById("lecture-list")?.focus()}
            >
              <ArrowLeft />
              Back to lecture list
            </button>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <span className="muted text-xs">
                {selected.code.replace(/^LEC\s*/i, "Lecture ")} ·{" "}
                {selected.date}
                {selected.duration ? ` · ${selected.duration}` : ""}
              </span>
              <Badge
                tone={selected.attendance === "Present" ? "success" : "neutral"}
              >
                {selected.attendance}
              </Badge>
            </div>
            <h2>{selected.title}</h2>
            <nav className="lecture-section-nav" aria-label="Lecture content">
              <a
                href="#lecture-video"
                onClick={(e) => {
                  e.preventDefault();
                  document
                    .getElementById("lecture-video")
                    ?.scrollIntoView({ behavior: "smooth" });
                }}
              >
                Video
              </a>
              <a
                href="#lecture-notes"
                onClick={(e) => {
                  e.preventDefault();
                  document
                    .getElementById("lecture-notes")
                    ?.scrollIntoView({ behavior: "smooth" });
                }}
              >
                Notes
              </a>
              <a
                href="#lecture-assignments"
                onClick={(e) => {
                  e.preventDefault();
                  document
                    .getElementById("lecture-assignments")
                    ?.scrollIntoView({ behavior: "smooth" });
                }}
              >
                Assignments ({selected.assignments.length})
              </a>
              <a
                href="#lecture-attendance"
                onClick={(e) => {
                  e.preventDefault();
                  document
                    .getElementById("lecture-attendance")
                    ?.scrollIntoView({ behavior: "smooth" });
                }}
              >
                Attendance
              </a>
            </nav>
            <section id="lecture-video" className="reader-section">
              <h3>Lecture video</h3>
              {selected.recordingUrl ? (
                <video
                  className="lecture-video"
                  controls
                  preload="metadata"
                  src={selected.recordingUrl}
                  aria-label="Lecture recording"
                />
              ) : (
                <p className="inline-note mt-4">
                  No recording has been uploaded for this lecture.
                </p>
              )}
            </section>
            <section id="lecture-notes" className="reader-section">
              <h3>Notes & materials</h3>
              <p className="summary lecture-notes-text">{selected.summary}</p>
              <p className="muted text-xs mb-3">
                Download the published course materials below. Sample notes are
                included for local learning.
              </p>
              {(selected.resources.length
                ? selected.resources
                : [{ title: selected.slides, type: "Slides" }]
              ).map((res, i) => (
                <div key={i} className="file-row">
                  <span className="file-name">
                    <FileText aria-hidden="true" />
                    {res.title.replace(
                      new RegExp(`^${subject.code}[_\\s-]+`, "i"),
                      "",
                    )}
                  </span>
                  <span className="shrink-0">{res.size || res.type}</span>
                  {res.url && (
                    <a className="text-link" href={res.url} download>
                      Download
                    </a>
                  )}
                </div>
              ))}
            </section>
            <section id="lecture-assignments" className="reader-section">
              <h3>Assignments</h3>
              {!selected.assignments.length && (
                <p className="muted mt-3">
                  No assignments have been published for this lecture.
                </p>
              )}
              {selected.assignments.map((item) => (
                <div key={item.id}>
                  <div className="assignment-row">
                    <div>
                      <h4>
                        <a
                          className="lecture-assignment-link"
                          href={`#${assignmentPath(currentSubjectCode, selected.id, item.id)}`}
                        >
                          {item.title}
                        </a>
                      </h4>
                      <p>
                        {item.dueDate
                          ? `Due ${item.dueDate}`
                          : "No due date listed"}
                        {item.score ? ` · Score ${item.score}` : ""}
                      </p>
                    </div>
                    <Badge
                      tone={item.status === "Pending" ? "warning" : "success"}
                    >
                      {submissions.find(
                        (s) => s.assignment_id === String(item.id),
                      )?.score
                        ? "Graded"
                        : submissions.some(
                              (s) => s.assignment_id === String(item.id),
                            )
                          ? "Submitted"
                          : "Pending"}
                    </Badge>
                  </div>
                  <AssignmentSubmission key={item.id} assignment={item} />
                </div>
              ))}
            </section>
            <section id="lecture-attendance" className="reader-section">
              <div className="section-header">
                <h3>Attendance</h3>
                <Badge
                  tone={
                    selected.attendance === "Present" ? "success" : "neutral"
                  }
                >
                  {selected.attendance === "Awaiting"
                    ? "Not recorded"
                    : selected.attendance}
                </Badge>
              </div>
              <p className="muted text-xs">
                Your attendance record for this lecture. Lesson completion and
                class attendance are tracked separately.
              </p>
              <p className="lecture-attendance-total">
                Course total:{" "}
                <strong>
                  {att.attended}/{att.total} classes · {att.percent.toFixed(1)}%
                </strong>
              </p>
            </section>
            <LearningTools key={selected.id} lectureId={selected.id} />
          </article>
        ) : (
          <div className="panel">
            <EmptyState
              title="Select a lecture"
              description={
                query
                  ? "Clear your search or choose a matching lecture to see its details."
                  : "Published lecture notes and assignments will appear here."
              }
            />
          </div>
        )}
      </div>
    </section>
  );
}
