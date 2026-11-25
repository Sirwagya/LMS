use serde::{Deserialize, Deserializer, Serialize};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Deserialize, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Language {
    Python,
    Java,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize)]
pub enum Entrypoint {
    #[serde(rename = "twoSum")]
    TwoSum,
    #[serde(rename = "isValid")]
    IsValid,
    #[serde(rename = "solve")]
    Solve,
    #[default]
    #[serde(rename = "stdin")]
    Stdin,
}

impl<'de> Deserialize<'de> for Entrypoint {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        let opt = Option::<String>::deserialize(deserializer)?;
        match opt.as_deref().unwrap_or("") {
            "" | "stdin" => Ok(Entrypoint::Stdin),
            "twoSum" => Ok(Entrypoint::TwoSum),
            "isValid" => Ok(Entrypoint::IsValid),
            "solve" => Ok(Entrypoint::Solve),
            other => Err(serde::de::Error::custom(format!(
                "Unknown solution method contract: {other}"
            ))),
        }
    }
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TestCaseInput {
    pub input: String,
    #[serde(default)]
    pub expected: Option<String>,
    #[serde(default)]
    pub hidden: bool,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GradeRequest {
    pub language: Language,
    pub source: String,
    #[serde(default)]
    pub entrypoint: Entrypoint,
    pub test_cases: Vec<TestCaseInput>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
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

impl Verdict {
    pub fn is_resource_limit(self) -> bool {
        matches!(
            self,
            Verdict::TimeLimitExceeded
                | Verdict::MemoryLimitExceeded
                | Verdict::OutputLimitExceeded
                | Verdict::FileLimitExceeded
        )
    }

    pub fn is_passing(self) -> bool {
        matches!(self, Verdict::Accepted | Verdict::Executed)
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
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

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GradeResult {
    pub verdict: Verdict,
    pub passed: usize,
    pub total: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub compile_error: Option<String>,
    pub results: Vec<CaseResult>,
}

#[derive(Debug, Clone, Serialize)]
pub struct IpcEnvelope {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub result: Option<GradeResult>,
    pub status: u16,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub message: Option<String>,
}

impl IpcEnvelope {
    pub fn ok(result: GradeResult) -> Self {
        Self {
            result: Some(result),
            status: 0,
            message: None,
        }
    }

    pub fn err(status: u16, message: impl Into<String>) -> Self {
        Self {
            result: None,
            status,
            message: Some(message.into()),
        }
    }
}

#[derive(Debug, thiserror::Error)]
pub enum GraderError {
    #[error("{0}")]
    Validation(String),
    #[error("The local runner is busy. Please try again shortly.")]
    Busy,
    #[error("{0}")]
    Unavailable(String),
}

impl GraderError {
    pub fn status_code(&self) -> u16 {
        match self {
            GraderError::Validation(_) => 400,
            GraderError::Busy => 429,
            GraderError::Unavailable(_) => 503,
        }
    }
}
