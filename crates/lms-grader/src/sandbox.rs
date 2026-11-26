use std::ffi::OsStr;
use std::fs;
use std::io;
use std::os::unix::process::{CommandExt, ExitStatusExt};
use std::path::{Path, PathBuf};
use std::process::{Command as StdCommand, ExitStatus, Stdio};
use std::sync::{Arc, OnceLock};
use std::time::{Duration, Instant};

use tempfile::TempDir;
use tokio::io::{AsyncRead, AsyncReadExt, AsyncWriteExt};
use tokio::process::{ChildStderr, ChildStdin, ChildStdout};
use tokio::sync::{Mutex, watch};

use crate::model::{GraderError, Verdict};
use crate::strategy::{CommandSpec, RuntimePaths};

const MAX_RSS_BYTES: u64 = 384 * 1024 * 1024;
const MAX_OUTPUT_BYTES: usize = 65_536;
const TRUNCATED_STREAM_BYTES: usize = 32_000;
const MAX_SANDBOX_FILES: usize = 60;
const MAX_SANDBOX_BYTES: u64 = 5 * 1024 * 1024;

static CACHED_RUNTIME: OnceLock<Result<RuntimePaths, String>> = OnceLock::new();

#[derive(Debug, Clone)]
pub struct ExecutionOutcome {
    pub exit_code: Option<i32>,
    pub stdout: String,
    pub stderr: String,
    pub status: Verdict,
    pub duration_ms: u64,
}

pub trait RuntimeResolver: Send + Sync {
    fn resolve(&self) -> Result<RuntimePaths, GraderError>;
}

pub struct MacOSRuntimeResolver;

impl RuntimeResolver for MacOSRuntimeResolver {
    fn resolve(&self) -> Result<RuntimePaths, GraderError> {
        let cached = CACHED_RUNTIME.get_or_init(detect_macos_runtime);
        cached.clone().map_err(GraderError::Unavailable)
    }
}

fn detect_macos_runtime() -> Result<RuntimePaths, String> {
    if !cfg!(target_os = "macos") {
        return Err(
            "This local runner requires macOS sandbox-exec. No unrestricted fallback is enabled."
                .to_string(),
        );
    }

    let python_candidate = std::env::var("LMS_PYTHON").unwrap_or_else(|_| {
        if Path::new("/opt/homebrew/bin/python3").exists() {
            "/opt/homebrew/bin/python3".to_string()
        } else {
            "/usr/bin/python3".to_string()
        }
    });
    if !Path::new(&python_candidate).is_absolute() {
        return Err("LMS_PYTHON must be an absolute path.".into());
    }
    let mut python = fs::canonicalize(&python_candidate)
        .map_err(|e| format!("Python runtime unavailable at {python_candidate}: {e}"))?;

    if let Ok(output) = std::process::Command::new(&python)
        .args(["-I", "-S", "-c", "import sys; print(sys.prefix)"])
        .output()
        && output.status.success()
    {
        let prefix = String::from_utf8_lossy(&output.stdout).trim().to_string();
        let framework = PathBuf::from(&prefix).join("Resources/Python.app/Contents/MacOS/Python");
        if framework.exists()
            && let Ok(canon) = fs::canonicalize(&framework)
        {
            python = canon;
        }
    }

    let java_home = match std::env::var("JAVA_HOME") {
        Ok(val) if !val.trim().is_empty() => val,
        _ => {
            let out = std::process::Command::new("/usr/libexec/java_home")
                .output()
                .map_err(|e| format!("Failed to locate Java runtime: {e}"))?;
            if !out.status.success() {
                return Err("Java runtime is not installed on this machine.".to_string());
            }
            String::from_utf8_lossy(&out.stdout).trim().to_string()
        }
    };

    let java = fs::canonicalize(Path::new(&java_home).join("bin/java"))
        .map_err(|e| format!("java binary unavailable: {e}"))?;
    let javac = fs::canonicalize(Path::new(&java_home).join("bin/javac"))
        .map_err(|e| format!("javac binary unavailable: {e}"))?;

    Ok(RuntimePaths {
        python: python.to_string_lossy().into_owned(),
        java: java.to_string_lossy().into_owned(),
        javac: javac.to_string_lossy().into_owned(),
    })
}

