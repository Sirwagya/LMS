use crate::model::{Entrypoint, Language};

const PYTHON_TWO_SUM: &str = "\
data = list(map(int, _input.split()))
n, target = data[:2]
result = Solution().twoSum(data[2:2+n], target)
if not isinstance(result, (list, tuple)) or len(result) != 2:
    raise ValueError(\"twoSum must return a list containing two indices\")
print(*sorted(result))";

const PYTHON_IS_VALID: &str = "\
result = Solution().isValid(_input.rstrip(\"\\r\\n\"))
if not isinstance(result, bool):
    raise ValueError(\"isValid must return a boolean\")
print(str(result).lower())";

const PYTHON_SOLVE: &str = "\
result = Solution().solve(_input)
if not isinstance(result, str):
    raise ValueError(\"solve must return a string\")
print(result)";

const JAVA_TWO_SUM: &str = "Scanner scanner = new Scanner(input); int n = scanner.nextInt(), target = scanner.nextInt(); int[] nums = new int[n]; for(int i=0;i<n;i++) nums[i]=scanner.nextInt(); int[] result = new Solution().twoSum(nums,target); if(result == null || result.length != 2) throw new IllegalArgumentException(\"twoSum must return two indices\"); Arrays.sort(result); System.out.println(result[0]+\" \"+result[1]);";

const JAVA_IS_VALID: &str = "System.out.println(new Solution().isValid(input.stripTrailing()));";

const JAVA_SOLVE: &str = "String result = new Solution().solve(input); if(result == null) throw new IllegalArgumentException(\"solve must return a string\"); System.out.print(result);";

pub fn wrap_solution(language: Language, source: &str, entrypoint: Entrypoint) -> String {
    if entrypoint == Entrypoint::Stdin {
        return source.to_string();
    }

    match language {
        Language::Python => {
            let body = match entrypoint {
                Entrypoint::TwoSum => PYTHON_TWO_SUM,
                Entrypoint::IsValid => PYTHON_IS_VALID,
                Entrypoint::Solve => PYTHON_SOLVE,
                Entrypoint::Stdin => unreachable!(),
            };
            let indented = body
                .lines()
                .map(|line| format!("    {line}"))
                .collect::<Vec<_>>()
                .join("\n");
            format!(
                "from typing import *\nimport collections, heapq, math, bisect, functools, itertools\nfrom collections import *\n{source}\n\nif __name__ == \"__main__\":\n    import sys\n    _input = sys.stdin.read()\n{indented}\n"
            )
        }
        Language::Java => {
            let call = match entrypoint {
                Entrypoint::TwoSum => JAVA_TWO_SUM,
                Entrypoint::IsValid => JAVA_IS_VALID,
                Entrypoint::Solve => JAVA_SOLVE,
                Entrypoint::Stdin => unreachable!(),
            };
            format!(
                "import java.util.*;\nimport java.io.*;\n{source}\n\npublic class Main {{\n    public static void main(String[] args) throws Exception {{\n        String input = new String(System.in.readAllBytes(), java.nio.charset.StandardCharsets.UTF_8);\n        {call}\n    }}\n}}\n"
            )
        }
    }
}
