import { lecturesBySubject } from "../../data/lmsData";
import { Field, ActionForm } from "./Forms";
import { courses } from "./selectors";
import type { StaffData } from "./types";
export function SubmissionsSection({
  data,
  reload,
}: {
  data: StaffData;
  reload: () => Promise<void>;
}) {
  return (
    <>
      {!data.submissions.length && (
        <p className="inline-note">
          No student submissions yet. Sign in as a student, open a course, and
          submit an assignment.
        </p>
      )}
      {data.submissions.map((s) => (
        <section className="panel panel-padding" key={s.id}>
          <h2>{s.name}</h2>
          <h3 className="mt-3">
            {
              Object.values(lecturesBySubject)
                .flat()
                .flatMap((l) => l.assignments)
                .find((a) => String(a.id) === s.assignment_id)?.title
            }
          </h3>
          <p className="muted">
            {courses().find((c) => c.code === s.course)?.name} ·{" "}
            {new Date(s.created).toLocaleString()}
          </p>
          <p className="submission-answer">{s.answer}</p>
          {s.filename && (
            <a href={`/api/submission-file/${s.id}`} className="text-link">
              Download {s.filename}
            </a>
          )}
          <ActionForm
            title={s.score ? "Update grade" : "Grade submission"}
            endpoint="/staff/grade"
            method="PUT"
            onDone={reload}
          >
            <input type="hidden" name="id" value={s.id} />
            <Field
              label="Score (earned / maximum)"
              name="score"
              defaultValue={s.score || ""}
            />
            <label>
              Feedback
              <textarea
                name="feedback"
                rows={3}
                defaultValue={s.feedback || ""}
              />
            </label>
          </ActionForm>
        </section>
      ))}
    </>
  );
}
