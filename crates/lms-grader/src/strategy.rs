use std::collections::HashMap;
use std::sync::Arc;

use crate::model::Language;

#[derive(Debug, Clone)]
pub struct RuntimePaths {
    pub python: String,
    pub java: String,
    pub javac: String,
}

impl RuntimePaths {
    pub fn executables(&self) -> [&str; 3] {
        [&self.python, &self.java, &self.javac]
    }
}

#[derive(Debug, Clone)]
pub struct CommandSpec {
    pub program: String,
    pub args: Vec<String>,
    pub timeout_ms: u64,
    pub cpu_limit_secs: u64,
}

pub trait LanguageStrategy: Send + Sync {
    fn source_filename(&self) -> &'static str;
    fn compile_command(&self, runtime: &RuntimePaths) -> Option<CommandSpec>;
    fn run_command(&self, runtime: &RuntimePaths) -> CommandSpec;
    fn rlimit_nofile(&self) -> u64 {
        64
    }
}

pub struct PythonStrategy;

impl LanguageStrategy for PythonStrategy {
    fn source_filename(&self) -> &'static str {
        "solution.py"
    }

    fn compile_command(&self, _runtime: &RuntimePaths) -> Option<CommandSpec> {
        None
    }

    fn run_command(&self, runtime: &RuntimePaths) -> CommandSpec {
        CommandSpec {
            program: runtime.python.clone(),
            args: vec![
                "-I".to_string(),
                "-S".to_string(),
                "-B".to_string(),
                "-u".to_string(),
                "solution.py".to_string(),
            ],
            timeout_ms: 3000,
            cpu_limit_secs: 2,
        }
    }
}

pub struct JavaStrategy;

impl LanguageStrategy for JavaStrategy {
    fn source_filename(&self) -> &'static str {
        "Main.java"
    }

    fn compile_command(&self, runtime: &RuntimePaths) -> Option<CommandSpec> {
        Some(CommandSpec {
            program: runtime.javac.clone(),
            args: vec![
                "-J-Xmx128m".to_string(),
                "-J-XX:+UseSerialGC".to_string(),
                "-J-XX:ActiveProcessorCount=2".to_string(),
                "-J-XX:-UsePerfData".to_string(),
                "-proc:none".to_string(),
                "Main.java".to_string(),
            ],
            timeout_ms: 10000,
            cpu_limit_secs: 8,
        })
    }

    fn run_command(&self, runtime: &RuntimePaths) -> CommandSpec {
        CommandSpec {
            program: runtime.java.clone(),
            args: vec![
                "-XX:+UseSerialGC".to_string(),
                "-XX:ActiveProcessorCount=2".to_string(),
                "-Xms16m".to_string(),
                "-Xmx128m".to_string(),
                "-XX:MaxMetaspaceSize=96m".to_string(),
                "-XX:ReservedCodeCacheSize=32m".to_string(),
                "-XX:-UsePerfData".to_string(),
                "-Djava.io.tmpdir=.".to_string(),
                "-cp".to_string(),
                ".".to_string(),
                "Main".to_string(),
            ],
            timeout_ms: 3000,
            cpu_limit_secs: 2,
        }
    }

    fn rlimit_nofile(&self) -> u64 {
        128
    }
}

#[derive(Clone)]
pub struct StrategyRegistry {
    strategies: HashMap<Language, Arc<dyn LanguageStrategy>>,
}

impl Default for StrategyRegistry {
    fn default() -> Self {
        Self::with_defaults()
    }
}

impl StrategyRegistry {
    pub fn with_defaults() -> Self {
        let mut strategies: HashMap<Language, Arc<dyn LanguageStrategy>> = HashMap::new();
        strategies.insert(Language::Python, Arc::new(PythonStrategy));
        strategies.insert(Language::Java, Arc::new(JavaStrategy));
        Self { strategies }
    }

    pub fn register(&mut self, language: Language, strategy: Arc<dyn LanguageStrategy>) {
        self.strategies.insert(language, strategy);
    }

    pub fn get(&self, language: Language) -> Option<&Arc<dyn LanguageStrategy>> {
        self.strategies.get(&language)
    }
}
