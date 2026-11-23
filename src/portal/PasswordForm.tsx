import { useState, type FormEvent } from "react";
import { api } from "./api";
export function PasswordForm() {
  const [message, setMessage] = useState(""),
    [busy, setBusy] = useState(false);
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    setBusy(true);
    try {
      const f = new FormData(form);
      if (f.get("next") !== f.get("confirm"))
        throw new Error("New passwords must match.");
      await api("/password", "POST", Object.fromEntries(f));
      form.reset();
      setMessage("Password changed. Other sessions have been signed out.");
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="panel panel-padding">
      <h2>Change password</h2>
      <form className="portal-form" onSubmit={submit}>
        <label>
          Current password
          <input
            name="current"
            type="password"
            autoComplete="current-password"
            required
            maxLength={128}
          />
        </label>
        <label>
          New password
          <input
            name="next"
            type="password"
            autoComplete="new-password"
            minLength={10}
            maxLength={128}
            required
          />
        </label>
        <label>
          Confirm new password
          <input
            name="confirm"
            type="password"
            autoComplete="new-password"
            minLength={10}
            maxLength={128}
            required
          />
        </label>
        <button className="btn btn-secondary" disabled={busy}>
          Change password
        </button>
        <p role="status">{message}</p>
      </form>
    </section>
  );
}