/// RAII temporary sandbox directory guaranteeing cleanup on normal exit or panic.
pub struct SandboxWorkspace {
    _temp_dir: TempDir,
    canonical_path: PathBuf,
}

impl SandboxWorkspace {
    pub fn create() -> io::Result<Self> {
        let temp_dir = tempfile::Builder::new().prefix("vedam-code-").tempdir()?;
        let canonical_path = fs::canonicalize(temp_dir.path())?;
        Ok(Self {
            _temp_dir: temp_dir,
            canonical_path,
        })
    }

    pub fn path(&self) -> &Path {
        &self.canonical_path
    }

    pub async fn write_file(&self, name: impl AsRef<OsStr>, contents: &str) -> io::Result<()> {
        tokio::fs::write(self.canonical_path.join(name.as_ref()), contents).await
    }
}

/// RAII guard ensuring child process group receives SIGKILL if dropped mid-execution.
struct ProcessGroup {
    pid: libc::pid_t,
    kill_target: libc::pid_t,
    live: std::sync::Mutex<bool>,
}

impl ProcessGroup {
    fn kill(&self) {
        let live = self.live.lock().unwrap();
        if *live {
            unsafe {
                libc::kill(self.kill_target, libc::SIGKILL);
            }
        }
    }
}

struct ProcessGroupGuard(Arc<ProcessGroup>);

impl Drop for ProcessGroupGuard {
    fn drop(&mut self) {
        self.0.kill();
    }
}

fn build_seatbelt_profile(
    sandbox_dir: &Path,
    runtime: &RuntimePaths,
    spec: &CommandSpec,
) -> String {
    // Executing student code requires source/classes and stdin/stdout only.
    // The trusted compiler may create class files; annotation processors are disabled.
    let workspace_writes = if spec.program == runtime.javac {
        "file-write*"
    } else {
        ""
    };
    let dir_json = serde_json::to_string(&sandbox_dir.to_string_lossy().as_ref())
        .expect("path serializes to json string");
    let executable = serde_json::to_string(&spec.program).expect("program serializes");

    format!(
        r#"(version 1)
(deny default)
(allow file-read-metadata)
(allow file-map-executable)
(allow file-read* (literal "/") (subpath "/System") (subpath "/usr") (subpath "/Library/Java") (subpath "/opt/homebrew") (subpath "/private/preboot") (subpath "/private/var/db/dyld"))
(allow file-read* {workspace_writes} (subpath {dir_json}))
(allow file-read* file-write* (literal "/dev/null"))
(allow file-read* (literal "/dev/random") (literal "/dev/urandom"))
(allow process-exec (literal {executable}))
(allow sysctl-read)
(allow signal (target self))
"#
    )
}

pub trait SandboxExecutor: Send + Sync {
    fn execute<'a>(
        &'a self,
        sandbox: &'a SandboxWorkspace,
        runtime: &'a RuntimePaths,
        spec: &'a CommandSpec,
        rlimit_nofile: u64,
        stdin_data: &'a str,
    ) -> std::pin::Pin<
        Box<dyn std::future::Future<Output = io::Result<ExecutionOutcome>> + Send + 'a>,
    >;
}

pub struct MacOSSandboxExecutor;

impl SandboxExecutor for MacOSSandboxExecutor {
    fn execute<'a>(
        &'a self,
        sandbox: &'a SandboxWorkspace,
        runtime: &'a RuntimePaths,
        spec: &'a CommandSpec,
        rlimit_nofile: u64,
        stdin_data: &'a str,
    ) -> std::pin::Pin<
        Box<dyn std::future::Future<Output = io::Result<ExecutionOutcome>> + Send + 'a>,
    > {
        Box::pin(async move {
            run_sandboxed(sandbox.path(), runtime, spec, rlimit_nofile, stdin_data).await
        })
    }
}

