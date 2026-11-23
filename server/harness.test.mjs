import { test } from "node:test";
import assert from "node:assert/strict";
import { createJudge } from "./coding/judge.mjs";
import { installCodingSamples } from "./coding/catalog.mjs";
import { methodContracts } from "./coding/harness.mjs";
const judge = createJudge();
const samples = {
  twoSum: {
    cases: [
      { input: "4 9\n2 7 11 15\n", expected: "0 1" },
      { input: "2 6\n3 3\n", expected: "0 1", hidden: true },
    ],
    python:
      "class Solution:\n    def twoSum(self, nums: List[int], target: int) -> List[int]:\n        seen = {}\n        for i, value in enumerate(nums):\n            if target - value in seen:\n                return [seen[target-value], i]\n            seen[value] = i",
    java: "class Solution {public int[] twoSum(int[] nums, int target){Map<Integer,Integer> seen = new HashMap<>();for(int i=0;i<nums.length;i++){if(seen.containsKey(target-nums[i]))return new int[]{i,seen.get(target-nums[i])};seen.put(nums[i],i);}return new int[0];}}",
  },
  isValid: {
    cases: [
      { input: "()\n", expected: "true" },
      { input: "\n", expected: "true" },
      { input: "(\n", expected: "false" },
    ],
    python:
      'class Solution:\n    def isValid(self, s: str) -> bool:\n        return s in ("", "()")',
    java: 'class Solution {public boolean isValid(String s){return s.equals("") || s.equals("()");}}',
  },
  solve: {
    cases: [{ input: "2 3\n", expected: "5" }],
    python:
      "class Solution:\n    def solve(self, input: str) -> str:\n        return str(sum(map(int, input.split())))",
    java: 'class Solution {public String solve(String input){Scanner s=new Scanner(input);return ""+(s.nextInt()+s.nextInt());}}',
  },
};
for (const [entrypoint, sample] of Object.entries(samples))
  for (const language of ["python", "java"])
    test(`${language} hidden driver invokes Solution.${entrypoint} without student boilerplate`, async () => {
      const result = await judge.evaluate({
        language,
        source: sample[language],
        entrypoint,
        testCases: sample.cases,
      });
      assert.equal(result.verdict, "Accepted", JSON.stringify(result));
    });
test("existing sample assignments migrate in place without resetting test cases or unrelated challenges", () => {
  const custom = {
    id: "custom",
    kind: "coding",
    problem: { description: "Preserve me" },
  };
  const old = {
    id: "coding-two-sum",
    kind: "coding",
    problem: {
      testCases: [{ input: "private", expected: "private", hidden: true }],
    },
  };
  const academic = {
    lecturesBySubject: { E0460A: [{ assignments: [custom, old] }] },
  };
  let persisted = 0;
  installCodingSamples({ academic, persist: () => persisted++ });
  assert.equal(old.problem.entrypoint, "twoSum");
  assert.deepEqual(old.problem.starterCode, methodContracts.twoSum);
  assert.equal(old.problem.testCases[0].input, "private");
  assert.equal(custom.problem.description, "Preserve me");
  installCodingSamples({ academic, persist: () => persisted++ });
  assert.equal(
    academic.lecturesBySubject.E0460A[0].assignments.filter(
      (a) => a.id === "coding-two-sum",
    ).length,
    1,
  );
  assert.ok(persisted);
});
