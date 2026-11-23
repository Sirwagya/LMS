import { test } from "node:test";
import assert from "node:assert/strict";
import {
  assignmentPath,
  assignmentLocation,
  resolveLearningRoute,
} from "../src/utils/learningRoutes.ts";
const catalog = {
  ALG: [
    {
      id: 1,
      title: "Arrays",
      assignments: [{ id: "two-sum", title: "Two Sum", kind: "coding" }],
    },
    { id: 2, title: "Graphs", assignments: [] },
  ],
  DB: [
    {
      id: 3,
      title: "Indexes",
      assignments: [{ id: "report", title: "Query report" }],
    },
  ],
};
test("subject, lecture and assignment routes resolve through their actual parents", () => {
  assert.equal(resolveLearningRoute("/subjects/ALG", catalog).kind, "subject");
  assert.equal(
    resolveLearningRoute("/subjects/ALG/lectures/1", catalog).lecture.title,
    "Arrays",
  );
  const assignment = resolveLearningRoute(
    assignmentPath("ALG", 1, "two-sum"),
    catalog,
  );
  assert.equal(assignment.assignment.title, "Two Sum");
  assert.equal(assignment.subjectCode, "ALG");
  assert.equal(
    assignmentLocation("report", catalog).path,
    "/subjects/DB/lectures/3/assignments/report",
  );
});
test("foreign lecture and assignment IDs cannot escape the subject hierarchy", () => {
  for (const route of [
    "/subjects/DB/lectures/1",
    "/subjects/ALG/lectures/2/assignments/two-sum",
    "/subjects/ALG/lectures/1/assignments/report",
    "/subjects/missing",
    "/subjects/ALG/lectures/1/unexpected",
  ])
    assert.equal(resolveLearningRoute(route, catalog), undefined);
});
test("old coding bookmarks resolve to canonical nested assignment URLs", () => {
  const legacy = resolveLearningRoute("/coding/two-sum", catalog);
  assert.equal(
    legacy.canonical,
    "/subjects/ALG/lectures/1/assignments/two-sum",
  );
  assert.equal(resolveLearningRoute("/coding/missing", catalog), undefined);
});
