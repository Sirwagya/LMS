import { Field, ActionForm } from "./Forms";
import type { StaffData } from "./types";
export function PeopleSection({
  data,
  reload,
}: {
  data: StaffData;
  reload: () => Promise<void>;
}) {
  return (
    <div className="staff-grid">
      <ActionForm
        title="Add student or teacher"
        endpoint="/staff/users"
        onDone={reload}
      >
        <Field label="Full name" name="name" />
        <Field label="Email" name="email" type="email" />
        <Field
          label="Initial password (10+ characters)"
          name="password"
          type="password"
        />
        <label>
          Role
          <select name="role">
            <option value="student">Student</option>
            <option value="teacher">Teacher</option>
          </select>
        </label>
      </ActionForm>
      <section className="panel panel-padding">
        <h2>People</h2>
        {data.users.map((p) => (
          <div className="notice-row" key={p.id}>
            <div>
              <h3>{p.name}</h3>
              <p>
                {p.email} · {p.role}
              </p>
            </div>
          </div>
        ))}
      </section>
    </div>
  );
}
