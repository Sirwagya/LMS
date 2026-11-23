import { CodingAssignmentForm } from "./CodingAssignmentForm";
import { lecturesBySubject } from "../../data/lmsData";
import { Field, LectureSelect, ActionForm } from "./Forms";
export function AssignmentsSection({
  reload,
}: {
  reload: () => Promise<void>;
}) {
  return (
    <>
      <CodingAssignmentForm reload={reload} />
      <ActionForm
        title="Create assignment"
        endpoint="/staff/assignments"
        onDone={reload}
      >
        <LectureSelect />
        <Field label="Assignment title / instructions" name="title" />
        <Field label="Due date" name="dueDate" type="date" />
      </ActionForm>
      <section className="panel panel-padding">
        <h2>Published assignments</h2>
        {Object.values(lecturesBySubject)
          .flat()
          .flatMap((l) =>
            l.assignments.map((a) => (
              <div key={a.id} className="notice-row">
                <div>
                  <h3>
                    {a.title}{" "}
                    {a.kind === "coding" && (
                      <span className="badge">Coding</span>
                    )}
                  </h3>
                  <p>
                    {l.title} · Due {a.dueDate}
                  </p>
                </div>
              </div>
            )),
          )}
      </section>
    </>
  );
}
