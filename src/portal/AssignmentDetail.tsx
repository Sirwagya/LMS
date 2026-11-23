import { ArrowLeft } from "lucide-react";
import { lecturesBySubject } from "../data/lmsData";
import { useLMS } from "../context/useLMS";
import { resolveLearningRoute, lecturePath } from "../utils/learningRoutes";
import { AssignmentSubmission } from "./LearningTools";
import { PageHeader, EmptyState } from "../components/ui/Workspace";
export function AssignmentDetail() {
  const { route } = useLMS(),
    learning = resolveLearningRoute(route, lecturesBySubject);
  if (!learning?.assignment || !learning.lecture)
    return (
      <EmptyState
        title="Assignment not found"
        description="Open the assignment from its parent lecture."
      />
    );
  return (
    <section className="screen">
      <a
        className="text-link self-start"
        href={`#${lecturePath(learning.subjectCode, learning.lecture.id)}`}
      >
        <ArrowLeft />
        Back to {learning.lecture.title}
      </a>
      <PageHeader
        eyebrow="LECTURE ASSIGNMENT"
        title={learning.assignment.title}
        description={`Due ${learning.assignment.dueDate || "No deadline"}`}
      />
      <div className="panel panel-padding">
        <AssignmentSubmission assignment={learning.assignment} expanded />
      </div>
    </section>
  );
}
