import { useLMS } from "../../context/useLMS";
import { resolveLearningRoute, lecturePath } from "../../utils/learningRoutes";
import { useState } from "react";
import { ArrowLeft, Play, Send, RotateCcw, Code2 } from "lucide-react";
import { lecturesBySubject, semesterSubjects } from "../../data/lmsData";
import { useSession } from "../session";
import { CodeEditor } from "./CodeEditor";
import { SubmissionHistory } from "./SubmissionHistory";
import { TestResults } from "./TestResults";
import { useCodingWorkspace } from "./useCodingWorkspace";
import type { Assignment, CodingProblem } from "../../types";
import type { Language } from "./types";
export function CodingWorkspace() {
  const { route } = useLMS(),
    learning = resolveLearningRoute(route, lecturesBySubject),
    id = String(learning?.assignment?.id || "");
  const match = learning?.assignment
    ? { a: learning.assignment, course: learning.subjectCode }
    : undefined;
  if (!match?.a.problem)
    return (
      <section className="screen">
        <h1>Coding challenge not found</h1>
        <a className="text-link" href="#/assignments">
          Back to assignments
        </a>
      </section>
    );
  return (
    <Challenge
      key={id}
      assignment={match.a}
      problem={match.a.problem}
      course={
        Object.values(semesterSubjects)
          .flat()
          .find((c) => c.code === match.course)?.name || "Course"
      }
    />
  );
}
function Challenge({
  assignment,
  problem,
  course,
}: {
  assignment: Assignment;
  problem: CodingProblem;
  course: string;
}) {
  const id = String(assignment.id),
    work = useCodingWorkspace(id, problem),
    { submissions } = useSession(),
    graded = submissions.find((s) => s.assignment_id === id)?.score;
  const { route } = useLMS(),
    parent = resolveLearningRoute(route, lecturesBySubject);
  const backPath = parent?.lecture
    ? lecturePath(parent.subjectCode, parent.lecture.id)
    : "/assignments";
  const [leftPanel, setLeftPanel] = useState<"description" | "history">(
    "description",
  );
  const [panel, setPanel] = useState<"tests" | "submissions">("tests"),
    [custom, setCustom] = useState(false),
    [input, setInput] = useState("");
  const examples =
    problem.examples || problem.testCases?.filter((t) => !t.hidden) || [];
  return (
    <section className="screen coding-screen">
      <a href={`#${backPath}`} className="text-link self-start">
        <ArrowLeft />
        Back to lecture
      </a>
      <div className="coding-heading">
        <div>
          <p className="eyebrow">{course} · CODING CHALLENGE</p>
          <h1>{assignment.title}</h1>
        </div>
        <span className="badge">
          {graded ? `Accepted · ${graded}` : problem.difficulty}
        </span>
      </div>
      <div className="coding-layout">
        <article className="panel problem-pane">
          <div
            className="console-tabs problem-tabs"
            role="tablist"
            aria-label="Problem panels"
          >
            <button
              role="tab"
              aria-selected={leftPanel === "description"}
              aria-pressed={leftPanel === "description"}
              onClick={() => setLeftPanel("description")}
            >
              Description
            </button>
            <button
              role="tab"
              aria-selected={leftPanel === "history"}
              aria-pressed={leftPanel === "history"}
              onClick={() => setLeftPanel("history")}
            >
              Submissions ({work.history.length})
            </button>
          </div>
          {leftPanel === "history" ? (
            <SubmissionHistory
              attempts={work.history}
              entrypoint={problem.entrypoint}
              busy={!!work.busy}
              onRestore={work.restore}
            />
          ) : (
            <>
              <div className="problem-section-title">
                <Code2 size={16} />
                <h2>Description</h2>
              </div>
              <p className="problem-description">{problem.description}</p>
              <section>
                <h3>Input format</h3>
                <p>{problem.inputFormat}</p>
              </section>
              <section>
                <h3>Output format</h3>
                <p>{problem.outputFormat}</p>
              </section>
              {examples.map((e, i) => (
                <section key={i}>
                  <h3>Example {i + 1}</h3>
                  <div className="example-block">
                    <strong>Input</strong>
                    <pre>{e.input}</pre>
                    <strong>Output</strong>
                    <pre>{e.expected}</pre>
                  </div>
                </section>
              ))}
              <section>
                <h3>Constraints</h3>
                <p>{problem.constraints}</p>
              </section>
              <p className="coding-contract">
                {problem.entrypoint ? (
                  <>
                    Implement <code>Solution.{problem.entrypoint}</code> and
                    return your answer. Input parsing, imports and the test
                    driver are handled for you.
                  </>
                ) : (
                  <>
                    Read from stdin and write to stdout. Java entry point:{" "}
                    <code>public class Main</code>.
                  </>
                )}{" "}
                Run checks examples; Submit checks all tests.
              </p>
            </>
          )}
        </article>
        <div className="coding-right">
          <section className="panel editor-pane">
            <div className="editor-toolbar">
              <label>
                Language
                <select
                  aria-label="Programming language"
                  value={work.language}
                  disabled={!!work.busy}
                  onChange={(e) =>
                    work.changeLanguage(e.target.value as Language)
                  }
                >
                  <option value="python">Python</option>
                  <option value="java">Java</option>
                </select>
              </label>
              <span className="editor-draft">Draft saved locally</span>
              <button
                className="icon-button"
                aria-label="Reset code"
                disabled={!!work.busy}
                onClick={() => {
                  if (
                    window.confirm(
                      "Replace your current code with the starter template?",
                    )
                  )
                    work.reset();
                }}
              >
                <RotateCcw size={15} />
              </button>
            </div>
            <CodeEditor
              key={work.language}
              source={work.source}
              language={work.language}
              onChange={work.edit}
            />
            <div className="editor-actions">
              <span className="muted">3s / test · 20 KB source</span>
              <div className="inline-actions">
                <button
                  className="btn btn-secondary"
                  disabled={!!work.busy}
                  onClick={() => {
                    setPanel("tests");
                    void work.evaluate("run", custom ? input : undefined);
                  }}
                >
                  <Play />
                  {work.busy === "run" ? "Running…" : "Run"}
                </button>
                <button
                  className="btn btn-primary"
                  disabled={!!work.busy}
                  onClick={() => {
                    setPanel("tests");
                    setLeftPanel("history");
                    void work.evaluate("submit");
                  }}
                >
                  <Send />
                  {work.busy === "submit" ? "Judging…" : "Submit"}
                </button>
              </div>
            </div>
          </section>
          <section className="panel coding-console">
            <div className="console-tabs">
              <button
                aria-pressed={panel === "tests"}
                onClick={() => setPanel("tests")}
              >
                Test cases
              </button>
              <button
                aria-pressed={panel === "submissions"}
                onClick={() => setPanel("submissions")}
              >
                Submission history
              </button>
            </div>
            {panel === "tests" ? (
              <>
                <label className="custom-input-toggle">
                  <input
                    type="checkbox"
                    checked={custom}
                    onChange={(e) => setCustom(e.target.checked)}
                  />
                  Use custom input
                </label>
                {custom && (
                  <textarea
                    className="custom-stdin"
                    aria-label="Custom input"
                    value={input}
                    maxLength={10000}
                    rows={3}
                    onChange={(e) => setInput(e.target.value)}
                  />
                )}
                <p className="muted text-xs">
                  {custom
                    ? "Custom runs show stdout without judging an expected answer."
                    : `${examples.length} example tests · ${(problem.totalTests || problem.testCases?.length || 0) - examples.length} hidden tests on Submit`}
                </p>
                {work.busy ? (
                  <p role="status" className="runner-progress">
                    {work.busy === "run"
                      ? "Running your code…"
                      : "Compiling and judging all test cases…"}
                  </p>
                ) : work.result ? (
                  <TestResults
                    key={JSON.stringify(work.result)}
                    result={work.result}
                  />
                ) : (
                  <p className="console-empty">
                    Run your code to see the output here.
                  </p>
                )}
              </>
            ) : (
              <SubmissionHistory
                attempts={work.history}
                entrypoint={problem.entrypoint}
                busy={!!work.busy}
                onRestore={work.restore}
              />
            )}
            {work.error && (
              <p role="alert" className="inline-note mt-4">
                {work.error}
              </p>
            )}
          </section>
        </div>
      </div>
    </section>
  );
}
