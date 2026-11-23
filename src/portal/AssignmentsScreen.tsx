import { assignmentPath } from "../utils/learningRoutes";
import { useState } from "react";
import { lecturesBySubject } from "../data/lmsData";
import { useLMS } from "../context/useLMS";
import { useSession } from "./session";
import { PageHeader, Badge, EmptyState } from "../components/ui/Workspace";
import { AssignmentSubmission } from "./LearningTools";
export function AssignmentsScreen() {
  const { allSubjects } = useLMS(),
    { submissions, progress } = useSession(),
    [filter, setFilter] = useState("All");
  const assignments = allSubjects.flatMap((c) =>
    (lecturesBySubject[c.code] || []).flatMap((l) =>
      l.assignments.map((a) => ({
        assignment: a,
        lecture: l,
        course: c,
        submission: submissions.find((s) => s.assignment_id === String(a.id)),
      })),
    ),
  );
  const filtered = assignments.filter(
    (a) =>
      filter === "All" ||
      (filter === "Pending"
        ? !a.submission
        : filter === "Graded"
          ? !!a.submission?.score
          : !!a.submission && !a.submission.score),
  );
  const lessons = allSubjects.flatMap((c) => lecturesBySubject[c.code] || []);
  return (
    <section className="screen">
      <PageHeader
        title="Assignments & progress"
        description="Your work, feedback and learning progress for this semester."
      />
      <p className="inline-note">
        {lessons.filter((l) => progress.includes(l.id)).length} of{" "}
        {lessons.length} lectures completed ·{" "}
        {assignments.filter((a) => !!a.submission).length} of{" "}
        {assignments.length} assignments submitted.
      </p>
      <div className="segmented" aria-label="Filter assignments">
        {["All", "Pending", "Submitted", "Graded"].map((f) => (
          <button
            key={f}
            aria-pressed={filter === f}
            onClick={() => setFilter(f)}
          >
            {f}
          </button>
        ))}
      </div>
      {filtered.length ? (
        filtered.map(
          ({ assignment: a, lecture: l, course: c, submission: s }) => (
            <article key={a.id} className="panel panel-padding">
              <div className="section-header">
                <div>
                  <p className="eyebrow">{c.name}</p>
                  <h2>{a.title}</h2>
                  <p className="muted">Due {a.dueDate || "No deadline"}</p>
                </div>
                <Badge tone={s ? "success" : "warning"}>
                  {s?.score
                    ? `Graded · ${s.score}`
                    : s
                      ? "Submitted"
                      : "Pending"}
                </Badge>
              </div>
              <a
                className="text-link"
                href={`#/subjects/${c.code}/lectures/${l.id}`}
              >
                Open lecture notes →
              </a>
              {a.kind === "coding" ? (
                <AssignmentSubmission key={a.id} assignment={a} />
              ) : (
                <a
                  className="btn btn-primary mt-4"
                  href={`#${assignmentPath(c.code, l.id, a.id)}`}
                >
                  Open assignment
                </a>
              )}
            </article>
          ),
        )
      ) : (
        <EmptyState
          title="No assignments in this view"
          description="Change the filter or check another semester."
        />
      )}
    </section>
  );
}
