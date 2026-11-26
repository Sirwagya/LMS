use std::sync::Arc;

use tokio::sync::Semaphore;

use crate::harness::wrap_solution;
use crate::model::{CaseResult, GradeRequest, GradeResult, GraderError, Verdict};
use crate::sandbox::{
    MacOSRuntimeResolver, MacOSSandboxExecutor, RuntimeResolver, SandboxExecutor, SandboxWorkspace,
};
use crate::strategy::StrategyRegistry;

pub struct JudgePipeline {
    registry: StrategyRegistry,
    runtime_resolver: Arc<dyn RuntimeResolver>,
    executor: Arc<dyn SandboxExecutor>,
    concurrency: Semaphore,
}

impl Default for JudgePipeline {
    fn default() -> Self {
        Self::new(
            StrategyRegistry::with_defaults(),
            Arc::new(MacOSRuntimeResolver),
            Arc::new(MacOSSandboxExecutor),
            2,
        )
    }
}

impl JudgePipeline {
    pub fn new(
        registry: StrategyRegistry,
        runtime_resolver: Arc<dyn RuntimeResolver>,
        executor: Arc<dyn SandboxExecutor>,
        max_concurrent: usize,
    ) -> Self {
        Self {
            registry,
            runtime_resolver,
            executor,
            concurrency: Semaphore::new(max_concurrent),
        }
    }

    /// Template Method orchestrating validation -> RAII sandbox setup -> compile -> per-case execution -> verdict aggregation.
    pub async fn evaluate(&self, request: GradeRequest) -> Result<GradeResult, GraderError> {
        let strategy = self
            .registry
            .get(request.language)
            .ok_or_else(|| GraderError::Validation("Choose Java or Python.".to_string()))?;

        if request.source.is_empty()
            || request.source.len() > 80_000
            || !(1..=10).contains(&request.test_cases.len())
            || request.test_cases.iter().any(|test| {
                test.input.len() > 40_000
                    || test
                        .expected
                        .as_ref()
                        .is_some_and(|value| value.len() > 40_000)
            })
        {
            return Err(GraderError::Validation(
                "Source or test cases exceed the grading limits.".into(),
            ));
        }

        let _permit = self
            .concurrency
            .try_acquire()
            .map_err(|_| GraderError::Busy)?;

        let runtime = self.runtime_resolver.resolve()?;

        let sandbox = SandboxWorkspace::create()
            .map_err(|e| GraderError::Unavailable(format!("Create sandbox workspace: {e}")))?;

        let wrapped = wrap_solution(request.language, &request.source, request.entrypoint);
        sandbox
            .write_file(strategy.source_filename(), &wrapped)
            .await
            .map_err(|e| GraderError::Unavailable(format!("Prepare sandbox files: {e}")))?;

        let nofile = strategy.rlimit_nofile();
        if let Some(compile_spec) = strategy.compile_command(&runtime) {
            let outcome = self
                .executor
                .execute(&sandbox, &runtime, &compile_spec, nofile, "")
                .await
                .map_err(|e| GraderError::Unavailable(format!("Run compiler: {e}")))?;

            if outcome.exit_code != Some(0) {
                let verdict = if outcome.status == Verdict::RuntimeError {
                    Verdict::CompilationError
                } else {
                    outcome.status
                };
                let msg = if !outcome.stderr.is_empty() {
                    outcome.stderr
                } else if !outcome.stdout.is_empty() {
                    outcome.stdout
                } else {
                    "Compiler could not start inside the sandbox.".to_string()
                };
                return Ok(GradeResult {
                    verdict,
                    passed: 0,
                    total: request.test_cases.len(),
                    compile_error: Some(msg),
                    results: Vec::new(),
                });
            }
        }

        let run_spec = strategy.run_command(&runtime);
        let mut results = Vec::with_capacity(request.test_cases.len());

        for (i, test) in request.test_cases.iter().enumerate() {
            let outcome = self
                .executor
                .execute(&sandbox, &runtime, &run_spec, nofile, &test.input)
                .await
                .map_err(|e| GraderError::Unavailable(format!("Run test case: {e}")))?;

            let status = if outcome.status != Verdict::Executed || outcome.exit_code != Some(0) {
                outcome.status
            } else {
                match &test.expected {
                    None => Verdict::Executed,
                    Some(exp) if normalize(&outcome.stdout) == normalize(exp) => Verdict::Accepted,
                    Some(_) => Verdict::WrongAnswer,
                }
            };

            let case_result = if test.hidden {
                CaseResult {
                    index: i + 1,
                    hidden: true,
                    status,
                    duration_ms: outcome.duration_ms,
                    input: None,
                    expected: None,
                    stdout: None,
                    stderr: None,
                }
            } else {
                CaseResult {
                    index: i + 1,
                    hidden: false,
                    status,
                    duration_ms: outcome.duration_ms,
                    input: Some(test.input.clone()),
                    expected: test.expected.clone(),
                    stdout: Some(outcome.stdout),
                    stderr: Some(outcome.stderr),
                }
            };
            results.push(case_result);

            if status.is_resource_limit() {
                break;
            }
        }

        let passed = results
            .iter()
            .filter(|r| r.status == Verdict::Accepted)
            .count();

        let verdict = results
            .iter()
            .find(|r| !r.status.is_passing())
            .map(|r| r.status)
            .unwrap_or_else(|| {
                if request.test_cases.iter().any(|t| t.expected.is_none()) {
                    Verdict::Executed
                } else {
                    Verdict::Accepted
                }
            });

        Ok(GradeResult {
            verdict,
            passed,
            total: request.test_cases.len(),
            compile_error: None,
            results,
        })
    }
}

fn normalize(value: &str) -> String {
    value.replace("\r\n", "\n").trim_end().to_string()
}