async fn run_sandboxed(
    dir: &Path,
    runtime: &RuntimePaths,
    spec: &CommandSpec,
    rlimit_nofile: u64,
    stdin_data: &str,
) -> io::Result<ExecutionOutcome> {
    let start = Instant::now();
    let profile = build_seatbelt_profile(dir, runtime, spec);

    let mut cmd = StdCommand::new("/usr/bin/sandbox-exec");
    cmd.arg("-p")
        .arg(&profile)
        .arg(&spec.program)
        .args(&spec.args)
        .current_dir(dir)
        .env_clear()
        .env("PATH", "/usr/bin:/bin")
        .env("HOME", dir)
        .env("TMPDIR", dir)
        .env("LANG", "en_US.UTF-8")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());

    let cpu_secs = spec.cpu_limit_secs;
    // The API supervises the whole group. Each sandboxed interpreter is
    // prohibited from creating subprocesses, and watchdogs kill its PID only.
    let managed = std::env::var_os("LMS_GRADER_MANAGED_GROUP").is_some_and(|value| value == "1")
        && unsafe { libc::getpgrp() == libc::getpid() };
    unsafe {
        cmd.pre_exec(move || {
            if !managed && libc::setpgid(0, 0) != 0 {
                return Err(io::Error::last_os_error());
            }
            set_limit(libc::RLIMIT_CPU, cpu_secs, cpu_secs + 1)?;
            set_limit(libc::RLIMIT_FSIZE, MAX_SANDBOX_BYTES, MAX_SANDBOX_BYTES)?;
            set_limit(libc::RLIMIT_NOFILE, rlimit_nofile, rlimit_nofile)?;
            set_limit(libc::RLIMIT_CORE, 0, 0)?;
            Ok(())
        });
    }

    let mut child = cmd.spawn()?;
    let pid = child.id() as libc::pid_t;
    let std_stdin = child.stdin.take();
    let std_stdout = child.stdout.take();
    let std_stderr = child.stderr.take();
    drop(child);

    let group = Arc::new(ProcessGroup {
        pid,
        kill_target: if managed { pid } else { -pid },
        live: std::sync::Mutex::new(true),
    });
    let _pg_guard = ProcessGroupGuard(Arc::clone(&group));
    let group_wait = Arc::clone(&group);
    let mut wait_handle = tokio::task::spawn_blocking(move || wait4_child(&group_wait));

    if let Some(raw_stdin) = std_stdin {
        let mut stdin = ChildStdin::from_std(raw_stdin)?;
        let data = stdin_data.as_bytes().to_vec();
        tokio::spawn(async move {
            let _ = stdin.write_all(&data).await;
            let _ = stdin.shutdown().await;
        });
    }

    let limit_reason: Arc<Mutex<Option<Verdict>>> = Arc::new(Mutex::new(None));
    let total_output = Arc::new(std::sync::atomic::AtomicUsize::new(0));
    let (stop_tx, stop_rx) = watch::channel(false);

    let stdout_handle = match std_stdout {
        Some(raw) => {
            let reader = ChildStdout::from_std(raw)?;
            let reason = Arc::clone(&limit_reason);
            let counter = Arc::clone(&total_output);
            let group = Arc::clone(&group);
            Some(tokio::spawn(async move {
                read_capped(reader, counter, reason, group).await
            }))
        }
        None => None,
    };
    let stderr_handle = match std_stderr {
        Some(raw) => {
            let reader = ChildStderr::from_std(raw)?;
            let reason = Arc::clone(&limit_reason);
            let counter = Arc::clone(&total_output);
            let group = Arc::clone(&group);
            Some(tokio::spawn(async move {
                read_capped(reader, counter, reason, group).await
            }))
        }
        None => None,
    };

    // Background watchdog: RSS/phys_footprint memory cap + sandbox directory file/size cap
    let watchdog_reason = Arc::clone(&limit_reason);
    let watchdog_dir = dir.to_path_buf();
    let mut watchdog_stop = stop_rx.clone();
    let watchdog_group = Arc::clone(&group);
    let watchdog_handle = tokio::spawn(async move {
        let mut interval = tokio::time::interval(Duration::from_millis(20));
        loop {
            tokio::select! {
                _ = interval.tick() => {
                    if let Some(rss) = read_process_rss_bytes(pid)
                        && rss > MAX_RSS_BYTES
                    {
                        trip_limit(&watchdog_reason, Verdict::MemoryLimitExceeded, &watchdog_group).await;
                        break;
                    }
                    if exceeds_dir_quota(&watchdog_dir) {
                        trip_limit(&watchdog_reason, Verdict::FileLimitExceeded, &watchdog_group).await;
                        break;
                    }
                }
                _ = watchdog_stop.changed() => {
                    break;
                }
            }
        }
    });

    let wait_outcome = match tokio::time::timeout(
        Duration::from_millis(spec.timeout_ms),
        &mut wait_handle,
    )
    .await
    {
        Ok(join_res) => join_res.map_err(io::Error::other)??,
        Err(_) => {
            trip_limit(&limit_reason, Verdict::TimeLimitExceeded, &group).await;
            wait_handle.await.map_err(io::Error::other)??
        }
    };

    let _ = stop_tx.send(true);
    let _ = watchdog_handle.await;

    let mut stdout = match stdout_handle {
        Some(h) => h.await.unwrap_or_default(),
        None => String::new(),
    };
    let mut stderr = match stderr_handle {
        Some(h) => h.await.unwrap_or_default(),
        None => String::new(),
    };

    let mut reason = *limit_reason.lock().await;
    if reason.is_none() && wait_outcome.max_rss_bytes > MAX_RSS_BYTES {
        reason = Some(Verdict::MemoryLimitExceeded);
    }
    if reason.is_none() && exceeds_dir_quota(dir) {
        reason = Some(Verdict::FileLimitExceeded);
    }
    if reason == Some(Verdict::OutputLimitExceeded) {
        truncate_chars(&mut stdout, TRUNCATED_STREAM_BYTES);
        truncate_chars(&mut stderr, TRUNCATED_STREAM_BYTES);
    }

    let sig = wait_outcome.exit_status.signal();
    let code = wait_outcome.exit_status.code();
    let status = if let Some(verdict) = reason {
        verdict
    } else if sig == Some(libc::SIGXCPU) {
        Verdict::TimeLimitExceeded
    } else if sig == Some(libc::SIGXFSZ) {
        Verdict::FileLimitExceeded
    } else if code == Some(0) {
        Verdict::Executed
    } else {
        Verdict::RuntimeError
    };

    Ok(ExecutionOutcome {
        exit_code: code,
        stdout,
        stderr,
        status,
        duration_ms: start.elapsed().as_millis() as u64,
    })
}

