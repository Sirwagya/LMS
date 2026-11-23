import { ActionForm, Field, LectureSelect } from "./Forms";
const sample = JSON.stringify(
  [
    { input: "2 3\n", expected: "5", hidden: false },
    { input: "-1 1\n", expected: "0", hidden: true },
  ],
  null,
  2,
);
export function CodingAssignmentForm({
  reload,
}: {
  reload: () => Promise<void>;
}) {
  return (
    <ActionForm
      title="Create coding challenge"
      endpoint="/staff/coding/assignments"
      onDone={reload}
      transform={async (d) => ({
        ...d,
        testCases: JSON.parse(String(d.testCases)),
      })}
    >
      <LectureSelect />
      <Field label="Challenge title" name="title" />
      <Field label="Due date" name="dueDate" type="date" />
      <label>
        Difficulty
        <select name="difficulty">
          <option>Easy</option>
          <option>Medium</option>
          <option>Hard</option>
        </select>
      </label>
      <label>
        Problem statement
        <textarea name="description" rows={4} required maxLength={10000} />
      </label>
      <Field label="Input format" name="inputFormat" />
      <Field label="Output format" name="outputFormat" />
      <Field label="Constraints" name="constraints" />
      <label>
        Test cases (JSON, 2–10 cases)
        <textarea
          name="testCases"
          rows={12}
          defaultValue={sample}
          required
          spellCheck={false}
        />
      </label>
      <p className="muted text-xs">
        Include at least one example with hidden: false and one hidden test.
        Students implement Solution.solve(input) and return a string. The driver
        handles stdin/stdout; hidden tests stay private from students.
      </p>
    </ActionForm>
  );
}
