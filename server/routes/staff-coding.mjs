import { randomUUID } from "node:crypto";
import { fail, str } from "../domain/validation.mjs";
import { send, body } from "../http/response.mjs";
import { methodContracts } from "../coding/harness.mjs";
export function createRoutes({ lecture, persist }) {
  return [
    {
      match: ({ path, method }) =>
        path === "/api/staff/coding/assignments" && method === "POST",
      access: "staff",
      handle: async ({ req, res }) => {
        const b = await body(req),
          l = lecture(b.lectureId);
        if (!l) fail(404, "Lecture not found.");
        const title = str(b.title, "title", 250),
          description = str(b.description, "problem statement", 10000),
          inputFormat = str(b.inputFormat, "input format", 2000),
          outputFormat = str(b.outputFormat, "output format", 2000),
          constraints = str(b.constraints, "constraints", 2000);
        if (
          !/^\d{4}-\d{2}-\d{2}$/.test(b.dueDate) ||
          Number.isNaN(Date.parse(b.dueDate))
        )
          fail(400, "Use a valid due date.");
        if (
          !Array.isArray(b.testCases) ||
          b.testCases.length < 2 ||
          b.testCases.length > 10
        )
          fail(
            400,
            "Add 2–10 test cases with at least one example and one hidden test.",
          );
        const testCases = b.testCases.map((t) => {
          if (
            typeof t.input !== "string" ||
            typeof t.expected !== "string" ||
            t.input.length > 10000 ||
            t.expected.length > 10000
          )
            fail(
              400,
              "Test input/output must be strings under 10000 characters.",
            );
          return { input: t.input, expected: t.expected, hidden: !!t.hidden };
        });
        if (
          !testCases.some((t) => t.hidden) ||
          !testCases.some((t) => !t.hidden)
        )
          fail(400, "Include both example and hidden test cases.");
        const problem = {
          description,
          inputFormat,
          outputFormat,
          constraints,
          difficulty: ["Easy", "Medium", "Hard"].includes(b.difficulty)
            ? b.difficulty
            : "Easy",
          entrypoint: "solve",
          starterCode: methodContracts.solve,
          testCases,
        };
        l.assignments.push({
          id: randomUUID(),
          kind: "coding",
          title,
          dueDate: b.dueDate,
          status: "Pending",
          problem,
        });
        persist();
        return send(res, 201, { ok: true });
      },
    },
  ];
}
