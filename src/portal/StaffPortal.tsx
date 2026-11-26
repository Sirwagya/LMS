import { ContestAuthoring } from "./contests/ContestAuthoring";
import { useContestsEnabled } from "./contests/useContestsEnabled";
import { useCallback, useEffect, useRef, useState } from "react";
import { lecturesBySubject, drawerNotices } from "../data/lmsData";
import { api } from "./api";
import { useSession } from "./session";
import type { StaffData, StaffRecords } from "./staff/types";
import { ActionForm, Field } from "./staff/Forms";
import { courses } from "./staff/selectors";
import { DiscussionsSection } from "./staff/DiscussionsSection";
import { SettingsSection } from "./staff/SettingsSection";
import { OverviewSection } from "./staff/OverviewSection";
import { CoursesSection } from "./staff/CoursesSection";
import { LecturesSection } from "./staff/LecturesSection";
import { AssignmentsSection } from "./staff/AssignmentsSection";
import { SubmissionsSection } from "./staff/SubmissionsSection";
import { AttendanceSection } from "./staff/AttendanceSection";
import { TimetableSection } from "./staff/TimetableSection";
import { NoticesSection } from "./staff/NoticesSection";
import { FeesSection } from "./staff/FeesSection";
import { PeopleSection } from "./staff/PeopleSection";
export function StaffPortal() {
  const { user, logout, refresh } = useSession();
  const contestsEnabled = useContestsEnabled();
  const [tab, setTab] = useState("Overview"),
    [data, setData] = useState<StaffData | null>(null),
    [error, setError] = useState("");
  useEffect(() => {
    document.title = `${tab} · ${user.role === "admin" ? "Admin" : "Teacher"} · Vedam`;
  }, [tab, user.role]);
  const [pages, setPages] = useState<Record<StaffRecords, number>>({
    users: 0, submissions: 0, attendance: 0, fees: 0,
  });
  const [loading, setLoading] = useState(true);
  const requestID = useRef(0);
  const loadPage = useCallback(async (next: Record<StaffRecords, number>) => {
    const id = ++requestID.current;
    const query = new URLSearchParams(
      Object.entries(next).map(([kind, page]) => [kind + "Page", String(page)]),
    );
    setLoading(true);
    try {
      const result = await api<StaffData>(`/staff?${query}`);
      if (requestID.current === id) {
        setData(result);
        setPages(next);
        setError("");
      }
    } finally {
      if (requestID.current === id) setLoading(false);
    }
  }, []);
  const reload = async () => {
    await refresh();
    await loadPage(pages);
  };
  useEffect(() => {
    void loadPage({ users: 0, submissions: 0, attendance: 0, fees: 0 })
      .catch((e) => setError(e.message));
    return () => { requestID.current += 1; };
  }, [loadPage]);
  const pagination = (kind: StaffRecords, label: string) => {
    const page = pages[kind];
    if (page === 0 && !data?.hasMore[kind]) return null;
    const changePage = (target: number) => {
      void loadPage({ ...pages, [kind]: target }).catch((e) => setError(e.message));
    };
    return (
      <nav className="inline-actions" aria-label={label}>
        <button className="btn btn-secondary" type="button" disabled={loading || page === 0}
          onClick={() => changePage(page - 1)}>Previous</button>
        <span>{label} · Page {page + 1}</span>
        <button className="btn btn-secondary" type="button" disabled={loading || !data?.hasMore[kind] || page >= 10000}
          onClick={() => changePage(page + 1)}>Next</button>
      </nav>
    );
  };
  const [discussionId, setDiscussionId] = useState<number>(
    Object.values(lecturesBySubject).flat()[0]?.id || 0,
  );
  const students = data?.users.filter((u) => u.role === "student") || [];
  const studentSelect = (
    <>
    <label>
      Student
      <select name="userId" required>
        <option value="">Choose a student</option>
        {students.map((s) => (
          <option value={s.id} key={s.id}>
            {s.name} · {s.email}
          </option>
        ))}
      </select>
    </label>
    {pagination("users", "Student choices")}
    </>
  );
  const tabs = [
    "Overview",
    "Courses",
    "Lectures",
    "Assignments",
    ...(contestsEnabled ? ["Contests"] : []),
    "Submissions",
    "Discussions",
    "Attendance",
    "Timetable",
    "Notices",
    ...(user.role === "admin" ? ["Fees", "People"] : []),
    "Settings",
  ];
  return (
    <div className="app-shell">
      <header className="top-header" data-tauri-drag-region>
        <div className="header-primary" data-tauri-drag-region>
          <a href="#" className="brand">
            <span className="brand-name">vedam</span>
            <span className="badge">
              {user.role === "admin" ? "Admin" : "Teacher"}
            </span>
          </a>
          <div className="inline-actions">
            <span className="muted">{user.name}</span>
            <button
              className="btn btn-secondary"
              onClick={() => document.documentElement.classList.toggle("dark")}
            >
              Toggle theme
            </button>
            <button
              className="btn btn-secondary"
              onClick={() => logout().catch((e) => setError(e.message))}
            >
              Sign out
            </button>
          </div>
        </div>
        <nav className="main-nav staff-nav" aria-label="Staff navigation">
          {tabs.map((t) => (
            <button
              key={t}
              className="nav-item"
              aria-current={tab === t ? "page" : undefined}
              onClick={() => setTab(t)}
            >
              {t}
            </button>
          ))}
        </nav>
      </header>
      <main className="workspace-body">
        <div className="page-header">
          <div>
            <p className="eyebrow">STAFF WORKSPACE</p>
            <h1>{tab}</h1>
            <p className="page-description">
              Manage learning, support students, and keep academic records up to
              date.
            </p>
          </div>
          <button
            className="btn btn-secondary"
            disabled={loading}
            onClick={() => reload().catch((e) => setError(e.message))}
          >
            Refresh records
          </button>
        </div>
        {error && <p role="alert">{error}</p>}
        {tab === "Contests" ? <ContestAuthoring /> : !data ? (
          <p role="status">Loading records…</p>
        ) : (
          <div className="screen staff-content">
            {tab === "Discussions" && (
              <DiscussionsSection
                discussionId={discussionId}
                setDiscussionId={setDiscussionId}
              />
            )}
            {tab === "Settings" && <SettingsSection />}
            {["Courses", "Lectures", "Notices"].includes(tab) && (
              <ActionForm
                title={`Edit ${tab === "Courses" ? "course" : tab === "Lectures" ? "lecture" : "notice"}`}
                endpoint="/staff/catalog"
                method="PUT"
                onDone={reload}
              >
                <input
                  type="hidden"
                  name="kind"
                  value={
                    tab === "Courses"
                      ? "course"
                      : tab === "Lectures"
                        ? "lecture"
                        : "notice"
                  }
                />
                <label>
                  Record
                  <select name="id" required>
                    {(tab === "Courses"
                      ? courses().map((c) => ({ id: c.code, title: c.name }))
                      : tab === "Lectures"
                        ? Object.values(lecturesBySubject)
                            .flat()
                            .map((l) => ({ id: String(l.id), title: l.title }))
                        : drawerNotices.map((n) => ({
                            id: n.id || "",
                            title: n.title,
                          }))
                    ).map((r) => (
                      <option key={r.id} value={r.id}>
                        {r.title}
                      </option>
                    ))}
                  </select>
                </label>
                <Field label="New title / name" name="title" />
                <label>
                  {tab === "Courses" ? "Instructor" : "Updated notes / message"}
                  <textarea name="description" rows={4} required />
                </label>
              </ActionForm>
            )}
            {tab === "Overview" && <OverviewSection data={data} />}
            {tab === "Courses" && <CoursesSection reload={reload} />}
            {tab === "Lectures" && <LecturesSection reload={reload} />}
            {tab === "Assignments" && <AssignmentsSection reload={reload} />}
            {tab === "Submissions" && (<>
              <SubmissionsSection data={data} reload={reload} />
              {pagination("submissions", "Submissions")}
            </>)}
            {tab === "Attendance" && (
              <AttendanceSection
                data={data}
                reload={reload}
                studentSelect={studentSelect}
              />
            )}
            {tab === "Timetable" && <TimetableSection reload={reload} />}
            {tab === "Notices" && <NoticesSection reload={reload} />}
            {tab === "Fees" && (
              <FeesSection
                data={data}
                reload={reload}
                studentSelect={studentSelect}
              />
            )}
            {tab === "People" && <PeopleSection data={data} reload={reload} />}
            {tab === "Attendance" && pagination("attendance", "Attendance records")}
            {tab === "Fees" && pagination("fees", "Invoices")}
            {tab === "People" && pagination("users", "People")}
            {tab === "Overview" && pagination("users", "People")}
            {tab === "Overview" && pagination("submissions", "Submissions")}
          </div>
        )}
      </main>
    </div>
  );
}
