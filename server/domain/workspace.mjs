import { publicAssignments } from "../coding/catalog.mjs";
import { publicUser } from "./passwords.mjs";
export function createWorkspace(
  { attendance, submissions: submissionRepository, fees, progress },
  { academic, assignment },
) {
  function bootstrap(u) {
    const subjects = structuredClone(academic.semesterSubjects);
    if (u.role === "student")
      for (const s of Object.values(subjects).flat()) {
        const a = attendance.forCourse(u.id, s.code);
        s.attendedClasses = a?.attended || 0;
        s.totalClasses = a?.total || 0;
      }
    const submissions = submissionRepository.forStudent(u.id);
    const summaries =
      u.email === "student@vedam.local"
        ? structuredClone(academic.contestSummary)
        : {};
    for (const c of Object.keys(academic.lecturesBySubject)) {
      const graded = submissions.filter((s) => s.course === c && s.score);
      if (graded.length) {
        const history = graded.map((s, i) => {
          const [a, b] = s.score.split("/").map(Number);
          return {
            label: `A${i + 1}`,
            title: assignment(s.assignment_id)?.title || "Assignment",
            date: s.created.slice(0, 10),
            score: s.score,
            percent: Math.round((a / b) * 100),
            rank: "—",
          };
        });
        summaries[c] = {
          percent: Math.round(
            history.reduce((n, a) => n + a.percent, 0) / history.length,
          ),
          detail: `${history.length} graded assignments`,
          history,
        };
      }
    }
    const visible = publicAssignments(academic, u.role !== "student");
    if (u.role === "student" && u.email !== "student@vedam.local")
      for (const lectures of Object.values(visible.lecturesBySubject))
        for (const l of lectures) l.attendance = "Awaiting";
    return {
      ...visible,
      contestSummary: summaries,
      recentContests:
        u.email === "student@vedam.local" ? academic.recentContests : [],
      semesterSubjects: subjects,
      user: publicUser(u),
      fees: fees.forStudent(u.id).map((f) => JSON.parse(f.data)),
      submissions,
      progress: progress.forStudent(u.id).map((p) => p.lecture_id),
    };
  }

  return bootstrap;
}
