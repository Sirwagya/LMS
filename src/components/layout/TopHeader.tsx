import { useEffect, useState } from "react";
import { LearningBreadcrumb } from "./LearningBreadcrumb";
import { Bell, Sun, Moon, RefreshCw } from "lucide-react";
import { useLMS } from "../../context/useLMS";
import { transport, type SyncStatus } from "../../portal/api";
import { Navigation } from "./Navigation";

function SyncStatusPill() {
  const [status, setStatus] = useState<SyncStatus>({
    state: typeof navigator !== "undefined" && !navigator.onLine ? "offline" : "synced",
    pendingCount: 0,
    conflictCount: 0,
    lastSyncedAt: null,
  });
  const [syncingNow, setSyncingNow] = useState(false);

  useEffect(() => {
    let active = true;
    transport
      .getSyncStatus()
      .then((snapshot) => {
        if (active) setStatus(snapshot);
      })
      .catch(() => {});

    const unsubscribe = transport.subscribeEvents((eventName, payload) => {
      if (!active) return;
      if (eventName === "sync-updated" && payload && typeof payload === "object") {
        setStatus(payload as SyncStatus);
      } else if (eventName === "sync.available") {
        transport
          .getSyncStatus()
          .then((snapshot) => {
            if (active) setStatus(snapshot);
          })
          .catch(() => {});
      }
    });

    return () => {
      active = false;
      unsubscribe();
    };
  }, []);

  const handleTrigger = async () => {
    if (syncingNow) return;
    setSyncingNow(true);
    try {
      const next = await transport.triggerSyncNow();
      setStatus(next);
    } catch {
      /* Preserve latest known state if manual sync hits transient failure. */
    } finally {
      setSyncingNow(false);
    }
  };

  const hasConflicts = status.conflictCount > 0;
  const effectiveState = syncingNow ? "syncing" : status.state;
  const toneClass = hasConflicts
    ? "is-warning"
    : effectiveState === "offline" || effectiveState === "error"
      ? "is-warning"
      : effectiveState === "syncing"
        ? "is-syncing"
        : "is-synced";

  const label = hasConflicts
    ? `Conflict · ${status.conflictCount}`
    : effectiveState === "syncing"
      ? "Syncing…"
      : effectiveState === "offline"
        ? status.pendingCount > 0
          ? `Offline · ${status.pendingCount} queued`
          : "Offline"
        : effectiveState === "error"
          ? "Sync issue"
          : status.pendingCount > 0
            ? `Syncing · ${status.pendingCount}`
            : "Synced";

  const title = status.lastSyncedAt
    ? `Last synced ${new Date(status.lastSyncedAt).toLocaleTimeString()} · Click to sync now`
    : "Click to sync workspace";

  return (
    <button
      type="button"
      className={`sync-pill ${toneClass}`}
      onClick={handleTrigger}
      disabled={syncingNow}
      title={title}
      aria-label={`Sync status: ${label}. Click to sync now.`}
    >
      <span className="sync-pill-dot" aria-hidden="true" />
      <span>{label}</span>
      {effectiveState === "syncing" && (
        <RefreshCw className="sync-pill-spin" aria-hidden="true" />
      )}
    </button>
  );
}

export function TopHeader() {
  const {
    currentSemester,
    setSemester,
    theme,
    toggleTheme,
    setIsNoticesDrawerOpen,
    route,
  } = useLMS();
  const label = route.startsWith("/coding/")
    ? "Coding challenge"
    : route.startsWith("/subjects/")
      ? "Coursework"
      : ({
          "/dashboard": "Dashboard",
          "/overview": "Attendance & results",
          "/assignments": "Assignments & progress",
          "/schedule": "Timetable",
          "/fees": "Fees",
          "/account": "Account",
        }[route] ?? "Workspace");
  return (
    <header className="top-header" data-tauri-drag-region>
      <div className="header-primary" data-tauri-drag-region>
        <a href="#/dashboard" className="brand" aria-label="Vedam dashboard">
          <span className="brand-mark" aria-hidden="true">
            v.
          </span>
          <span className="brand-name">vedam</span>
        </a>
        <Navigation />
      </div>
      <div className="header-secondary">
        <LearningBreadcrumb fallback={label} />
        <div className="header-tools">
          <SyncStatusPill />
          <label className="sr-only" htmlFor="semester">
            Academic semester
          </label>
          <select
            className="semester-select"
            id="semester"
            value={currentSemester}
            onChange={(e) => setSemester(Number(e.target.value))}
          >
            {[1, 2, 3].map((sem) => (
              <option key={sem} value={sem}>
                Semester {sem}
                {sem < 3 ? " · Archived" : ""}
              </option>
            ))}
          </select>
          <button
            className="icon-button"
            onClick={() => setIsNoticesDrawerOpen(true)}
            aria-label="Open notices"
          >
            <Bell />
          </button>
          <button
            className="icon-button"
            onClick={toggleTheme}
            aria-label={
              theme === "dark" ? "Switch to light mode" : "Switch to dark mode"
            }
          >
            {theme === "dark" ? <Sun /> : <Moon />}
          </button>
        </div>
      </div>
    </header>
  );
}
