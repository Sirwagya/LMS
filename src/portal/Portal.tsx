import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import App from "../App";
import { hydrateAcademic } from "../data/lmsData";
import { api, transport, type SyncStatus } from "./api";
import { SessionContext, type PortalUser, type Submission } from "./session";
import { StaffPortal } from "./StaffPortal";

type Bootstrap = Parameters<typeof hydrateAcademic>[0] & {
  user: PortalUser;
  submissions: Submission[];
  progress: number[];
};

const TAB_SHORTCUTS: Record<string, string> = {
  "1": "#/dashboard",
  "2": "#/overview",
  "3": "#/assignments",
  "4": "#/schedule",
  "5": "#/fees",
  "6": "#/account",
  ",": "#/account",
};

const ASSET_PATH_PREFIXES = [
  "/api/receipts/",
  "/api/notes/",
  "/api/assets/",
  "/api/submission-file/",
];

function filenameForPath(url: string, label: string | null): string | undefined {
  const cleanLabel = label?.trim() || "";
  if (cleanLabel.toLowerCase().startsWith("download ")) {
    const candidate = cleanLabel.slice("download ".length).trim();
    if (candidate && candidate.includes(".")) return candidate;
  }
  if (url.startsWith("/api/receipts/")) {
    const id = url.split("/").pop()?.split("?")[0] || "receipt";
    return `receipt-${id}.html`;
  }
  if (url.startsWith("/api/notes/")) {
    const id = url.split("/").pop()?.split("?")[0] || "notes";
    return `lecture-${id}-notes.md`;
  }
  return undefined;
}

export default function Portal() {
  const [data, setData] = useState<Bootstrap | null>(null),
    [loading, setLoading] = useState(true),
    [error, setError] = useState(""),
    [mode, setMode] = useState<"login" | "register">("login"),
    [busy, setBusy] = useState(false);
  const lastSyncStampRef = useRef<string>("");

  const refresh = useCallback(async () => {
    const b = await api<Bootstrap>("/bootstrap");
    hydrateAcademic(b);
    setData(b);
  }, []);

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

  useEffect(() => {
    if (!data) return;
    return transport.subscribeEvents((eventName, payload) => {
      if (
        eventName === "sync.available" ||
        eventName === "grade.updated" ||
        eventName === "notice.published"
      ) {
        void refresh().catch(() => {});
        return;
      }
      if (eventName === "sync-updated" && payload && typeof payload === "object") {
        const status = payload as SyncStatus;
        if (status.state === "synced" && status.lastSyncedAt) {
          const stamp = `${status.cursor ?? 0}:${status.lastSyncedAt}`;
          if (lastSyncStampRef.current && lastSyncStampRef.current !== stamp) {
            lastSyncStampRef.current = stamp;
            void refresh().catch(() => {});
          } else {
            lastSyncStampRef.current = stamp;
          }
        }
      }
    });
  }, [data, refresh]);

  useEffect(() => {
    if (!transport.isDesktop) return;
    const handleAssetLinkClick = (event: MouseEvent) => {
      const target = event.target as HTMLElement | null;
      const anchor = target?.closest("a");
      if (!anchor) return;
      const href = anchor.getAttribute("href");
      if (!href || !ASSET_PATH_PREFIXES.some((p) => href.startsWith(p))) {
        return;
      }
      event.preventDefault();
      const suggested =
        anchor.getAttribute("download") ||
        filenameForPath(href, anchor.textContent);
      void transport.downloadAsset(href, suggested).catch((err) => {
        setError((err as Error).message);
      });
    };
    document.addEventListener("click", handleAssetLinkClick);
    return () => document.removeEventListener("click", handleAssetLinkClick);
  }, []);

  useEffect(() => {
    if (!data || data.user.role !== "student") return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (!event.metaKey || event.ctrlKey || event.altKey) return;
      if (event.shiftKey && event.key.toLowerCase() === "s") {
        event.preventDefault();
        void transport
          .triggerSyncNow()
          .then(() => refresh())
          .catch(() => {});
        return;
      }
      const nextHash = TAB_SHORTCUTS[event.key];
      if (!event.shiftKey && nextHash) {
        event.preventDefault();
        window.location.hash = nextHash;
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [data, refresh]);

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
        <div className="desktop-drag-strip" data-tauri-drag-region />
        <h1>Opening your workspace…</h1>
      </main>
    );
  if (!data)
    return (
      <main className="auth-shell">
        <div className="desktop-drag-strip" data-tauri-drag-region />
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
