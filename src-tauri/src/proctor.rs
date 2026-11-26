use serde::{Deserialize, Serialize};

#[cfg(target_os = "macos")]
use std::{
    path::PathBuf,
    sync::atomic::{AtomicU64, Ordering},
    time::{Duration, SystemTime, UNIX_EPOCH},
};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct RunningApp {
    pub name: String,
    pub bundle_id: String,
    pub pid: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct QuitAppsResult {
    pub requested: Vec<String>,
    pub remaining: Vec<RunningApp>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct ScreenCaptureResult {
    pub content_type: String,
    pub data_base64: String,
    pub byte_size: usize,
}

const BASE64_TABLE: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

/// Encode raw bytes into standard RFC 4648 base64 without external crate dependencies.
pub fn encode_base64(input: &[u8]) -> String {
    if input.is_empty() {
        return String::new();
    }
    let mut out = String::with_capacity(input.len().div_ceil(3) * 4);
    for chunk in input.chunks(3) {
        let b0 = chunk[0] as u32;
        let b1 = if chunk.len() > 1 { chunk[1] as u32 } else { 0 };
        let b2 = if chunk.len() > 2 { chunk[2] as u32 } else { 0 };
        let triple = (b0 << 16) | (b1 << 8) | b2;

        out.push(BASE64_TABLE[((triple >> 18) & 0x3f) as usize] as char);
        out.push(BASE64_TABLE[((triple >> 12) & 0x3f) as usize] as char);
        if chunk.len() > 1 {
            out.push(BASE64_TABLE[((triple >> 6) & 0x3f) as usize] as char);
        } else {
            out.push('=');
        }
        if chunk.len() > 2 {
            out.push(BASE64_TABLE[(triple & 0x3f) as usize] as char);
        } else {
            out.push('=');
        }
    }
    out
}

const ALLOWED_SYSTEM_APP_NAMES: &[&str] = &[
    "vedam lms",
    "vedam-lms-desktop",
    "finder",
    "dock",
    "systemuiserver",
    "windowmanager",
    "controlcenter",
    "notificationcenter",
    "spotlight",
    "loginwindow",
    "coreservicesuiagent",
    "usernotificationcenter",
    "securityagent",
    "osduihelper",
    "textinputmenuagent",
    "universalaccessauthwarn",
    "talagent",
    "airplayuiagent",
    "screencaptureui",
];

const ALLOWED_SYSTEM_BUNDLE_IDS: &[&str] = &[
    "com.vedam.lms",
    "com.apple.finder",
    "com.apple.dock",
    "com.apple.systemuiserver",
    "com.apple.windowmanager",
    "com.apple.controlcenter",
    "com.apple.notificationcenterui",
    "com.apple.spotlight",
    "com.apple.loginwindow",
    "com.apple.coreservicesuiagent",
    "com.apple.usernotificationcenter",
    "com.apple.securityagent",
];

fn matches_bundle_prefix(bundle_lower: &str, prefix: &str) -> bool {
    bundle_lower == prefix
        || bundle_lower
            .strip_prefix(prefix)
            .is_some_and(|rest| rest.starts_with('.'))
}

fn is_allowed_process(app: &RunningApp, self_pid: u32) -> bool {
    if app.pid == self_pid {
        return true;
    }
    let name_lower = app.name.trim().to_ascii_lowercase();
    if ALLOWED_SYSTEM_APP_NAMES.contains(&name_lower.as_str()) {
        return true;
    }
    let bundle_lower = app.bundle_id.trim().to_ascii_lowercase();
    if bundle_lower.is_empty() {
        return false;
    }
    ALLOWED_SYSTEM_BUNDLE_IDS
        .iter()
        .any(|prefix| matches_bundle_prefix(&bundle_lower, prefix))
}

/// Filter out Vedam LMS itself and essential macOS system/shell UI processes.
pub fn filter_disallowed_apps(apps: Vec<RunningApp>, self_pid: u32) -> Vec<RunningApp> {
    apps.into_iter()
        .filter(|app| !is_allowed_process(app, self_pid))
        .collect()
}

/// Parse tab-delimited `name\tbundle_id\tpid` lines emitted by System Events AppleScript.
pub fn parse_process_lines(raw: &str) -> Vec<RunningApp> {
    raw.lines()
        .filter_map(|line| {
            let trimmed = line.trim_end_matches('\r');
            if trimmed.trim().is_empty() {
                return None;
            }
            let mut parts = trimmed.split('\t');
            let name = parts.next()?.trim();
            let bundle_id = parts.next().unwrap_or("").trim();
            let pid_str = parts.next()?.trim();
            if name.is_empty() {
                return None;
            }
            let pid = pid_str.parse::<u32>().ok().filter(|&p| p > 0)?;
            Some(RunningApp {
                name: name.to_string(),
                bundle_id: bundle_id.to_string(),
                pid,
            })
        })
        .collect()
}

#[cfg(any(target_os = "macos", test))]
fn escape_applescript_string(value: &str) -> String {
    let mut escaped = String::with_capacity(value.len());
    for ch in value.chars() {
        match ch {
            '\\' => escaped.push_str("\\\\"),
            '"' => escaped.push_str("\\\""),
            '\r' | '\n' => escaped.push(' '),
            _ => escaped.push(ch),
        }
    }
    escaped
}

#[cfg(target_os = "macos")]
async fn list_apps_macos() -> Result<Vec<RunningApp>, String> {
    let script = r#"tell application "System Events"
    set output to ""
    repeat with proc in (every application process whose background only is false)
        set procName to ""
        try
            set procName to (name of proc as text)
        end try
        set procBid to ""
        try
            set rawBid to bundle identifier of proc
            if rawBid is not missing value then
                set procBid to (rawBid as text)
            end if
        end try
        set procPid to "0"
        try
            set procPid to ((unix id of proc) as text)
        end try
        if procName is not "" then
            set output to output & procName & tab & procBid & tab & procPid & linefeed
        end if
    end repeat
    return output
end tell"#;

    let mut cmd = tokio::process::Command::new("osascript");
    cmd.arg("-e").arg(script).kill_on_drop(true);

    let output = tokio::time::timeout(Duration::from_secs(4), cmd.output())
        .await
        .map_err(|_| "Timed out while inspecting running macOS applications.".to_string())?
        .map_err(|e| format!("Failed to inspect running applications: {e}"))?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
        if stderr.is_empty() {
            return Err("Failed to inspect running applications.".to_string());
        }
        return Err(format!("Failed to inspect running applications: {stderr}"));
    }

    let stdout = String::from_utf8_lossy(&output.stdout);
    let parsed = parse_process_lines(&stdout);
    Ok(filter_disallowed_apps(parsed, std::process::id()))
}

#[cfg(target_os = "macos")]
async fn quit_app_macos(app: &RunningApp) {
    let script = if !app.bundle_id.trim().is_empty() {
        format!(
            "tell application id \"{}\" to quit",
            escape_applescript_string(app.bundle_id.trim())
        )
    } else {
        format!(
            "tell application \"{}\" to quit",
            escape_applescript_string(app.name.trim())
        )
    };

    let mut cmd = tokio::process::Command::new("osascript");
    cmd.arg("-e").arg(script).kill_on_drop(true);
    let _ = tokio::time::timeout(Duration::from_secs(2), cmd.output()).await;
}

#[cfg(target_os = "macos")]
struct TempCaptureFile(PathBuf);

#[cfg(target_os = "macos")]
impl TempCaptureFile {
    fn new_jpg() -> PathBuf {
        static COUNTER: AtomicU64 = AtomicU64::new(1);
        let seq = COUNTER.fetch_add(1, Ordering::Relaxed);
        let ts = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|d| d.as_nanos())
            .unwrap_or(0);
        std::env::temp_dir().join(format!(
            "vedam-proctor-{}-{}-{}.jpg",
            std::process::id(),
            ts,
            seq
        ))
    }
}

