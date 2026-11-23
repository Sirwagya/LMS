import { methodContracts } from "./harness.mjs";
export const starterCode = {
  python:
    'import sys\n\ndef solve():\n    # Read input from stdin and print the answer.\n    data = sys.stdin.read().split()\n    \n\nif __name__ == "__main__":\n    solve()\n',
  java: "import java.util.*;\n\npublic class Main {\n    public static void main(String[] args) {\n        Scanner in = new Scanner(System.in);\n        // Read input and print the answer.\n        \n    }\n}\n",
};
const twoSum = {
  description:
    "Given an array of integers and a target, return the zero-based indices of two distinct elements whose sum equals the target. Exactly one valid pair exists. Print the smaller index first.",
  inputFormat: "Line 1: n and target. Line 2: n space-separated integers.",
  outputFormat:
    "Two zero-based indices separated by a space, in ascending order.",
  constraints:
    "2 ≤ n ≤ 10000; −1000000 ≤ nums[i], target ≤ 1000000. Exactly one solution exists.",
  difficulty: "Easy",
  entrypoint: "twoSum",
  starterCode: methodContracts.twoSum,
  testCases: [
    { input: "4 9\n2 7 11 15\n", expected: "0 1", hidden: false },
    { input: "3 6\n3 2 4\n", expected: "1 2", hidden: false },
    { input: "2 6\n3 3\n", expected: "0 1", hidden: true },
    { input: "5 0\n-3 8 2 3 10\n", expected: "0 3", hidden: true },
    { input: "4 8\n1 2 3 5\n", expected: "2 3", hidden: true },
  ],
};
const parentheses = {
  description:
    "Given a string containing only (, ), [, ], { and }, determine whether all brackets are balanced and correctly nested.",
  inputFormat:
    "A single line containing a bracket string. An empty line is valid input.",
  outputFormat:
    "Print true if the string is valid; otherwise print false (lowercase).",
  constraints: "0 ≤ string length ≤ 10000.",
  difficulty: "Easy",
  entrypoint: "isValid",
  starterCode: methodContracts.isValid,
  testCases: [
    { input: "()[]{}\n", expected: "true", hidden: false },
    { input: "([)]\n", expected: "false", hidden: false },
    { input: "{[()]}\n", expected: "true", hidden: true },
    { input: "(\n", expected: "false", hidden: true },
    { input: "\n", expected: "true", hidden: true },
  ],
};
export function installCodingSamples({ academic, persist }) {
  let migrated = false;
  for (const l of Object.values(academic.lecturesBySubject).flat())
    for (const a of l.assignments) {
      const entrypoint =
        a.id === "coding-two-sum"
          ? "twoSum"
          : a.id === "coding-valid-parentheses"
            ? "isValid"
            : null;
      if (entrypoint && a.problem && a.problem.entrypoint !== entrypoint) {
        a.problem.entrypoint = entrypoint;
        a.problem.starterCode = methodContracts[entrypoint];
        migrated = true;
      }
    }
  if (migrated) persist();
  const ids = new Set(
    Object.values(academic.lecturesBySubject)
      .flat()
      .flatMap((l) => l.assignments.map((a) => a.id)),
  );
  const lectures =
    academic.lecturesBySubject.E0460A ||
    Object.values(academic.lecturesBySubject)[0];
  if (!lectures?.length) return;
  for (const [id, title, problem] of [
    ["coding-two-sum", "Two Sum", twoSum],
    ["coding-valid-parentheses", "Valid Parentheses", parentheses],
  ]) {
    if (ids.has(id)) continue;
    lectures[0].assignments.push({
      id,
      title,
      kind: "coding",
      dueDate: "2026-12-15",
      status: "Pending",
      problem,
    });
  }
  persist();
}
export function publicAssignments(academic, isStaff) {
  if (isStaff) return academic;
  const data = structuredClone(academic);
  for (const lectures of Object.values(data.lecturesBySubject))
    for (const l of lectures)
      for (const a of l.assignments)
        if (a.problem) {
          const { testCases, ...problem } = a.problem;
          a.problem = {
            ...problem,
            examples: (testCases || [])
              .filter((t) => !t.hidden)
              .map(({ input, expected }) => ({ input, expected })),
            totalTests: (testCases || []).length,
          };
        }
  return data;
}
