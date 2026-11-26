import type { Lecture } from "../types";
export type LectureCatalog = Record<string, Lecture[]>;
export const subjectPath = (subjectCode: string) =>
  `/subjects/${encodeURIComponent(subjectCode)}`;
export const lecturePath = (subjectCode: string, lectureId: number) =>
  `${subjectPath(subjectCode)}/lectures/${lectureId}`;
export const assignmentPath = (
  subjectCode: string,
  lectureId: number,
  assignmentId: string | number,
) =>
  `${lecturePath(subjectCode, lectureId)}/assignments/${encodeURIComponent(String(assignmentId))}`;
export function assignmentLocation(
  id: string | number,
  catalog: LectureCatalog,
) {
  for (const [subjectCode, lectures] of Object.entries(catalog))
    for (const lecture of lectures) {
      const assignment = lecture.assignments.find(
        (a) => String(a.id) === String(id),
      );
      if (assignment)
        return {
          subjectCode,
          lecture,
          assignment,
          path: assignmentPath(subjectCode, lecture.id, id),
        };
    }
  return undefined;
}
export function resolveLearningRoute(route: string, catalog: LectureCatalog) {
  const parts = route.split("/");
  if (parts[1] === "coding" && parts.length === 3) {
    const found = assignmentLocation(parts[2], catalog);
    return found
      ? {
          kind: "assignment" as const,
          subjectCode: found.subjectCode,
          lecture: found.lecture,
          assignment: found.assignment,
          canonical: found.path,
        }
      : undefined;
  }
  if (parts[1] !== "subjects" || !Object.hasOwn(catalog, parts[2])) return undefined;
  const subjectCode = parts[2];
  if (parts.length === 3)
    return {
      kind: "subject" as const,
      subjectCode,
      canonical: subjectPath(subjectCode),
    };
  if (parts[3] !== "lectures") return undefined;
  const lecture = catalog[subjectCode].find((l) => l.id === Number(parts[4]));
  if (!lecture) return undefined;
  if (parts.length === 5)
    return {
      kind: "lecture" as const,
      subjectCode,
      lecture,
      canonical: lecturePath(subjectCode, lecture.id),
    };
  if (parts.length !== 7 || parts[5] !== "assignments") return undefined;
  const assignment = lecture.assignments.find((a) => String(a.id) === parts[6]);
  return assignment
    ? {
        kind: "assignment" as const,
        subjectCode,
        lecture,
        assignment,
        canonical: assignmentPath(subjectCode, lecture.id, assignment.id),
      }
    : undefined;
}