struct Wait4Outcome {
    exit_status: ExitStatus,
    max_rss_bytes: u64,
}

fn wait4_child(group: &ProcessGroup) -> io::Result<Wait4Outcome> {
    let pid = group.pid;
    let mut raw_status: libc::c_int = 0;
    let mut rusage: libc::rusage = unsafe { std::mem::zeroed() };
    loop {
        let mut live = group.live.lock().unwrap();
        let rc = unsafe { libc::wait4(pid, &mut raw_status, libc::WNOHANG, &mut rusage) };
        if rc == 0 {
            drop(live);
            std::thread::sleep(Duration::from_millis(4));
            continue;
        }
        if rc == pid {
            *live = false;
            break;
        }
        if rc < 0 {
            let err = io::Error::last_os_error();
            if err.kind() == io::ErrorKind::Interrupted {
                continue;
            }
            *live = false;
            return Err(err);
        }
    }
    #[cfg(target_os = "macos")]
    let max_rss_bytes = rusage.ru_maxrss.max(0) as u64;
    #[cfg(not(target_os = "macos"))]
    let max_rss_bytes = rusage.ru_maxrss.max(0) as u64 * 1024;

    Ok(Wait4Outcome {
        exit_status: ExitStatus::from_raw(raw_status),
        max_rss_bytes,
    })
}

