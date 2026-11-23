import { courses } from "./selectors";
import type { StaffData } from "./types";
export function OverviewSection({ data }: { data: StaffData }) {
  return (
    <>
      <dl className="metrics">
        <div className="metric">
          <dt>Students</dt>
          <dd>{data.users.filter((u) => u.role === "student").length}</dd>
        </div>
        <div className="metric">
          <dt>Courses</dt>
          <dd>{courses().length}</dd>
        </div>
        <div className="metric">
          <dt>Awaiting grading</dt>
          <dd>{data.submissions.filter((s) => !s.score).length}</dd>
        </div>
      </dl>
      <section className="panel panel-padding">
        <h2>Your teaching workspace</h2>
        <p className="muted">
          Publish courses and lecture materials, set assignments, review
          submissions and record attendance. Students see updates in their
          workspace.
        </p>
        <p className="inline-note mt-4">
          Local sample data · Files and records stay in this machine's SQLite
          database. Fee payments are simulations.
        </p>
      </section>
    </>
  );
}
