import { fail, str } from "../domain/validation.mjs";
import { send, body } from "../http/response.mjs";
export function createRoutes({ academic, persist, course }) {
  return [
    {
      match: ({ path, method }) =>
        path === "/api/staff/schedule" && method === "POST",
      access: "staff",
      handle: async ({ req, res }) => {
        const b = await body(req);
        if (!course(b.course)) fail(404, "Course not found.");
        const date = str(b.date, "date", 10);
        if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || Number.isNaN(Date.parse(date)))
          fail(400, "Invalid date.");
        const from = str(b.from, "start time", 5),
          to = str(b.to, "end time", 5);
        if (
          !/^([01]\d|2[0-3]):[0-5]\d$/.test(from) ||
          !/^([01]\d|2[0-3]):[0-5]\d$/.test(to) ||
          to <= from
        )
          fail(400, "End time must follow start time.");
        const d = new Date(date + "T12:00:00");
        d.setDate(d.getDate() - ((d.getDay() + 6) % 7));
        academic.demoWeek.start = d.toISOString().slice(0, 10);
        academic.demoWeek.published = true;
        const entries = academic.demoWeek.days[date] || [];
        if (entries.some((e) => from < e.to && to > e.from))
          fail(409, "This session overlaps an existing class.");
        entries.push({
          from,
          to,
          code: b.course,
          room: str(b.room, "room", 150),
          topic: str(b.topic, "topic", 200),
          type: "Lecture",
        });
        academic.demoWeek.days[date] = entries.sort((a, b) =>
          a.from.localeCompare(b.from),
        );
        persist();
        return send(res, 201, { ok: true });
      },
    },
  ];
}
