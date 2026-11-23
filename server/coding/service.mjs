import { randomUUID } from "node:crypto";
import { fail, str } from "../domain/validation.mjs";
export function createCodingService({
  judge,
  attempts,
  assignment,
  submissions,
}) {
  const running = new Set();
  const rates = new Map();
  return {
    history(userId, assignmentId) {
      if (!assignment(assignmentId)) fail(404, "Assignment not found.");
      return attempts.history(userId, String(assignmentId));
    },
    async evaluate(user, request) {
      if (user.role !== "student") fail(403, "Student access required.");
      const a = assignment(request.assignmentId);
      if (!a || a.kind !== "coding" || !a.problem)
        fail(404, "Coding assignment not found.");
      const source = str(request.source, "source code", 20000);
      if (!["java", "python"].includes(request.language))
        fail(400, "Choose Java or Python.");
      if (!["run", "submit"].includes(request.action))
        fail(400, "Choose Run or Submit.");
      if (running.has(user.id))
        fail(429, "Wait for your current execution to finish.");
      const rate = rates.get(user.id) || {
        count: 0,
        reset: Date.now() + 60000,
      };
      if (rate.reset < Date.now()) {
        rate.count = 0;
        rate.reset = Date.now() + 60000;
      }
      if (++rate.count > 20)
        fail(429, "Run limit reached. Try again in a minute.");
      rates.set(user.id, rate);
      const submitting = request.action === "submit";
      let cases = a.problem.testCases;
      if (!submitting) {
        if (request.customInput !== undefined) {
          if (
            typeof request.customInput !== "string" ||
            request.customInput.length > 10000
          )
            fail(400, "Custom input must be under 10000 characters.");
          cases = [
            { input: request.customInput, expected: null, hidden: false },
          ];
        } else cases = cases.filter((t) => !t.hidden);
      }
      if (!cases?.length)
        fail(
          400,
          "The teacher must configure test cases before running this assignment.",
        );
      running.add(user.id);
      try {
        const result = await judge.evaluate({
          language: request.language,
          source,
          testCases: cases,
          entrypoint: a.problem.entrypoint,
        });
        if (submitting) {
          const id = randomUUID(),
            created = new Date().toISOString();
          attempts.save({
            id,
            userId: user.id,
            assignmentId: String(a.id),
            language: request.language,
            source,
            ...result,
            result,
            created,
          });
          if (result.verdict === "Accepted") {
            submissions.save(
              randomUUID(),
              user.id,
              String(a.id),
              a.course,
              `Language: ${request.language}\n\n${source}`,
              "",
              null,
              created,
            );
            submissions.grade(
              `${result.passed}/${result.total}`,
              `Accepted · ${result.passed}/${result.total} tests passed (${request.language}).`,
              submissions
                .forStudent(user.id)
                .find((s) => s.assignment_id === String(a.id)).id,
            );
          }
          return { ...result, attemptId: id };
        }
        return result;
      } finally {
        running.delete(user.id);
      }
    },
  };
}
