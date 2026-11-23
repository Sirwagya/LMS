import { Field, CourseSelect, ActionForm } from "./Forms";
import { courses } from "./selectors";
import type { StaffData } from "./types";
import type { ReactNode } from "react";
export function AttendanceSection({
  data,
  reload,
  studentSelect,
}: {
  data: StaffData;
  reload: () => Promise<void>;
  studentSelect: ReactNode;
}) {
  return (
    <div className="staff-grid">
      <ActionForm
        title="Record course attendance"
        endpoint="/staff/attendance"
        method="PUT"
        onDone={reload}
      >
        {studentSelect}
        <CourseSelect />
        <Field
          label="Classes attended"
          name="attended"
          type="number"
          min={0}
          max={1000}
        />
        <Field
          label="Total classes"
          name="total"
          type="number"
          min={0}
          max={1000}
        />
        <p className="muted">
          Enter cumulative class counts. These replace the previous totals.
        </p>
      </ActionForm>
      <section className="panel panel-padding">
        <h2>Recorded attendance</h2>
        {data.attendance.map((a) => (
          <div key={a.user_id + a.course} className="notice-row">
            <div>
              <h3>
                {
                  data.users
                    .filter((u) => u.role === "student")
                    .find((s) => s.id === a.user_id)?.name
                }
              </h3>
              <p>{courses().find((c) => c.code === a.course)?.name}</p>
              <p>
                {a.attended}/{a.total} classes ·{" "}
                {a.total ? Math.round((a.attended / a.total) * 100) : 0}%
              </p>
            </div>
          </div>
        ))}
      </section>
    </div>
  );
}
