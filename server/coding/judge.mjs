import { wrapSolution } from "./harness.mjs";
import { mkdtemp, writeFile, rm, realpath } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { languages } from "./languages.mjs";
import {
  execute,
  runtimeConfig,
  profileFor,
  launcherSource,
} from "./sandbox.mjs";
import { fail } from "../domain/validation.mjs";
const normalize = (value) => value.replace(/\r\n/g, "\n").trimEnd();
export function createJudge({
  run = execute,
  getRuntime = runtimeConfig,
} = {}) {
  let active = 0;
  return {
    async evaluate({ language, source, testCases, entrypoint }) {
      const strategy = languages[language];
      if (!strategy) fail(400, "Choose Java or Python.");
      if (active >= 2)
        fail(429, "The local runner is busy. Please try again shortly.");
      active++;
      let directory;
      try {
        let runtime;
        try {
          runtime = getRuntime();
        } catch (e) {
          fail(503, e.message);
        }
        directory = await realpath(
          await mkdtemp(join(tmpdir(), "vedam-code-")),
        );
        await Promise.all([
          writeFile(
            join(directory, strategy.filename),
            wrapSolution(language, source, entrypoint),
          ),
          writeFile(
            join(directory, "profile.sb"),
            profileFor(directory, runtime),
          ),
          writeFile(join(directory, "launcher.py"), await launcherSource()),
        ]);
        const invoke = (spec, phase, stdin = "") =>
          run({ runtime, directory, ...spec, phase, stdin });
        if (strategy.compile) {
          const result = await invoke(strategy.compile(runtime), "compile");
          if (result.code !== 0)
            return {
              verdict:
                result.status === "Runtime Error"
                  ? "Compilation Error"
                  : result.status,
              passed: 0,
              total: testCases.length,
              compileError:
                result.stderr ||
                result.stdout ||
                "Compiler could not start inside the sandbox.",
              results: [],
            };
        }
        const results = [];
        for (const [i, test] of testCases.entries()) {
          const result = await invoke(
            strategy.run(runtime),
            strategy.phase,
            test.input,
          );
          const status =
            result.code !== 0
              ? result.status
              : test.expected === null
                ? "Executed"
                : normalize(result.stdout) === normalize(test.expected)
                  ? "Accepted"
                  : "Wrong Answer";
          results.push({
            index: i + 1,
            hidden: !!test.hidden,
            status,
            durationMs: result.durationMs,
            ...(!test.hidden
              ? {
                  input: test.input,
                  expected: test.expected,
                  stdout: result.stdout,
                  stderr: result.stderr,
                }
              : {}),
          });
          if (
            [
              "Time Limit Exceeded",
              "Output Limit Exceeded",
              "File Limit Exceeded",
              "Memory Limit Exceeded",
            ].includes(status)
          )
            break;
        }
        const passed = results.filter((r) => r.status === "Accepted").length;
        return {
          verdict:
            results.find((r) => !["Accepted", "Executed"].includes(r.status))
              ?.status ||
            (testCases.some((t) => t.expected === null)
              ? "Executed"
              : "Accepted"),
          passed,
          total: testCases.length,
          results,
        };
      } finally {
        active--;
        if (directory) await rm(directory, { recursive: true, force: true });
      }
    },
  };
}
