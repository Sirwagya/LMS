import { fail, str } from "../domain/validation.mjs";
import { send, file, body } from "../http/response.mjs";
import { randomUUID } from "node:crypto";
export function createRoutes({
  submissions,
  progress,
  discussions,
  assets,
  lecture,
  assignment,
}) {
  return [
    {
      match: ({ path, method }) =>
        path === "/api/progress" && method === "POST",
      access: "authenticated",
      handle: async ({ req, res, u }) => {
        const b = await body(req);
        if (!lecture(b.lectureId)) fail(404, "Lecture not found.");
        if (b.complete) progress.complete(u.id, b.lectureId);
        else progress.incomplete(u.id, b.lectureId);
        return send(res, 200, { ok: true });
      },
    },
    {
      match: ({ path, method }) =>
        path === "/api/submissions" && method === "POST",
      access: "authenticated",
      handle: async ({ req, res, u }) => {
        if (u.role !== "student") fail(403, "Student access required.");
        const b = await body(req),
          a = assignment(b.assignmentId);
        if (!a) fail(404, "Assignment not found.");
        if (a.kind === "coding")
          fail(400, "Use the coding workspace to submit this assignment.");
        const answer = str(b.answer, "submission answer", 20000);
        const existing = submissions.gradeForAssignment(u.id, String(a.id));
        if (existing?.score) fail(409, "Graded submissions cannot be changed.");
        let bytes = null;
        let name = "";
        if (b.file) {
          name = str(b.filename, "filename", 200);
          bytes = Buffer.from(b.file, "base64");
          if (bytes.length > 10 * 1024 * 1024)
            fail(413, "Maximum attachment size is 10 MB.");
        }
        submissions.save(
          randomUUID(),
          u.id,
          String(a.id),
          a.course,
          answer,
          name,
          bytes,
          new Date().toISOString(),
        );
        return send(res, 200, { ok: true });
      },
    },
    {
      match: ({ path, method }) =>
        path.startsWith("/api/submission-file/") && method === "GET",
      access: "authenticated",
      handle: async ({ res, path, u }) => {
        const s = submissions.byId(path.split("/").pop());
        if (!s || !s.file) fail(404, "Attachment not found.");
        if (u.role === "student" && s.user_id !== u.id)
          fail(403, "Access denied.");
        return file(res, s.file, s.filename);
      },
    },
    {
      match: ({ path, method }) =>
        path === "/api/discussions" && method === "GET",
      access: "authenticated",
      handle: async ({ res, url }) => {
        const id = Number(url.searchParams.get("lecture"));
        return send(res, 200, discussions.forLecture(id));
      },
    },
    {
      match: ({ path, method }) =>
        path === "/api/discussions" && method === "POST",
      access: "authenticated",
      handle: async ({ req, res, u }) => {
        const b = await body(req);
        if (!lecture(b.lectureId)) fail(404, "Lecture not found.");
        discussions.create(
          randomUUID(),
          u.id,
          b.lectureId,
          str(b.body, "message", 2000),
          new Date().toISOString(),
        );
        return send(res, 201, { ok: true });
      },
    },
    {
      match: ({ path, method }) =>
        path.startsWith("/api/notes/") && method === "GET",
      access: "authenticated",
      handle: async ({ res, path }) => {
        const l = lecture(path.split("/").pop());
        if (!l) fail(404, "Lecture not found.");
        return file(
          res,
          `${l.title}\n${l.code} · ${l.date}\n\n${l.summary}\n\nSample local course notes. Replace these with instructor materials in the staff portal.`,
          `${l.title}.txt`,
          "text/plain; charset=utf-8",
        );
      },
    },
    {
      match: ({ path, method }) =>
        path.startsWith("/api/assets/") && method === "GET",
      access: "authenticated",
      handle: async ({ res, path }) => {
        const a = assets.byId(path.split("/").pop());
        if (!a) fail(404, "File not found.");
        return file(
          res,
          a.bytes,
          a.name,
          a.mime,
          a.mime !== "video/mp4" && a.mime !== "video/webm",
        );
      },
    },
  ];
}
