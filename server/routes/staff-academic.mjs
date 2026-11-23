import { fail, str, num } from "../domain/validation.mjs";
import { send, body } from "../http/response.mjs";
import { randomUUID } from "node:crypto";
export function createRoutes({ assets, academic, persist, course, lecture }) {
  return [
    {
      match: ({ path, method }) =>
        path === "/api/staff/catalog" && method === "PUT",
      access: "staff",
      handle: async ({ req, res }) => {
        const b = await body(req);
        const kind = str(b.kind, "record type", 30);
        if (kind === "course") {
          const c = course(b.id);
          if (!c) fail(404, "Course not found.");
          c.name = str(b.title, "name", 200);
          c.instructor = str(b.description, "instructor", 100);
        } else if (kind === "lecture") {
          const l = lecture(b.id);
          if (!l) fail(404, "Lecture not found.");
          l.title = str(b.title, "title", 200);
          l.summary = str(b.description, "notes", 20000);
        } else if (kind === "notice") {
          const n = academic.drawerNotices.find((n) => n.id === b.id);
          if (!n) fail(404, "Notice not found.");
          n.title = str(b.title, "title", 200);
          n.desc = str(b.description, "message", 5000);
        } else fail(400, "Unsupported record type.");
        persist();
        return send(res, 200, { ok: true });
      },
    },
    {
      match: ({ path, method }) =>
        path === "/api/staff/courses" && method === "POST",
      access: "staff",
      handle: async ({ req, res }) => {
        const b = await body(req);
        const sem = num(b.semester, "semester", 1, 3);
        if (!Number.isInteger(sem)) fail(400, "Invalid semester.");
        const code = str(b.code, "course identifier", 30);
        if (!/^[\w-]+$/.test(code))
          fail(400, "Use letters, numbers and hyphens for course identifier.");
        const existing = course(code);
        if (existing) fail(409, "Course identifier already exists.");
        academic.semesterSubjects[sem].push({
          code,
          name: str(b.name, "course name", 200),
          instructor: str(b.instructor, "instructor", 100),
          credits: num(b.credits, "credits", 1, 10),
          next: "New course",
          att: "0%",
          skips: "0",
          attendedClasses: 0,
          totalClasses: 0,
        });
        academic.lecturesBySubject[code] = [];
        persist();
        return send(res, 201, { ok: true });
      },
    },
    {
      match: ({ path, method }) =>
        path === "/api/staff/lectures" && method === "POST",
      access: "staff",
      handle: async ({ req, res }) => {
        const b = await body(req);
        if (!course(b.course)) fail(404, "Course not found.");
        const l = {
          id: Date.now(),
          code: `LEC ${academic.lecturesBySubject[b.course].length + 1}`,
          date: new Date().toISOString().slice(0, 10),
          month: "",
          unit: 1,
          title: str(b.title, "title", 200),
          rawTitle: b.title,
          attendance: "Awaiting",
          summary: str(b.summary, "lecture notes", 20000),
          slides: "",
          assignments: [],
          resources: [],
          recordingUrl: null,
        };
        academic.lecturesBySubject[b.course].unshift(l);
        persist();
        return send(res, 201, { id: l.id });
      },
    },
    {
      match: ({ path, method }) =>
        path === "/api/staff/assignments" && method === "POST",
      access: "staff",
      handle: async ({ req, res }) => {
        const b = await body(req);
        const l = lecture(b.lectureId);
        if (!l) fail(404, "Lecture not found.");
        const date = str(b.dueDate, "due date", 10);
        if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || Number.isNaN(Date.parse(date)))
          fail(400, "Use a valid date.");
        l.assignments.push({
          id: randomUUID(),
          title: str(b.title, "assignment title", 250),
          dueDate: date,
          status: "Pending",
        });
        persist();
        return send(res, 201, { ok: true });
      },
    },
    {
      match: ({ path, method }) =>
        path === "/api/staff/notices" && method === "POST",
      access: "staff",
      handle: async ({ req, res }) => {
        const b = await body(req);
        academic.drawerNotices.unshift({
          id: randomUUID(),
          title: str(b.title, "title", 200),
          desc: str(b.desc, "notice", 5000),
          tag: b.tag === "URGENT" ? "URGENT" : "ACADEMIC",
          tagBg: "",
          time: new Date().toLocaleString("en-GB"),
        });
        persist();
        return send(res, 201, { ok: true });
      },
    },
    {
      match: ({ path, method }) =>
        path === "/api/staff/assets" && method === "POST",
      access: "staff",
      handle: async ({ req, res }) => {
        const b = await body(req);
        const l = lecture(b.lectureId);
        if (!l) fail(404, "Lecture not found.");
        const name = str(b.name, "filename", 200);
        const bytes = Buffer.from(
          str(b.bytes, "file contents", 15 * 1024 * 1024),
          "base64",
        );
        if (bytes.length > 10 * 1024 * 1024)
          fail(413, "Maximum file size is 10 MB.");
        const isVideo = /\.(mp4|webm)$/i.test(name);
        if (b.recording && !isVideo) fail(400, "Use an MP4 or WebM video.");
        const id = randomUUID(),
          mime = isVideo
            ? name.endsWith(".webm")
              ? "video/webm"
              : "video/mp4"
            : "application/octet-stream";
        assets.create(id, name, mime, bytes);
        if (b.recording) l.recordingUrl = `/api/assets/${id}`;
        else
          l.resources.push({
            title: name,
            type: "File",
            size: `${Math.ceil(bytes.length / 1024)} KB`,
            url: `/api/assets/${id}`,
          });
        persist();
        return send(res, 201, { ok: true });
      },
    },
  ];
}
