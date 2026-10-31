import { Bell, Sun, Moon, ChevronRight } from "lucide-react";
import { useLMS } from "../../context/useLMS";
import { Navigation } from "./Navigation";

export function TopHeader() {
  const {
    currentSemester,
    setSemester,
    theme,
    toggleTheme,
    setIsNoticesDrawerOpen,
    route,
  } = useLMS();
  const label = route.startsWith("/subjects/")
    ? "Coursework"
    : ({
        "/dashboard": "Dashboard",
        "/overview": "Attendance & results",
        "/schedule": "Timetable",
        "/fees": "Fees",
        "/account": "Account",
      }[route] ?? "Workspace");
  return (
    <header className="top-header">
      <div className="header-primary">
        <a href="#/dashboard" className="brand" aria-label="Vedam dashboard">
          <span className="brand-mark" aria-hidden="true">
            v.
          </span>
          <span className="brand-name">vedam</span>
        </a>
        <Navigation />
      </div>
      <div className="header-secondary">
        <div className="breadcrumb">
          <span className="breadcrumb-root">Student workspace</span>
          <ChevronRight
            className="breadcrumb-separator"
            size={12}
            aria-hidden="true"
          />
          <strong>{label}</strong>
        </div>
        <div className="header-tools">
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
