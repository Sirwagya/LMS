import { newQuestion, type ContestQuestion, type ContestTestCase } from "./types";

export function ContestQuestionEditor({ question, onChange }: {
  question: ContestQuestion;
  onChange: (question: ContestQuestion) => void;
}) {
  return (
    <>
      <label>
        Question type
        <select value={question.kind} onChange={(e) => onChange(newQuestion(e.target.value === "mcq" ? "mcq" : "coding"))}>
          <option value="mcq">Multiple choice</option>
          <option value="coding">Coding</option>
        </select>
      </label>
      <label>
        Prompt
        <textarea rows={4} required maxLength={10000} value={question.prompt} onChange={(e) => onChange({ ...question, prompt: e.target.value })} />
      </label>
      <label>
        Points
        <input type="number" required min={1} max={1000} value={question.points} onChange={(e) => onChange({ ...question, points: Number(e.target.value) })} />
      </label>
      {question.kind === "mcq" ? (
        <>
          {question.choices.map((choice, index) => (
            <label key={index}>
              Choice {index + 1}
              <input required maxLength={1000} value={choice} onChange={(e) => onChange({
                ...question,
                choices: question.choices.map((value, i) => i === index ? e.target.value : value),
              })} />
            </label>
          ))}
          <div className="inline-actions">
            <button type="button" className="btn btn-secondary" disabled={question.choices.length >= 8} onClick={() => onChange({ ...question, choices: [...question.choices, ""] })}>Add choice</button>
            <button type="button" className="btn btn-secondary" disabled={question.choices.length <= 2} onClick={() => onChange({
              ...question,
              choices: question.choices.slice(0, -1),
              correctChoice: Math.min(question.correctChoice, question.choices.length - 2),
            })}>Remove last choice</button>
          </div>
          <label>
            Correct answer
            <select value={question.correctChoice} onChange={(e) => onChange({ ...question, correctChoice: Number(e.target.value) })}>
              {question.choices.map((_, i) => <option value={i} key={i}>Choice {i + 1}</option>)}
            </select>
          </label>
          <label>
            Explanation (optional)
            <textarea rows={2} maxLength={4000} value={question.explanation ?? ""} onChange={(e) => onChange({ ...question, explanation: e.target.value })} />
          </label>
        </>
      ) : <CodingCases cases={question.testCases} onChange={(testCases) => onChange({ ...question, testCases })} />}
    </>
  );
}

function CodingCases({ cases, onChange }: { cases: ContestTestCase[]; onChange: (cases: ContestTestCase[]) => void }) {
  return (
    <>
      <p className="muted">Students solve the problem in Python or Java using standard input and output. Add at least one example and one hidden case.</p>
      {cases.map((test, index) => {
        const update = (value: ContestTestCase) => onChange(cases.map((current, i) => i === index ? value : current));
        return (
          <fieldset key={index} className="portal-form">
            <legend>Test case {index + 1}</legend>
            <label>
              Input
              <textarea rows={2} maxLength={10000} spellCheck={false} value={test.input} onChange={(e) => update({ ...test, input: e.target.value })} />
            </label>
            <label>
              Expected output
              <textarea rows={2} maxLength={10000} spellCheck={false} value={test.expected} onChange={(e) => update({ ...test, expected: e.target.value })} />
            </label>
            <label>
              Visibility
              <select value={test.hidden ? "hidden" : "example"} onChange={(e) => update({ ...test, hidden: e.target.value === "hidden" })}>
                <option value="example">Example</option><option value="hidden">Hidden</option>
              </select>
            </label>
          </fieldset>
        );
      })}
      <div className="inline-actions">
        <button type="button" className="btn btn-secondary" disabled={cases.length >= 10} onClick={() => onChange([...cases, { input: "", expected: "", hidden: true }])}>Add test case</button>
        <button type="button" className="btn btn-secondary" disabled={cases.length <= 2} onClick={() => onChange(cases.slice(0, -1))}>Remove last case</button>
      </div>
    </>
  );
}
