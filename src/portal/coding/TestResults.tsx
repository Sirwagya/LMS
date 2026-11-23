import { useState } from "react";
import type { JudgeResult } from "./types";
export function TestResults({ result }: { result: JudgeResult }) {
  const [selected, setSelected] = useState(0),
    test = result.results[selected] || result.results[0];
  return (
    <section className="judge-result" role="status">
      <div className="judge-verdict">
        <strong data-accepted={result.verdict === "Accepted"}>
          {result.verdict}
        </strong>
        <span>
          {result.verdict === "Executed"
            ? "Custom run"
            : `${result.passed}/${result.total} tests passed`}
        </span>
      </div>
      {result.compileError && (
        <pre className="code-output">{result.compileError}</pre>
      )}
      {!!result.results.length && (
        <>
          <div className="test-case-tabs">
            {result.results.map((t, i) => (
              <button
                key={i}
                className="test-case-tab"
                aria-pressed={selected === i}
                onClick={() => setSelected(i)}
              >
                {t.status === "Accepted"
                  ? "✓"
                  : t.status === "Executed"
                    ? "·"
                    : "×"}{" "}
                {t.hidden ? "Hidden" : "Case"} {t.index}
              </button>
            ))}
          </div>
          {test && (
            <>
              <div className="judge-case-meta">
                <strong>{test.status}</strong>
                <span>{test.durationMs} ms</span>
              </div>
              {test.hidden ? (
                <p className="muted">
                  Hidden test inputs and outputs are kept private.
                </p>
              ) : (
                <div className="test-output-grid">
                  {[
                    ["Input", test.input],
                    ["Your output", test.stdout || "(empty)"],
                    ...(test.expected !== null
                      ? [["Expected output", test.expected]]
                      : []),
                    ...(test.stderr ? [["Error", test.stderr]] : []),
                  ].map(([label, text]) => (
                    <div key={label}>
                      <h4>{label}</h4>
                      <pre className="code-output">{text}</pre>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}
        </>
      )}
    </section>
  );
}