#[cfg(target_os = "macos")]
impl Drop for TempCaptureFile {
    fn drop(&mut self) {
        let _ = std::fs::remove_file(&self.0);
    }
}

#[tauri::command]
pub async fn lms_proctor_list_apps() -> Result<Vec<RunningApp>, String> {
    #[cfg(target_os = "macos")]
    {
        list_apps_macos().await
    }
    #[cfg(not(target_os = "macos"))]
    {
        Ok(Vec::new())
    }
}

#[tauri::command]
pub async fn lms_proctor_quit_other_apps() -> Result<QuitAppsResult, String> {
    #[cfg(target_os = "macos")]
    {
        let apps = list_apps_macos().await?;
        let requested: Vec<String> = apps.iter().map(|app| app.name.clone()).collect();
        for app in &apps {
            quit_app_macos(app).await;
        }
        if !apps.is_empty() {
            tokio::time::sleep(Duration::from_millis(650)).await;
        }
        let remaining = list_apps_macos().await?;
        Ok(QuitAppsResult {
            requested,
            remaining,
        })
    }
    #[cfg(not(target_os = "macos"))]
    {
        Ok(QuitAppsResult {
            requested: Vec::new(),
            remaining: Vec::new(),
        })
    }
}

#[tauri::command]
pub async fn lms_proctor_set_fullscreen(
    window: tauri::WebviewWindow,
    enabled: bool,
) -> Result<bool, String> {
    window
        .set_fullscreen(enabled)
        .map_err(|e| format!("Failed to update fullscreen state: {e}"))?;
    window.is_fullscreen().map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn lms_proctor_get_fullscreen(window: tauri::WebviewWindow) -> Result<bool, String> {
    window.is_fullscreen().map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn lms_proctor_capture_screen() -> Result<ScreenCaptureResult, String> {
    #[cfg(target_os = "macos")]
    {
        const CAPTURE_ERR: &str = "Screen capture failed. Allow Screen Recording for Vedam LMS in macOS System Settings -> Privacy & Security -> Screen Recording.";

        let temp_file = TempCaptureFile(TempCaptureFile::new_jpg());
        let mut cmd = tokio::process::Command::new("/usr/sbin/screencapture");
        cmd.arg("-x")
            .arg("-t")
            .arg("jpg")
            .arg(&temp_file.0)
            .kill_on_drop(true);

        let output = match tokio::time::timeout(Duration::from_secs(5), cmd.output()).await {
            Ok(Ok(output)) => output,
            _ => {
                let _ = tokio::fs::remove_file(&temp_file.0).await;
                return Err(CAPTURE_ERR.to_string());
            }
        };

        if !output.status.success() {
            let _ = tokio::fs::remove_file(&temp_file.0).await;
            return Err(CAPTURE_ERR.to_string());
        }

        let bytes = tokio::fs::read(&temp_file.0).await.unwrap_or_default();
        let _ = tokio::fs::remove_file(&temp_file.0).await;

        if bytes.is_empty() {
            return Err(CAPTURE_ERR.to_string());
        }

        Ok(ScreenCaptureResult {
            content_type: "image/jpeg".to_string(),
            data_base64: encode_base64(&bytes),
            byte_size: bytes.len(),
        })
    }
    #[cfg(not(target_os = "macos"))]
    {
        Err("Native screen capture is supported only on macOS.".to_string())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn encode_base64_matches_rfc4648_vectors() {
        assert_eq!(encode_base64(b""), "");
        assert_eq!(encode_base64(b"f"), "Zg==");
        assert_eq!(encode_base64(b"fo"), "Zm8=");
        assert_eq!(encode_base64(b"foo"), "Zm9v");
        assert_eq!(encode_base64(b"foob"), "Zm9vYg==");
        assert_eq!(encode_base64(b"fooba"), "Zm9vYmE=");
        assert_eq!(encode_base64(b"foobar"), "Zm9vYmFy");
        assert_eq!(encode_base64(&[0xff, 0x00, 0xfe]), "/wD+");
    }

    #[test]
    fn filter_disallowed_apps_keeps_only_disallowed_user_apps() {
        let apps = vec![
            RunningApp {
                name: "SelfApp".into(),
                bundle_id: "com.example.self".into(),
                pid: 4242,
            },
            RunningApp {
                name: "Vedam LMS".into(),
                bundle_id: "com.vedam.lms".into(),
                pid: 100,
            },
            RunningApp {
                name: "vedam-lms-desktop".into(),
                bundle_id: "".into(),
                pid: 101,
            },
            RunningApp {
                name: "Custom Helper".into(),
                bundle_id: "COM.VEDAM.LMS.helper".into(),
                pid: 102,
            },
            RunningApp {
                name: "Finder".into(),
                bundle_id: "com.apple.finder".into(),
                pid: 103,
            },
            RunningApp {
                name: "Dock".into(),
                bundle_id: "com.apple.dock".into(),
                pid: 104,
            },
            RunningApp {
                name: "SystemUIServer".into(),
                bundle_id: "com.apple.systemuiserver".into(),
                pid: 105,
            },
            RunningApp {
                name: "WindowManager".into(),
                bundle_id: "com.apple.WindowManager".into(),
                pid: 106,
            },
            RunningApp {
                name: "ControlCenter".into(),
                bundle_id: "com.apple.controlcenter".into(),
                pid: 107,
            },
            RunningApp {
                name: "NotificationCenter".into(),
                bundle_id: "com.apple.notificationcenterui".into(),
                pid: 108,
            },
            RunningApp {
                name: "Spotlight".into(),
                bundle_id: "com.apple.Spotlight".into(),
                pid: 109,
            },
            RunningApp {
                name: "loginwindow".into(),
                bundle_id: "com.apple.loginwindow".into(),
                pid: 110,
            },
            RunningApp {
                name: "CoreServicesUIAgent".into(),
                bundle_id: "com.apple.CoreServicesUIAgent".into(),
                pid: 111,
            },
            RunningApp {
                name: "UserNotificationCenter".into(),
                bundle_id: "".into(),
                pid: 112,
            },
            RunningApp {
                name: "SecurityAgent".into(),
                bundle_id: "com.apple.SecurityAgent".into(),
                pid: 113,
            },
            RunningApp {
                name: "OSDUIHelper".into(),
                bundle_id: "".into(),
                pid: 114,
            },
            RunningApp {
                name: "TextInputMenuAgent".into(),
                bundle_id: "".into(),
                pid: 115,
            },
            RunningApp {
                name: "universalAccessAuthWarn".into(),
                bundle_id: "".into(),
                pid: 116,
            },
            RunningApp {
                name: "talagent".into(),
                bundle_id: "".into(),
                pid: 117,
            },
            RunningApp {
                name: "AirPlayUIAgent".into(),
                bundle_id: "".into(),
                pid: 118,
            },
            RunningApp {
                name: "screencaptureui".into(),
                bundle_id: "".into(),
                pid: 119,
            },
            RunningApp {
                name: "Google Chrome".into(),
                bundle_id: "com.google.Chrome".into(),
                pid: 200,
            },
            RunningApp {
                name: "Safari".into(),
                bundle_id: "com.apple.Safari".into(),
                pid: 201,
            },
            RunningApp {
                name: "Slack".into(),
                bundle_id: "com.tinyspeck.slackmacgap".into(),
                pid: 202,
            },
        ];

        let remaining = filter_disallowed_apps(apps, 4242);
        assert_eq!(
            remaining,
            vec![
                RunningApp {
                    name: "Google Chrome".into(),
                    bundle_id: "com.google.Chrome".into(),
                    pid: 200,
                },
                RunningApp {
                    name: "Safari".into(),
                    bundle_id: "com.apple.Safari".into(),
                    pid: 201,
                },
                RunningApp {
                    name: "Slack".into(),
                    bundle_id: "com.tinyspeck.slackmacgap".into(),
                    pid: 202,
                },
            ]
        );
    }

    #[test]
    fn parse_process_lines_handles_valid_and_malformed_rows() {
        let raw = concat!(
            "Google Chrome\tcom.google.Chrome\t512\n",
            "CLI Helper\t\t777\r\n",
            "\n",
            "   \n",
            "\tcom.missing.name\t123\n",
            "ZeroPid\tcom.zero.pid\t0\n",
            "BadPid\tcom.bad.pid\tabc\n",
            "MissingPid\tcom.no.pid\n",
            "Visual Studio Code\tcom.microsoft.VSCode\t9001\n",
        );

        let parsed = parse_process_lines(raw);
        assert_eq!(
            parsed,
            vec![
                RunningApp {
                    name: "Google Chrome".into(),
                    bundle_id: "com.google.Chrome".into(),
                    pid: 512,
                },
                RunningApp {
                    name: "CLI Helper".into(),
                    bundle_id: "".into(),
                    pid: 777,
                },
                RunningApp {
                    name: "Visual Studio Code".into(),
                    bundle_id: "com.microsoft.VSCode".into(),
                    pid: 9001,
                },
            ]
        );
    }

    #[test]
    fn escape_applescript_string_escapes_quotes_and_backslashes() {
        assert_eq!(
            escape_applescript_string("App \"One\"\\Two\nThree"),
            "App \\\"One\\\"\\\\Two Three"
        );
    }
}
