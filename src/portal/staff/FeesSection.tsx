import { Field, ActionForm } from "./Forms";
import type { StaffData } from "./types";
import type { ReactNode } from "react";
export function FeesSection({
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
        title="Issue local invoice"
        endpoint="/staff/fees"
        onDone={reload}
      >
        {studentSelect}
        <Field label="Description" name="description" />
        <Field label="Semester" name="semester" type="number" min={1} max={3} />
        <Field label="Amount (INR)" name="amount" type="number" min={1} />
        <Field label="Due date" name="dueDate" type="date" />
      </ActionForm>
      <section className="panel panel-padding">
        <h2>Student ledger</h2>
        <p className="muted">Local simulation · No real money collected.</p>
        {data.fees.map((f) => (
          <div className="notice-row" key={f.id}>
            <div>
              <h3>
                {
                  data.users
                    .filter((u) => u.role === "student")
                    .find((s) => s.id === f.userId)?.name
                }
              </h3>
              <p>
                {f.description} · ₹{f.amount.toLocaleString("en-IN")} ·{" "}
                {f.status}
              </p>
            </div>
          </div>
        ))}
      </section>
    </div>
  );
}
