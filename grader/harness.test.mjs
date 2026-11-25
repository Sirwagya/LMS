import { test } from "node:test";
import assert from "node:assert/strict";
import { createJudge } from "./judge.mjs";
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
