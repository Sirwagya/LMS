import { Fragment } from "react";
import { ChevronRight } from "lucide-react";
import { useLMS } from "../../context/useLMS";
import { lecturesBySubject } from "../../data/lmsData";
import {
  resolveLearningRoute,
  subjectPath,
  lecturePath,
} from "../../utils/learningRoutes";
export function LearningBreadcrumb({ fallback }: { fallback: string }) {
  const { route, subjectByCode } = useLMS(),
    learning = resolveLearningRoute(route, lecturesBySubject);
  const items: { label: string; path?: string }[] = [
    { label: "Subjects", path: "/dashboard" },
  ];
  if (learning) {
    items.push({
      label: subjectByCode(learning.subjectCode)?.name || "Subject",
      path:
        learning.kind !== "subject"
          ? subjectPath(learning.subjectCode)
          : undefined,
    });
    if (learning.lecture)
      items.push({
        label: learning.lecture.title,
        path:
          learning.kind === "assignment"
            ? lecturePath(learning.subjectCode, learning.lecture.id)
            : undefined,
      });
    if (learning.assignment) items.push({ label: learning.assignment.title });
  } else {
    items[0] = { label: "Student workspace", path: "/dashboard" };
    items.push({ label: fallback });
  }
  return (
    <nav className="breadcrumb learning-breadcrumb" aria-label="Breadcrumb">
      {items.map((item, i) => (
        <Fragment key={i}>
          {i > 0 && <ChevronRight size={12} aria-hidden="true" />}
          {item.path ? (
            <a href={`#${item.path}`}>{item.label}</a>
          ) : (
            <strong aria-current="page">{item.label}</strong>
          )}
        </Fragment>
      ))}
    </nav>
  );
}
