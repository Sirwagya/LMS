import { drawerNotices } from "../../data/lmsData";
import { Field, ActionForm } from "./Forms";
export function NoticesSection({ reload }: { reload: () => Promise<void> }) {
  return (
    <div className="staff-grid">
      <ActionForm
        title="Publish announcement"
        endpoint="/staff/notices"
        onDone={reload}
      >
        <Field label="Title" name="title" />
        <label>
          Message
          <textarea name="desc" rows={5} required />
        </label>
        <label>
          Priority
          <select name="tag">
            <option value="ACADEMIC">Academic</option>
            <option value="URGENT">Urgent</option>
          </select>
        </label>
      </ActionForm>
      <section className="panel panel-padding">
        <h2>Notice board</h2>
        {drawerNotices.map((n) => (
          <article className="drawer-notice" key={n.id}>
            <h3>{n.title}</h3>
            <p>{n.desc}</p>
            <small className="muted">{n.time}</small>
          </article>
        ))}
      </section>
    </div>
  );
}
