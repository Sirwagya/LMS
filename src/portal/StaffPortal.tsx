import { useEffect, useState } from "react";
import { lecturesBySubject, drawerNotices } from "../data/lmsData";
import { api } from "./api";
import { useSession } from "./session";
import type { StaffData } from "./staff/types";
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
  const [tab, setTab] = useState("Overview"),
    [data, setData] = useState<StaffData | null>(null),
    [error, setError] = useState("");
  useEffect(() => {
    document.title = `${tab} · ${user.role === "admin" ? "Admin" : "Teacher"} · Vedam`;
  }, [tab, user.role]);
  const reload = async () => {
    await refresh();
    setData(await api<StaffData>("/staff"));
  };
  useEffect(() => {
    api<StaffData>("/staff")
      .then(setData)
      .catch((e) => setError(e.message));
  }, []);
  const [discussionId, setDiscussionId] = useState<number>(
    Object.values(lecturesBySubject).flat()[0]?.id || 0,
  );
  const students = data?.users.filter((u) => u.role === "student") || [];
  const studentSelect = (
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
  );
  const tabs = [
    "Overview",
    "Courses",
    "Lectures",
    "Assignments",
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
      <header className="top-header">
        <div className="header-primary">
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
            onClick={() => reload().catch((e) => setError(e.message))}
          >
            Refresh records
          </button>
        </div>
        {error && <p role="alert">{error}</p>}
        {!data ? (
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
            {tab === "Submissions" && (
              <SubmissionsSection data={data} reload={reload} />
            )}
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
          </div>
        )}
      </main>
    </div>
  );
}
