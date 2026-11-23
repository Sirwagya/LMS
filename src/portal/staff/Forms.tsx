import { courses } from "./selectors";
import { useState, type FormEvent, type ReactNode } from "react";
import { lecturesBySubject } from "../../data/lmsData";
import { api } from "../api";
export function Field({
  label,
  name,
  type = "text",
  required = true,
  ...props
}: {
  label: string;
  name: string;
  type?: string;
  required?: boolean;
  min?: number;
  max?: number;
  defaultValue?: string | number;
}) {
  return (
    <label>
      {label}
      <input name={name} type={type} required={required} {...props} />
    </label>
  );
}
export function CourseSelect() {
  return (
    <label>
      Course
      <select name="course" required>
        {courses().map((c) => (
          <option value={c.code} key={c.code}>
            {c.name}
          </option>
        ))}
      </select>
    </label>
  );
}
export function LectureSelect() {
  return (
    <label>
      Lecture
      <select name="lectureId" required>
        {Object.entries(lecturesBySubject).flatMap(([code, ls]) =>
          ls.map((l) => (
            <option key={l.id} value={l.id}>
              {courses().find((c) => c.code === code)?.name} · {l.title}
            </option>
          )),
        )}
      </select>
    </label>
  );
}
export function ActionForm({
  title,
  endpoint,
  method = "POST",
  children,
  onDone,
  transform,
}: {
  title: string;
  endpoint: string;
  method?: string;
  children: ReactNode;
  onDone: () => Promise<void>;
  transform?: (
    data: Record<string, unknown>,
    form: HTMLFormElement,
  ) => Promise<Record<string, unknown>>;
}) {
  const [busy, setBusy] = useState(false),
    [message, setMessage] = useState("");
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    setBusy(true);
    setMessage("");
    try {
      const d: Record<string, unknown> = {};
      for (const [k, v] of new FormData(form)) {
        if (v instanceof File) continue;
        d[k] = [
          "semester",
          "credits",
          "lectureId",
          "amount",
          "total",
          "attended",
        ].includes(k)
          ? Number(v)
          : v;
      }
      await api(endpoint, method, transform ? await transform(d, form) : d);
      await onDone();
      setMessage("Saved successfully.");
      form.reset();
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="panel panel-padding">
      <h2>{title}</h2>
      <form className="portal-form" onSubmit={submit}>
        {children}
        <p role="status">{message}</p>
        <button className="btn btn-primary" disabled={busy}>
          {busy ? "Saving…" : "Save"}
        </button>
      </form>
    </section>
  );
}
