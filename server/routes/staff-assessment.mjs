import { fail, str, num } from "../domain/validation.mjs";
import { send, body } from "../http/response.mjs";
export function createRoutes({ attendance, submissions, course, student }) {
  return [
    {
      match: ({ path, method }) =>
        path === "/api/staff/attendance" && method === "PUT",
      access: "staff",
      handle: async ({ req, res }) => {
        const b = await body(req);
        student(b.userId);
        if (!course(b.course)) fail(404, "Course not found.");
        const total = num(b.total, "total classes", 0, 1000),
          attended = num(b.attended, "attended classes", 0, total);
        if (!Number.isInteger(total) || !Number.isInteger(attended))
          fail(400, "Use whole class counts.");
        attendance.save(b.userId, b.course, attended, total);
        return send(res, 200, { ok: true });
      },
    },
    {
      match: ({ path, method }) =>
        path === "/api/staff/grade" && method === "PUT",
      access: "staff",
      handle: async ({ req, res }) => {
        const b = await body(req);
        const s = submissions.exists(b.id);
        if (!s) fail(404, "Submission not found.");
        const score = str(b.score, "score", 30);
        if (!/^\d+(\.\d+)?\s*\/\s*\d+(\.\d+)?$/.test(score))
          fail(400, "Use earned / maximum, e.g. 18/20.");
        const [earned, max] = score.split("/").map(Number);
        if (max <= 0 || earned > max)
          fail(400, "Score must be between zero and maximum.");
        submissions.grade(score, String(b.feedback || "").slice(0, 5000), b.id);
        return send(res, 200, { ok: true });
      },
    },
  ];
}
