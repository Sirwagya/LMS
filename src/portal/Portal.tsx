import { useEffect, useState, type FormEvent } from "react";
import App from "../App";
import { hydrateAcademic } from "../data/lmsData";
import { api } from "./api";
import { SessionContext, type PortalUser, type Submission } from "./session";
import { StaffPortal } from "./StaffPortal";

type Bootstrap = Parameters<typeof hydrateAcademic>[0] & {
  user: PortalUser;
  submissions: Submission[];
  progress: number[];
};
export default function Portal() {
  const [data, setData] = useState<Bootstrap | null>(null),
    [loading, setLoading] = useState(true),
    [error, setError] = useState(""),
    [mode, setMode] = useState<"login" | "register">("login"),
    [busy, setBusy] = useState(false);
  const refresh = async () => {
    const b = await api<Bootstrap>("/bootstrap");
    hydrateAcademic(b);
    setData(b);
  };
  useEffect(() => {
    api<Bootstrap>("/bootstrap")
      .then((b) => {
        hydrateAcademic(b);
        setData(b);
      })
      .catch((e) => {
        if (e.message !== "Please sign in.") setError(e.message);
      })
      .finally(() => setLoading(false));
  }, []);
  useEffect(() => {
    const expire = () => {
      setData(null);
      setError("Your session ended. Please sign in again.");
    };
    window.addEventListener("lms:session-expired", expire);
    return () => window.removeEventListener("lms:session-expired", expire);
  }, []);
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    setError("");
    const f = new FormData(e.currentTarget);
    try {
      await api(`/${mode}`, "POST", Object.fromEntries(f));
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  if (loading)
    return (
      <main className="auth-shell">
        <h1>Opening your workspace…</h1>
      </main>
    );
  if (!data)
    return (
      <main className="auth-shell">
        <div className="auth-intro">
          <span className="eyebrow">VEDAM · LOCAL LMS</span>
          <h1>
            A place to learn.
            <br />
            Room to grow.
          </h1>
          <p>Your courses, assignments and academic life, together.</p>
        </div>
        <section className="panel panel-padding auth-card">
          <h2>
            {mode === "login" ? "Welcome back" : "Create a student account"}
          </h2>
          <p className="muted">
            Students and staff sign in here. Your role opens the right
            workspace.
          </p>
          <form className="portal-form" onSubmit={submit}>
            {mode === "register" && (
              <label>
                Full name
                <input
                  name="name"
                  autoComplete="name"
                  required
                  maxLength={100}
                />
              </label>
            )}
            <label>
              Email
              <input
                name="email"
                type="email"
                autoComplete="username"
                required
              />
            </label>
            <label>
              Password
              <input
                name="password"
                type="password"
                autoComplete={
                  mode === "login" ? "current-password" : "new-password"
                }
                minLength={10}
                maxLength={128}
                required
              />
            </label>
            {error && (
              <p role="alert" className="inline-note">
                {error}
              </p>
            )}
            <button className="btn btn-primary" disabled={busy}>
              {busy
                ? "Please wait…"
                : mode === "login"
                  ? "Sign in"
                  : "Create account"}
            </button>
          </form>
          <button
            className="text-link"
            onClick={() => {
              setMode(mode === "login" ? "register" : "login");
              setError("");
            }}
          >
            {mode === "login"
              ? "New student? Create account"
              : "Already registered? Sign in"}
          </button>
          <details className="demo-access">
            <summary>Local demo accounts</summary>
            <p>
              Student: student@vedam.local
              <br />
              Teacher: teacher@vedam.local
              <br />
              Admin: admin@vedam.local
              <br />
              Password for all: <code>VedamLocal123!</code>
            </p>
            <p className="muted">
              Sample academic records. Payments are simulated.
            </p>
          </details>
        </section>
      </main>
    );
  return (
    <SessionContext.Provider
      value={{
        user: data.user,
        submissions: data.submissions,
        progress: data.progress,
        refresh,
        logout: async () => {
          await api("/logout", "POST", {});
          setData(null);
        },
      }}
    >
      {data.user.role === "student" ? (
        <App key={data.user.id} />
      ) : (
        <StaffPortal />
      )}
    </SessionContext.Provider>
  );
}
