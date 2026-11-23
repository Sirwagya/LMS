import { lecturesBySubject } from "../../data/lmsData";
import { LearningTools } from "../LearningTools";
export function DiscussionsSection({
  discussionId,
  setDiscussionId,
}: {
  discussionId: number;
  setDiscussionId: (id: number) => void;
}) {
  return (
    <section className="panel panel-padding">
      <label>
        Lecture
        <select
          className="semester-select w-full"
          value={discussionId}
          onChange={(e) => setDiscussionId(Number(e.target.value))}
        >
          {Object.values(lecturesBySubject)
            .flat()
            .map((l) => (
              <option key={l.id} value={l.id}>
                {l.title}
              </option>
            ))}
        </select>
      </label>
      <LearningTools key={discussionId} lectureId={discussionId} />
    </section>
  );
}
