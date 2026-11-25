# Phase 3 — Rust Sandboxed Code Grader (`crates/lms-grader`)

> **Prerequisite**: [Phase 2](file:///Users/sirwagyashekhar/Projects/LMS/docs/phases/PHASE-2-GO-API-REDIS-AND-PARITY.md) complete.
> **Phase Goal**: Replace [`server/coding/sandbox.mjs`](file:///Users/sirwagyashekhar/Projects/LMS/server/coding/sandbox.mjs), [`server/coding/judge.mjs`](file:///Users/sirwagyashekhar/Projects/LMS/server/coding/judge.mjs), [`server/coding/languages.mjs`](file:///Users/sirwagyashekhar/Projects/LMS/server/coding/languages.mjs), [`server/coding/harness.mjs`](file:///Users/sirwagyashekhar/Projects/LMS/server/coding/harness.mjs), and `launcher.py` with a standalone, memory-safe Rust crate (`crates/lms-grader`) usable both as an IPC subprocess by the Go backend and as an embedded local practice runner inside the macOS Tauri app.

---

## 1. Design Patterns & SOLID Mapping

1. **Open/Closed Principle + Strategy Registry (`strategy.rs`)**:
   - `LanguageStrategy` trait defines `compile_cmd(&self, runtime: &RuntimePaths)` and `run_cmd(&self, runtime: &RuntimePaths)`.
   - `StrategyRegistry` holds a `HashMap<Language, Arc<dyn LanguageStrategy>>`. Adding C++20 or Go support registers a new strategy without altering `JudgePipeline`.
2. **Template Method (`pipeline.rs`)**:
   - `JudgePipeline::evaluate(&self, req: GradeRequest)` defines the invariant grading sequence:
     1. Acquire concurrency permit (`tokio::sync::Semaphore`, max 2 concurrent sandboxes).
     2. Allocate RAII `SandboxDir` (`mkdtemp` + canonical `realpath`).
     3. Generate platform-owned harness source (`harness::wrap_solution`) and macOS `.sb` Seatbelt policy.
     4. Compile if strategy requires compilation (fail fast with `Compilation Error` + sanitized stderr).
     5. Execute each test case against the compiled artifact / interpreter with wall-clock + RSS + disk + output caps.
     6. Aggregate per-test statuses and final verdict (`Accepted`, `Executed`, `Wrong Answer`, `Time Limit Exceeded`, `Memory Limit Exceeded`, `Output Limit Exceeded`, `File Limit Exceeded`, `Runtime Error`, `Compilation Error`).
     7. Automatically drop `SandboxDir` and `SIGKILL` process groups on return or panic.
3. **RAII Resource Safety (`sandbox.rs`)**:
   - `ProcessGuard` and `SandboxDir` implement `Drop` so no orphaned child process group (`killpg`) or temp directory can leak even under timeout or cancellation.

---

## 2. Rust Crate Layout & Core Types (`crates/lms-grader`)

### 2.1 `crates/lms-grader/Cargo.toml`

```toml
[package]
name = "lms-grader"
version = "0.1.0"
edition = "2024"

[dependencies]
libc = "0.2"
serde = { version = "1.0", features = ["derive"] }
serde_json = "1.0"
tempfile = "3.13"
thiserror = "2.0"
tokio = { version = "1.41", features = ["full"] }
```

### 2.2 Domain & IPC Contracts (`src/model.rs`)

Matches the JSON request/response contract consumed by [`src/portal/coding/types.ts`](file:///Users/sirwagyashekhar/Projects/LMS/src/portal/coding/types.ts):

```rust
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Deserialize, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Language {
    Python,
    Java,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub enum Entrypoint {
    TwoSum,
    IsValid,
    Solve,
    Stdin,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TestCaseInput {
    pub input: String,
    pub expected: Option<String>,
    pub hidden: bool,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GradeRequest {
    pub language: Language,
    pub source: String,
    pub entrypoint: Entrypoint,
    pub test_cases: Vec<TestCaseInput>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub enum Verdict {
    #[serde(rename = "Accepted")]
    Accepted,
    #[serde(rename = "Executed")]
    Executed,
    #[serde(rename = "Wrong Answer")]
    WrongAnswer,
    #[serde(rename = "Compilation Error")]
    CompilationError,
    #[serde(rename = "Runtime Error")]
    RuntimeError,
    #[serde(rename = "Time Limit Exceeded")]
    TimeLimitExceeded,
    #[serde(rename = "Memory Limit Exceeded")]
    MemoryLimitExceeded,
    #[serde(rename = "Output Limit Exceeded")]
    OutputLimitExceeded,
    #[serde(rename = "File Limit Exceeded")]
    FileLimitExceeded,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CaseResult {
    pub index: usize,
    pub hidden: bool,
    pub status: Verdict,
    pub duration_ms: u64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub input: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expected: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stdout: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stderr: Option<String>,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GradeResponse {
    pub verdict: Verdict,
    pub passed: usize,
    pub total: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub compile_error: Option<String>,
    pub results: Vec<CaseResult>,
}
```

### 2.3 Strategy Trait & Registry (`src/strategy.rs`)

```rust
use std::collections::HashMap;
use std::sync::Arc;
use crate::model::Language;

#[derive(Debug, Clone)]
pub struct RuntimePaths {
    pub python: String,
    pub java: String,
    pub javac: String,
}

#[derive(Debug, Clone)]
pub struct CommandSpec {
    pub program: String,
    pub args: Vec<String>,
    pub timeout_ms: u64,
}

pub trait LanguageStrategy: Send + Sync {
    fn source_filename(&self) -> &'static str;
    fn compile_command(&self, runtime: &RuntimePaths) -> Option<CommandSpec>;
    fn run_command(&self, runtime: &RuntimePaths) -> CommandSpec;
    fn rlimit_nofile(&self) -> u64 { 64 }
}

pub struct PythonStrategy;

impl LanguageStrategy for PythonStrategy {
    fn source_filename(&self) -> &'static str { "solution.py" }

    fn compile_command(&self, _runtime: &RuntimePaths) -> Option<CommandSpec> { None }

    fn run_command(&self, runtime: &RuntimePaths) -> CommandSpec {
        CommandSpec {
            program: runtime.python.clone(),
            args: vec![
                "-I".into(),
                "-S".into(),
                "-B".into(),
                "-u".into(),
                "solution.py".into(),
            ],
            timeout_ms: 3000,
        }
    }
}

pub struct JavaStrategy;

impl LanguageStrategy for JavaStrategy {
    fn source_filename(&self) -> &'static str { "Main.java" }

    fn compile_command(&self, runtime: &RuntimePaths) -> Option<CommandSpec> {
        Some(CommandSpec {
            program: runtime.javac.clone(),
            args: vec![
                "-J-Xmx128m".into(),
                "-J-XX:+UseSerialGC".into(),
                "-J-XX:ActiveProcessorCount=2".into(),
                "-J-XX:-UsePerfData".into(),
                "-proc:none".into(),
                "Main.java".into(),
            ],
            timeout_ms: 10000,
        })
    }

    fn run_command(&self, runtime: &RuntimePaths) -> CommandSpec {
        CommandSpec {
            program: runtime.java.clone(),
            args: vec![
                "-XX:+UseSerialGC".into(),
                "-XX:ActiveProcessorCount=2".into(),
                "-Xms16m".into(),
                "-Xmx128m".into(),
                "-XX:MaxMetaspaceSize=96m".into(),
                "-XX:ReservedCodeCacheSize=32m".into(),
                "-XX:-UsePerfData".into(),
                "-Djava.io.tmpdir=.".into(),
                "-cp".into(),
                ".".into(),
                "Main".into(),
            ],
            timeout_ms: 3000,
        }
    }

    fn rlimit_nofile(&self) -> u64 { 128 }
}

pub struct StrategyRegistry {
    entries: HashMap<Language, Arc<dyn LanguageStrategy>>,
}

impl StrategyRegistry {
    pub fn with_defaults() -> Self {
        let mut entries: HashMap<Language, Arc<dyn LanguageStrategy>> = HashMap::new();
        entries.insert(Language::Python, Arc::new(PythonStrategy));
        entries.insert(Language::Java, Arc::new(JavaStrategy));
        Self { entries }
    }

    pub fn register(&mut self, language: Language, strategy: Arc<dyn LanguageStrategy>) {
        self.entries.insert(language, strategy);
    }

    pub fn get(&self, language: Language) -> Option<&Arc<dyn LanguageStrategy>> {
        self.entries.get(&language)
    }
}
```

---

## 3. Platform-Owned Solution Harness (`src/harness.rs`)

Matches the contracts in [`server/coding/harness.mjs`](file:///Users/sirwagyashekhar/Projects/LMS/server/coding/harness.mjs) so student submissions cannot tamper with driver I/O:

- **`Entrypoint::TwoSum`**:
  - Python: wraps `class Solution` with `typing`, `collections`, `heapq`, `math`, `bisect`, `functools`, `itertools`, reads `n, target` + `nums`, validates tuple/list of length 2, prints `*sorted(result)`.
  - Java: imports `java.util.*`, `java.io.*`, parses `n`, `target`, `nums[]`, checks `result.length == 2`, sorts, prints `result[0] + " " + result[1]`.
- **`Entrypoint::IsValid`**:
  - Invokes `Solution().isValid(s)` (stripping trailing CR/LF), prints lowercase `true` or `false`.
- **`Entrypoint::Solve`**:
  - Passes raw `_input` string to `Solution().solve(input)`, verifies string return value, prints to stdout.
- **`Entrypoint::Stdin`**:
  - Preserves student source unmodified for standard stdin/stdout challenges.

---

## 4. OS & Resource Sandbox Isolation (`src/sandbox.rs`)

Every execution spawns inside an isolated process group (`unsafe { libc::setpgid(0, 0) }`) with strict kernel + user-space limits:

### 4.1 macOS Seatbelt Profile (`.sb`)

```scheme
(version 1)
(deny default)
(allow file-read-metadata)
(allow file-map-executable)
(allow file-read*
    (literal "/")
    (subpath "/System")
    (subpath "/usr")
    (subpath "/Library/Java")
    (subpath "/opt/homebrew")
    (subpath "/private/preboot")
    (subpath "/private/var/db/dyld"))
(allow file-read* file-write* (subpath "<CANONICAL_SANDBOX_DIR>"))
(allow file-read* file-write* (literal "/dev/null"))
(allow file-read* (literal "/dev/random") (literal "/dev/urandom"))
(allow process-exec (literal "<PYTHON_BIN>") (literal "<JAVA_BIN>") (literal "<JAVAC_BIN>"))
(allow sysctl-read)
(allow signal (target self))
```

### 4.2 Hard Resource Caps (Applied Pre-Exec + Async Watchdog)

| Resource Cap | Enforcement Mechanism | Limit Value | Mapped Verdict |
| :--- | :--- | :--- | :--- |
| **Network Access** | macOS `sandbox-exec` `(deny default)` | Completely blocked (`EPERM`) | `Runtime Error` |
| **Filesystem Writes** | Seatbelt `(allow file-read* file-write* (subpath <sandbox_dir>))` | Temp sandbox dir only; max `60` files / `5 MB` | `File Limit Exceeded` |
| **CPU Time** | `setrlimit(RLIMIT_CPU)` inside `Command::pre_exec` | `10s` compile / `3s` run | `Time Limit Exceeded` |
| **Wall-Clock Time** | `tokio::time::timeout` + `killpg(-pid, SIGKILL)` | `10s` compile / `3s` run | `Time Limit Exceeded` |
| **Memory (RSS)** | Watchdog polling `proc_pidinfo` (`rusage_v2`) every `50 ms` | `384 MB` | `Memory Limit Exceeded` |
| **File Creation Size** | `setrlimit(RLIMIT_FSIZE)` inside `pre_exec` | `5 MB` | `File Limit Exceeded` |
| **Stdio Output** | Non-blocking stream reader truncation | `64 KB` combined (`stdout + stderr`) | `Output Limit Exceeded` |
| **Environment** | `cmd.env_clear()` with minimal `PATH`, `HOME=dir`, `TMPDIR=dir` | Zero secret/host env vars leaked | — |

---

## 5. Integration with Go Backend (`backend/internal/adapters/grader/`)

Go depends on a narrow interface (`assessment.CodeJudge`):

```go
type CodeJudge interface {
	Evaluate(ctx context.Context, req JudgeInput) (JudgeOutput, error)
}
```

- **Local Mode (`SubprocessGrader`)**: Invokes `./target/release/lms-grader --json` via `exec.CommandContext`, writing `GradeRequest` to stdin and decoding `GradeResponse` from stdout.
- **Distributed Mode (`RedisStreamGrader`, scale trigger)**: `cmd/api` pushes jobs onto `stream:grading`, `cmd/worker` pops them, runs `lms-grader`, updates PostgreSQL, and pushes completion over `pubsub:user:{userId}` SSE.

---

## 6. Phase 3 Deliverables & Verification Gate

- **Deliverables**:
  - Workspace `Cargo.toml` at repo root
  - `crates/lms-grader/Cargo.toml` & `src/{lib,main,model,strategy,harness,sandbox,pipeline}.rs`
  - `backend/internal/adapters/grader/rust_ipc.go`
- **Verification Security & Parity Test Matrix (`cargo test`)**:
  1. `two_sum_python_and_java_accepted` (public + hidden cases pass, hidden fields redacted for student responses)
  2. `blocks_outbound_network_socket` (Python `socket.create_connection(("1.1.1.1", 80))` rejected with `EPERM`)
  3. `blocks_parent_filesystem_escape` (attempt to read `/etc/passwd` or `../` fails inside sandbox)
  4. `kills_infinite_loop_within_wall_timeout` -> `Time Limit Exceeded` + verifies temp dir & child PID cleanup
  5. `kills_memory_bomb` -> `Memory Limit Exceeded`
  6. `kills_fork_or_output_flood` -> `Output Limit Exceeded` / `File Limit Exceeded`
  7. `compile_error_for_malformed_java` -> `Compilation Error`
- **Verification Commands**:
  ```bash
  cargo test -p lms-grader -- --test-threads=2
  cargo build --release -p lms-grader
  cd backend && GRADER_BINARY_PATH=../target/release/lms-grader go test -race ./internal/adapters/grader/...
  ```
