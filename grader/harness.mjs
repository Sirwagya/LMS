// Platform-owned drivers: students implement Solution methods, never stdin plumbing.
export const methodContracts = {
  twoSum: {
    python:
      "class Solution:\n    def twoSum(self, nums: list[int], target: int) -> list[int]:\n        pass\n",
    java: "class Solution {\n    public int[] twoSum(int[] nums, int target) {\n        return new int[0];\n    }\n}\n",
  },
  isValid: {
    python:
      "class Solution:\n    def isValid(self, s: str) -> bool:\n        pass\n",
    java: "class Solution {\n    public boolean isValid(String s) {\n        return false;\n    }\n}\n",
  },
  solve: {
    python:
      "class Solution:\n    def solve(self, input: str) -> str:\n        # Return the answer as a string.\n        pass\n",
    java: 'class Solution {\n    public String solve(String input) {\n        return "";\n    }\n}\n',
  },
};
const pythonCalls = {
  twoSum:
    'data = list(map(int, _input.split()))\nn, target = data[:2]\nresult = Solution().twoSum(data[2:2+n], target)\nif not isinstance(result, (list, tuple)) or len(result) != 2:\n    raise ValueError("twoSum must return a list containing two indices")\nprint(*sorted(result))',
  isValid:
    'result = Solution().isValid(_input.rstrip("\\r\\n"))\nif not isinstance(result, bool):\n    raise ValueError("isValid must return a boolean")\nprint(str(result).lower())',
  solve:
    'result = Solution().solve(_input)\nif not isinstance(result, str):\n    raise ValueError("solve must return a string")\nprint(result)',
};
const javaCalls = {
  twoSum:
    'Scanner scanner = new Scanner(input); int n = scanner.nextInt(), target = scanner.nextInt(); int[] nums = new int[n]; for(int i=0;i<n;i++) nums[i]=scanner.nextInt(); int[] result = new Solution().twoSum(nums,target); if(result == null || result.length != 2) throw new IllegalArgumentException("twoSum must return two indices"); Arrays.sort(result); System.out.println(result[0]+" "+result[1]);',
  isValid: "System.out.println(new Solution().isValid(input.stripTrailing()));",
  solve:
    'String result = new Solution().solve(input); if(result == null) throw new IllegalArgumentException("solve must return a string"); System.out.print(result);',
};
export function wrapSolution(language, source, entrypoint) {
  if (!entrypoint) return source; // Compatibility for existing stdin/stdout challenges.
  if (!methodContracts[entrypoint])
    throw new Error("Unknown solution method contract.");
  if (language === "python")
    return `from typing import *\nimport collections, heapq, math, bisect, functools, itertools\nfrom collections import *\n${source}\n\nif __name__ == "__main__":\n    import sys\n    _input = sys.stdin.read()\n${pythonCalls[
      entrypoint
    ]
      .split("\n")
      .map((l) => "    " + l)
      .join("\n")}\n`;
  return `import java.util.*;\nimport java.io.*;\n${source}\n\npublic class Main {\n    public static void main(String[] args) throws Exception {\n        String input = new String(System.in.readAllBytes(), java.nio.charset.StandardCharsets.UTF_8);\n        ${javaCalls[entrypoint]}\n    }\n}\n`;
}
