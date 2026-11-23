import { semesterSubjects } from "../../data/lmsData";
import { Field, ActionForm } from "./Forms";
export function CoursesSection({ reload }: { reload: () => Promise<void> }) {
  return (
    <div className="staff-grid">
      <ActionForm
        title="Create course"
        endpoint="/staff/courses"
        onDone={reload}
      >
        <Field label="Course identifier" name="code" />
        <Field label="Course name" name="name" />
        <Field label="Instructor" name="instructor" />
        <Field label="Semester" name="semester" type="number" min={1} max={3} />
        <Field label="Credits" name="credits" type="number" min={1} max={10} />
      </ActionForm>
      <section className="panel panel-padding">
        <h2>Course catalog</h2>
        {Object.entries(semesterSubjects).map(([s, cs]) => (
          <div key={s}>
            <h3 className="mt-4">Semester {s}</h3>
            {cs.map((c) => (
              <p className="notice-row" key={c.code}>
                {c.name} · {c.instructor}
              </p>
            ))}
          </div>
        ))}
      </section>
    </div>
  );
}
