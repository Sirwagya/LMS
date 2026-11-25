use lms_grader::{Entrypoint, GradeRequest, JudgePipeline, Language, TestCaseInput, Verdict};

fn test_case(input: &str, expected: Option<&str>, hidden: bool) -> TestCaseInput {
    TestCaseInput {
        input: input.to_string(),
        expected: expected.map(str::to_string),
        hidden,
    }
}

#[tokio::test]
async fn python_verdicts_custom_input_and_hidden_redaction() {
    let judge = JudgePipeline::default();

    let accepted = judge
        .evaluate(GradeRequest {
            language: Language::Python,
            source: "a,b=map(int,input().split());print(a+b)".to_string(),
            entrypoint: Entrypoint::Stdin,
            test_cases: vec![test_case("2 3\n", Some("5"), false)],
        })
        .await
        .expect("evaluate accepted");
    assert_eq!(accepted.verdict, Verdict::Accepted);
    assert_eq!(accepted.passed, 1);

    let wrong = judge
        .evaluate(GradeRequest {
            language: Language::Python,
            source: "print(0)".to_string(),
            entrypoint: Entrypoint::Stdin,
            test_cases: vec![test_case("2 3\n", Some("5"), false)],
        })
        .await
        .expect("evaluate wrong answer");
    assert_eq!(wrong.verdict, Verdict::WrongAnswer);

    let custom = judge
        .evaluate(GradeRequest {
            language: Language::Python,
            source: "print(input()[::-1])".to_string(),
            entrypoint: Entrypoint::Stdin,
            test_cases: vec![test_case("abc", None, false)],
        })
        .await
        .expect("evaluate custom input");
    assert_eq!(custom.verdict, Verdict::Executed);
    assert_eq!(
        custom.results[0].stdout.as_deref().map(str::trim),
        Some("cba")
    );

    let runtime_err = judge
        .evaluate(GradeRequest {
            language: Language::Python,
            source: "print(1/0)".to_string(),
            entrypoint: Entrypoint::Stdin,
            test_cases: vec![test_case("2 3\n", Some("5"), false)],
        })
        .await
        .expect("evaluate runtime error");
    assert_eq!(runtime_err.verdict, Verdict::RuntimeError);

    let hidden = judge
        .evaluate(GradeRequest {
            language: Language::Python,
            source: "print(input())".to_string(),
            entrypoint: Entrypoint::Stdin,
            test_cases: vec![test_case("secret test data", Some("different"), true)],
        })
        .await
        .expect("evaluate hidden");
    assert_eq!(hidden.verdict, Verdict::WrongAnswer);
    assert!(hidden.results[0].hidden);
    assert!(hidden.results[0].input.is_none());
    assert!(hidden.results[0].expected.is_none());
    assert!(hidden.results[0].stdout.is_none());
    assert!(hidden.results[0].stderr.is_none());
}

#[tokio::test]
async fn java_compile_execute_and_compile_error() {
    let judge = JudgePipeline::default();

    let accepted = judge
        .evaluate(GradeRequest {
            language: Language::Java,
            source: "import java.util.*; public class Main {public static void main(String[] args){Scanner s=new Scanner(System.in);System.out.println(s.nextInt()+s.nextInt());}}".to_string(),
            entrypoint: Entrypoint::Stdin,
            test_cases: vec![test_case("2 3\n", Some("5"), false)],
        })
        .await
        .expect("evaluate java accepted");
    assert_eq!(accepted.verdict, Verdict::Accepted);

    let compile_err = judge
        .evaluate(GradeRequest {
            language: Language::Java,
            source: "public class Main { broken syntax }".to_string(),
            entrypoint: Entrypoint::Stdin,
            test_cases: vec![test_case("2 3\n", Some("5"), false)],
        })
        .await
        .expect("evaluate java compile error");
    assert_eq!(compile_err.verdict, Verdict::CompilationError);
    assert!(!compile_err.compile_error.unwrap_or_default().is_empty());
}

