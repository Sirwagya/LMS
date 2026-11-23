import { useState } from "react";
import {
  ArrowLeft,
  ArrowUpRight,
  Search,
  PlayCircle,
  FileText,
  ClipboardList,
} from "lucide-react";
import { lecturesBySubject } from "../../data/lmsData";
import { useLMS } from "../../context/useLMS";
import { useSession } from "../../portal/session";
import { attendance } from "../../utils/academic";
import { lecturePath } from "../../utils/learningRoutes";
import { PageHeader, Badge, EmptyState } from "../ui/Workspace";
export function SubjectScreen() {
  const { currentSubjectCode, subjectByCode } = useLMS(),
    { progress } = useSession(),
    [query, setQuery] = useState("");
  const subject = subjectByCode(currentSubjectCode),
    lectures = lecturesBySubject[currentSubjectCode] || [];
  if (!subject)
    return (
      <EmptyState
        title="Subject not found"
        description="Return to your subjects and choose an available course."
      />
    );
  const filtered = lectures.filter((l) =>
      `${l.title} ${l.summary}`
        .toLowerCase()
        .includes(query.trim().toLowerCase()),
    ),
    att = attendance(subject);
  return (
    <section className="screen">
      <a className="text-link self-start" href="#/dashboard">
        <ArrowLeft />
        All subjects
      </a>
      <PageHeader
        eyebrow={`${subject.credits} credits · ${lectures.length} lectures`}
        title={subject.name}
        description={subject.instructor}
        actions={<Badge>{att.percent.toFixed(1)}% course attendance</Badge>}
      />
      <div className="subject-learning-intro">
        <h2>Your lectures</h2>
        <p className="muted">
          Open a lecture for its video, notes, assignments and attendance.
        </p>
        <div className="search-field">
          <Search aria-hidden="true" />
          <input
            type="search"
            aria-label="Search subject lectures"
            placeholder="Find a lecture…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
      </div>
      <div className="subject-lecture-grid">
        {filtered.map((lecture, i) => (
          <a
            key={lecture.id}
            className="panel subject-lecture-card"
            href={`#${lecturePath(currentSubjectCode, lecture.id)}`}
          >
            <div className="section-header">
              <span className="eyebrow">
                {lecture.code.replace(/^LEC\s*/i, "Lecture ")} · {lecture.date}
              </span>
              {progress.includes(lecture.id) && (
                <Badge tone="success">Completed</Badge>
              )}
            </div>
            <h2>{lecture.title}</h2>
            <p className="muted">{lecture.summary}</p>
            <div className="lecture-content-counts">
              <span>
                <PlayCircle size={14} />
                {lecture.recordingUrl ? "Video" : "Video pending"}
              </span>
              <span>
                <FileText size={14} />
                Notes
              </span>
              <span>
                <ClipboardList size={14} />
                {lecture.assignments.length} assignments
              </span>
            </div>
            <div className="subject-lecture-footer">
              <span>Open lecture {i + 1}</span>
              <ArrowUpRight size={16} />
            </div>
          </a>
        ))}
      </div>
      {!filtered.length && (
        <EmptyState
          title={query ? "No matching lectures" : "No lectures yet"}
          description={
            query
              ? "Try another topic."
              : "Your teacher will publish lectures here."
          }
        />
      )}
    </section>
  );
}