async fn read_capped<R: AsyncRead + Unpin>(
    mut reader: R,
    total_output: Arc<std::sync::atomic::AtomicUsize>,
    limit_reason: Arc<Mutex<Option<Verdict>>>,
    group: Arc<ProcessGroup>,
) -> String {
    let mut out = Vec::new();
    let mut buf = [0u8; 4096];
    loop {
        match reader.read(&mut buf).await {
            Ok(0) | Err(_) => break,
            Ok(n) => {
                out.extend_from_slice(&buf[..n]);
                let prev = total_output.fetch_add(n, std::sync::atomic::Ordering::Relaxed);
                if prev + n > MAX_OUTPUT_BYTES {
                    trip_limit(&limit_reason, Verdict::OutputLimitExceeded, &group).await;
                    break;
                }
            }
        }
    }
    String::from_utf8_lossy(&out).into_owned()
}

async fn trip_limit(reason_slot: &Mutex<Option<Verdict>>, verdict: Verdict, group: &ProcessGroup) {
    let mut guard = reason_slot.lock().await;
    if guard.is_none() {
        *guard = Some(verdict);
        group.kill();
    }
}

fn truncate_chars(s: &mut String, max_bytes: usize) {
    if s.len() <= max_bytes {
        return;
    }
    let mut idx = max_bytes;
    while !s.is_char_boundary(idx) && idx > 0 {
        idx -= 1;
    }
    s.truncate(idx);
}

#[cfg(target_os = "macos")]
fn read_process_rss_bytes(pid: libc::pid_t) -> Option<u64> {
    let mut info = unsafe { std::mem::zeroed::<libc::rusage_info_v2>() };
    let rc = unsafe {
        libc::proc_pid_rusage(
            pid,
            libc::RUSAGE_INFO_V2,
            &mut info as *mut libc::rusage_info_v2 as *mut libc::rusage_info_t,
        )
    };
    if rc == 0 {
        Some(info.ri_resident_size.max(info.ri_phys_footprint))
    } else {
        None
    }
}

#[cfg(not(target_os = "macos"))]
fn read_process_rss_bytes(_pid: libc::pid_t) -> Option<u64> {
    None
}

fn exceeds_dir_quota(root: &Path) -> bool {
    let mut count = 0usize;
    let mut total_bytes = 0u64;
    let mut stack = vec![root.to_path_buf()];
    while let Some(dir) = stack.pop() {
        let Ok(entries) = fs::read_dir(&dir) else {
            continue;
        };
        for entry in entries.flatten() {
            let Ok(meta) = fs::symlink_metadata(entry.path()) else {
                continue;
            };
            count += 1;
            total_bytes += meta.len();
            if count > MAX_SANDBOX_FILES || total_bytes > MAX_SANDBOX_BYTES {
                return true;
            }
            if meta.is_dir() {
                stack.push(entry.path());
            }
        }
    }
    false
}

fn set_limit(resource: libc::c_int, soft: u64, hard: u64) -> io::Result<()> {
    let rlim = libc::rlimit {
        rlim_cur: soft as libc::rlim_t,
        rlim_max: hard as libc::rlim_t,
    };
    if unsafe { libc::setrlimit(resource, &rlim) } != 0 {
        return Err(io::Error::last_os_error());
    }
    Ok(())
}