#[tokio::test]
async fn solution_harnesses_python_and_java() {
    let judge = JudgePipeline::default();

    let cases_two_sum = vec![
        test_case("4 9\n2 7 11 15\n", Some("0 1"), false),
        test_case("2 6\n3 3\n", Some("0 1"), true),
    ];
    for (lang, src) in [
        (
            Language::Python,
            "class Solution:\n    def twoSum(self, nums: List[int], target: int) -> List[int]:\n        seen = {}\n        for i, value in enumerate(nums):\n            if target - value in seen:\n                return [seen[target-value], i]\n            seen[value] = i",
        ),
        (
            Language::Java,
            "class Solution {public int[] twoSum(int[] nums, int target){Map<Integer,Integer> seen = new HashMap<>();for(int i=0;i<nums.length;i++){if(seen.containsKey(target-nums[i]))return new int[]{i,seen.get(target-nums[i])};seen.put(nums[i],i);}return new int[0];}}",
        ),
    ] {
        let res = judge
            .evaluate(GradeRequest {
                language: lang,
                source: src.to_string(),
                entrypoint: Entrypoint::TwoSum,
                test_cases: cases_two_sum.clone(),
            })
            .await
            .expect("evaluate twoSum");
        assert_eq!(res.verdict, Verdict::Accepted, "twoSum {lang:?}: {res:?}");
    }

    let cases_valid = vec![
        test_case("()\n", Some("true"), false),
        test_case("\n", Some("true"), false),
        test_case("(\n", Some("false"), false),
    ];
    for (lang, src) in [
        (
            Language::Python,
            "class Solution:\n    def isValid(self, s: str) -> bool:\n        return s in (\"\", \"()\")",
        ),
        (
            Language::Java,
            "class Solution {public boolean isValid(String s){return s.equals(\"\") || s.equals(\"()\");}}",
        ),
    ] {
        let res = judge
            .evaluate(GradeRequest {
                language: lang,
                source: src.to_string(),
                entrypoint: Entrypoint::IsValid,
                test_cases: cases_valid.clone(),
            })
            .await
            .expect("evaluate isValid");
        assert_eq!(res.verdict, Verdict::Accepted, "isValid {lang:?}: {res:?}");
    }

    let cases_solve = vec![test_case("2 3\n", Some("5"), false)];
    for (lang, src) in [
        (
            Language::Python,
            "class Solution:\n    def solve(self, input: str) -> str:\n        return str(sum(map(int, input.split())))",
        ),
        (
            Language::Java,
            "class Solution {public String solve(String input){Scanner s=new Scanner(input);return \"\"+(s.nextInt()+s.nextInt());}}",
        ),
    ] {
        let res = judge
            .evaluate(GradeRequest {
                language: lang,
                source: src.to_string(),
                entrypoint: Entrypoint::Solve,
                test_cases: cases_solve.clone(),
            })
            .await
            .expect("evaluate solve");
        assert_eq!(res.verdict, Verdict::Accepted, "solve {lang:?}: {res:?}");
    }
}

#[tokio::test]
async fn resource_limits_tle_ole_mle_and_fle() {
    let judge = JudgePipeline::default();

    let tle = judge
        .evaluate(GradeRequest {
            language: Language::Python,
            source: "while True: pass".to_string(),
            entrypoint: Entrypoint::Stdin,
            test_cases: vec![test_case("", Some("5"), false)],
        })
        .await
        .expect("evaluate TLE");
    assert_eq!(tle.verdict, Verdict::TimeLimitExceeded);

    let ole = judge
        .evaluate(GradeRequest {
            language: Language::Python,
            source: "print(\"a\"*100000)".to_string(),
            entrypoint: Entrypoint::Stdin,
            test_cases: vec![test_case("", Some("5"), false)],
        })
        .await
        .expect("evaluate OLE");
    assert_eq!(ole.verdict, Verdict::OutputLimitExceeded);

    let mle = judge
        .evaluate(GradeRequest {
            language: Language::Python,
            source: "a = bytearray(450 * 1024 * 1024)\nfor i in range(0, len(a), 4096): a[i] = 1\nprint('ok')".to_string(),
            entrypoint: Entrypoint::Stdin,
            test_cases: vec![test_case("", Some("ok"), false)],
        })
        .await
        .expect("evaluate MLE");
    assert_eq!(mle.verdict, Verdict::MemoryLimitExceeded);

    let fle = judge
        .evaluate(GradeRequest {
            language: Language::Python,
            source: "for i in range(65):\n    open(f'f{i}.txt', 'w').write('x')\nprint('ok')".to_string(),
            entrypoint: Entrypoint::Stdin,
            test_cases: vec![test_case("", Some("ok"), false)],
        })
        .await
        .expect("evaluate FLE");
    assert_eq!(fle.verdict, Verdict::FileLimitExceeded);
}

#[tokio::test]
async fn os_sandbox_blocks_private_files_network_and_fork() {
    let judge = JudgePipeline::default();
    let dir = tempfile::Builder::new()
        .prefix("vedam-private-")
        .tempdir()
        .expect("create private dir");
    let secret = dir.path().join("private.txt");
    std::fs::write(&secret, "must stay private").expect("write secret");
    let secret_json = serde_json::to_string(&secret.to_string_lossy().as_ref()).expect("json");

    let source = format!(
        r#"import socket,os
try:
    open({secret_json}).read()
    print("file-open")
except PermissionError:
    print("file-blocked")
try:
    socket.socket().connect(("127.0.0.1",9))
    print("network-open")
except PermissionError:
    print("network-blocked")
try:
    os.fork()
    print("fork-open")
except PermissionError:
    print("fork-blocked")"#
    );

    let res = judge
        .evaluate(GradeRequest {
            language: Language::Python,
            source,
            entrypoint: Entrypoint::Stdin,
            test_cases: vec![test_case(
                "",
                Some("file-blocked\nnetwork-blocked\nfork-blocked"),
                false,
            )],
        })
        .await
        .expect("evaluate sandbox blocks");
    assert_eq!(res.verdict, Verdict::Accepted, "sandbox result: {res:?}");
}
