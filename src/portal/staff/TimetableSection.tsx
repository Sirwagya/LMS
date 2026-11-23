import { demoWeek } from "../../data/lmsData";
import { Field, CourseSelect, ActionForm } from "./Forms";
export function TimetableSection({ reload }: { reload: () => Promise<void> }) {
  return (
    <div className="staff-grid">
      <ActionForm
        title="Schedule a class"
        endpoint="/staff/schedule"
        onDone={reload}
      >
        <CourseSelect />
        <Field label="Date" name="date" type="date" />
        <Field label="Start time" name="from" type="time" />
        <Field label="End time" name="to" type="time" />
        <Field label="Room / meeting details" name="room" />
        <Field label="Topic" name="topic" />
      </ActionForm>
      <section className="panel panel-padding">
        <h2>Published classes</h2>
        {Object.entries(demoWeek.days)
          .sort()
          .map(([day, entries]) => (
            <div key={day}>
              <h3 className="mt-4">{day}</h3>
              {entries.map((e, i) => (
                <p className="notice-row" key={i}>
                  {e.from}–{e.to} · {e.topic} · {e.room}
                </p>
              ))}
            </div>
          ))}
      </section>
    </div>
  );
}
