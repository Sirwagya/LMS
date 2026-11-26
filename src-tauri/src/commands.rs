use std::{
    collections::HashSet,
    path::{Path, PathBuf},
    sync::Arc,
    time::Duration,
};

use futures_util::StreamExt;
use lms_grader::{Entrypoint, GradeRequest, GradeResult, JudgePipeline, Language, TestCaseInput};
use lms_sync::{
    CachedAsset, HttpRemote, LocalStorePort, SqliteStore, SyncEngine, SyncError, SyncState,
    SyncStatusSnapshot,
};
use reqwest::{Client, Method, StatusCode, redirect::Policy};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use tauri::{AppHandle, Emitter, State};
use tauri_plugin_dialog::DialogExt;
use tauri_plugin_notification::NotificationExt;
use tokio::{
    sync::{Mutex, Notify},
    task::JoinHandle,
};

use crate::keychain::{SessionVault, StoredSession};

const DEFAULT_API_URL: &str = "http://127.0.0.1:3001";
const SYNC_POLL_INTERVAL: Duration = Duration::from_secs(30);

#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct SyncStatusDto {
    pub state: SyncState,
    pub pending_count: usize,
    pub conflict_count: usize,
    pub last_synced_at: Option<String>,
    pub cursor: i64,
}

impl From<SyncStatusSnapshot> for SyncStatusDto {
    fn from(snapshot: SyncStatusSnapshot) -> Self {
        Self {
            state: snapshot.state,
            pending_count: snapshot.pending_count,
            conflict_count: snapshot.conflict_count,
            last_synced_at: snapshot.last_synced_at.map(|ts| ts.to_rfc3339()),
            cursor: snapshot.cursor,
        }
    }
}

/// Observer sink decoupling DesktopCore sync/SSE notifications from Tauri's AppHandle.
pub trait EventSink: Send + Sync + 'static {
    fn emit_sync_updated(&self, status: &SyncStatusDto);
    fn emit_server_event(&self, name: &str, payload: &Value);
    fn emit_navigate(&self, hash: &str);
    fn notify(&self, title: &str, body: &str);
}

#[derive(Default)]
#[cfg_attr(not(test), allow(dead_code))]
pub struct NoopEventSink;

impl EventSink for NoopEventSink {
    fn emit_sync_updated(&self, _status: &SyncStatusDto) {}
    fn emit_server_event(&self, _name: &str, _payload: &Value) {}
    fn emit_navigate(&self, _hash: &str) {}
    fn notify(&self, _title: &str, _body: &str) {}
}

pub struct TauriEventSink {
    app: AppHandle,
}

impl TauriEventSink {
    pub fn new(app: AppHandle) -> Self {
        Self { app }
    }
}

impl EventSink for TauriEventSink {
    fn emit_sync_updated(&self, status: &SyncStatusDto) {
        let _ = self.app.emit("lms://sync-updated", status);
    }

    fn emit_server_event(&self, name: &str, payload: &Value) {
        let _ = self.app.emit(
            "lms://event",
            json!({
                "name": name,
                "payload": payload,
            }),
        );
    }

    fn emit_navigate(&self, hash: &str) {
        let _ = self.app.emit("lms://navigate", json!({ "hash": hash }));
    }

    fn notify(&self, title: &str, body: &str) {
        let _ = self
            .app
            .notification()
            .builder()
            .title(title)
            .body(body)
            .show();
    }
}

struct AccountSession {
    account_id: String,
    store: Arc<SqliteStore>,
    engine: Arc<SyncEngine<SqliteStore, HttpRemote>>,
    wake: Arc<Notify>,
    tasks: std::sync::Mutex<Vec<JoinHandle<()>>>,
}

impl AccountSession {
    fn stop(&self) {
        if let Ok(mut guard) = self.tasks.lock() {
            for handle in guard.drain(..) {
                handle.abort();
            }
        }
    }
}

impl Drop for AccountSession {
    fn drop(&mut self) {
        self.stop();
    }
}

#[derive(Debug)]
enum ProxyFailure {
    Connectivity(String),
    Unauthorized,
    ApiError(String),
}

impl From<ProxyFailure> for String {
    fn from(err: ProxyFailure) -> Self {
        match err {
            ProxyFailure::Connectivity(msg) | ProxyFailure::ApiError(msg) => msg,
            ProxyFailure::Unauthorized => "Please sign in.".to_string(),
        }
    }
}

/// Core desktop orchestrator bridging offline SQLite sync, macOS Keychain sessions,
/// sandboxed local code execution, and authenticated Go API calls.
pub struct DesktopCore {
    api_base_url: String,
    data_dir: PathBuf,
    http: Client,
    sse_http: Client,
    vault: Arc<dyn SessionVault>,
    grader: Arc<JudgePipeline>,
    events: Arc<dyn EventSink>,
    active: Mutex<Option<Arc<AccountSession>>>,
    session_change: Mutex<()>,
}

impl DesktopCore {
    pub fn new(
        data_dir: impl Into<PathBuf>,
        api_base_url: impl Into<String>,
        vault: Arc<dyn SessionVault>,
        events: Arc<dyn EventSink>,
    ) -> Result<Self, String> {
        let api_base_url = HttpRemote::validate_api_origin(&api_base_url.into())
            .map_err(|err| err.to_string())?
            .as_str()
            .trim_end_matches('/')
            .to_string();
        let data_dir = data_dir.into();
        std::fs::create_dir_all(&data_dir).map_err(|e| format!("create desktop data dir: {e}"))?;
        let http = Client::builder()
            .connect_timeout(Duration::from_secs(5))
            .timeout(Duration::from_secs(30))
            .redirect(Policy::none())
            .build()
            .map_err(|e| e.to_string())?;
        let sse_http = Client::builder()
            .connect_timeout(Duration::from_secs(5))
            .read_timeout(Duration::from_secs(60))
            .redirect(Policy::none())
            .build()
            .map_err(|e| e.to_string())?;
        Ok(Self {
            api_base_url,
            data_dir,
            http,
            sse_http,
            vault,
            grader: Arc::new(JudgePipeline::default()),
            events,
            active: Mutex::new(None),
            session_change: Mutex::new(()),
        })
    }

    pub fn resolved_api_url() -> String {
        std::env::var("LMS_API_URL")
            .ok()
            .map(|s| s.trim().to_string())
            .filter(|s| !s.is_empty())
            .unwrap_or_else(|| DEFAULT_API_URL.to_string())
    }

    pub fn events(&self) -> &Arc<dyn EventSink> {
        &self.events
    }

    /// Initialize any persisted Keychain student session at desktop startup.
    pub async fn resume_persisted_session(&self) {
        if let Some(session) = self.vault.load()
            && session.role == "student"
            && let Ok(account) = self.ensure_student_session(&session).await
            && let Ok(snapshot) = account.store.status_snapshot()
        {
            self.events.emit_sync_updated(&snapshot.into());
        }
    }

    pub async fn request(
        &self,
        path: &str,
        method: Option<&str>,
        body: Option<Value>,
    ) -> Result<Value, String> {
        let method = method.unwrap_or("GET").to_ascii_uppercase();
        let (route, _) = split_path_query(path);
        match (method.as_str(), route) {
            ("POST", "/login") | ("POST", "/register") => self.handle_auth(route, body).await,
            ("POST", "/logout") => self.handle_logout().await,
            ("GET", "/bootstrap") => self.handle_bootstrap().await,
            ("POST", "/progress") | ("PUT", "/profile") | ("POST", "/discussions") => {
                self.handle_offline_or_proxy_mutation(&method, path, route, body)
                    .await
            }
            ("GET", "/discussions") => self.handle_discussions_get(path).await,
            ("POST", "/coding/evaluate") => self.handle_coding_evaluate(path, body).await,
            ("GET", "/coding/history") => self.handle_coding_history(path).await,
            ("POST", "/password") => self.handle_password_change(path, body).await,
            _ => self.handle_authenticated_proxy(&method, path, body).await,
        }
    }

    pub async fn sync_now(&self) -> Result<SyncStatusDto, String> {
        let Some(session) = self.vault.load() else {
            return Ok(SyncStatusDto {
                state: SyncState::Offline,
                ..SyncStatusDto::default()
            });
        };
        if session.role != "student" {
            return Ok(SyncStatusDto {
                state: SyncState::Synced,
                ..SyncStatusDto::default()
            });
        }
        let account = self.ensure_student_session(&session).await?;
        let _ = account.store.clear_retry_schedule();
        let dto = match account.engine.synchronize(&session.token).await {
            Ok(snapshot) => SyncStatusDto::from(snapshot),
            Err(SyncError::Unauthorized) => {
                let _ = self.invalidate_session(&session).await;
                return Err("Please sign in.".to_string());
            }
            Err(err) => {
                let mut dto = account
                    .store
                    .status_snapshot()
                    .map(SyncStatusDto::from)
                    .unwrap_or_default();
                dto.state = if err.is_transient() {
                    SyncState::Offline
                } else {
                    SyncState::Error
                };
                dto
            }
        };
        self.events.emit_sync_updated(&dto);
        Ok(dto)
    }

    pub async fn sync_status(&self) -> Result<SyncStatusDto, String> {
        let Some(session) = self.vault.load() else {
            return Ok(SyncStatusDto {
                state: SyncState::Offline,
                ..SyncStatusDto::default()
            });
        };
        if session.role != "student" {
            return Ok(SyncStatusDto {
                state: SyncState::Synced,
                ..SyncStatusDto::default()
            });
        }
        let account = self.ensure_student_session(&session).await?;
        let mut watch_state = account.engine.subscribe().borrow().clone();
        if let Ok(latest) = account.store.status_snapshot() {
            watch_state.pending_count = latest.pending_count;
            watch_state.conflict_count = latest.conflict_count;
            watch_state.cursor = latest.cursor;
            if latest.last_synced_at.is_some() {
                watch_state.last_synced_at = latest.last_synced_at;
            }
        }
        Ok(SyncStatusDto::from(watch_state))
    }

    async fn run_code_local(&self, request: GradeRequest) -> Result<GradeResult, String> {
        if self.require_session()?.role != "student" {
            return Err("Student access required.".into());
        }
        self.grader
            .evaluate(request)
            .await
            .map_err(|e| e.to_string())
    }

    pub async fn save_code_draft(
        &self,
        assignment_id: &str,
        language: &str,
        source: &str,
    ) -> Result<(), String> {
        let Some(session) = self.vault.load() else {
            return Ok(());
        };
        if session.role != "student" {
            return Ok(());
        }
        let account = self.ensure_student_session(&session).await?;
        account
            .store
            .save_code_draft(assignment_id, language, source)
            .map_err(|e| e.to_string())
    }

    pub async fn load_code_draft(
        &self,
        assignment_id: &str,
        language: &str,
    ) -> Result<Option<String>, String> {
        let Some(session) = self.vault.load() else {
            return Ok(None);
        };
        if session.role != "student" {
            return Ok(None);
        }
        let account = self.ensure_student_session(&session).await?;
        account
            .store
            .code_draft(assignment_id, language)
            .map_err(|e| e.to_string())
    }

    /// Downloads an `/api/*` receipt/note/submission attachment (using cached bytes if available offline)
    /// and returns `(suggested_filename, bytes)` for saving via NSSavePanel.
    pub async fn fetch_asset_bytes(
        &self,
        url: &str,
        filename: Option<&str>,
    ) -> Result<(String, Vec<u8>), String> {
        if !url.starts_with("/api/") {
            return Err("Unsupported asset URL.".to_string());
        }
        let session = self.require_session()?;
        let cache_key = sanitize_filename(url.trim_start_matches('/'));
        let default_name = filename
            .filter(|s| !s.trim().is_empty())
            .map(|s| s.to_string())
            .unwrap_or_else(|| infer_asset_filename(url));

        let account = if session.role == "student" {
            Some(self.ensure_student_session(&session).await?)
        } else {
            None
        };

        if let Some(acc) = &account
            && let Ok(Some(cached)) = acc.store.asset(&cache_key)
            && let Ok(bytes) = tokio::fs::read(&cached.local_path).await
        {
            return Ok((cached.filename, bytes));
        }

        let target = format!("{}{}", self.api_base_url, url);
        let resp = self
            .http
            .get(&target)
            .bearer_auth(&session.token)
            .send()
            .await
            .map_err(|e| format!("Download failed: {e}"))?;

        if resp.status() == StatusCode::UNAUTHORIZED {
            let _ = self.invalidate_session(&session).await;
            return Err("Please sign in.".to_string());
        }
        if !resp.status().is_success() {
            return Err(format!("Asset request failed (HTTP {}).", resp.status()));
        }

        let mime_type = resp
            .headers()
            .get(reqwest::header::CONTENT_TYPE)
            .and_then(|v| v.to_str().ok())
            .unwrap_or("application/octet-stream")
            .to_string();
        let resolved_name = resp
            .headers()
            .get(reqwest::header::CONTENT_DISPOSITION)
            .and_then(|v| v.to_str().ok())
            .and_then(parse_content_disposition_filename)
            .unwrap_or_else(|| sanitize_filename(&default_name));

        let bytes = resp
            .bytes()
            .await
            .map_err(|e| format!("Read asset body: {e}"))?
            .to_vec();

        if let Some(acc) = &account {
            let assets_dir = self
                .data_dir
                .join("assets")
                .join(sanitize_filename(&session.user_id));
            tokio::fs::create_dir_all(&assets_dir)
                .await
                .map_err(|e| e.to_string())?;
            let cached_path =
                assets_dir.join(format!("{cache_key}_{}", sanitize_filename(&resolved_name)));
            tokio::fs::write(&cached_path, &bytes)
                .await
                .map_err(|e| e.to_string())?;
            let _ = acc.store.save_asset(&CachedAsset {
                id: cache_key,
                filename: resolved_name.clone(),
                mime_type,
                local_path: cached_path.to_string_lossy().to_string(),
                byte_size: bytes.len() as i64,
            });
        }

        Ok((resolved_name, bytes))
    }

    async fn handle_auth(&self, route: &str, body: Option<Value>) -> Result<Value, String> {
        let _change = self.session_change.lock().await;
        let payload = body.ok_or_else(|| "Credentials required.".to_string())?;
        let previous = self.vault.load();
        let mut response = self
            .proxy_raw("POST", route, None, Some(payload))
            .await
            .map_err(String::from)?;

        let token = response
            .get("token")
            .and_then(Value::as_str)
            .map(str::to_string)
            .ok_or_else(|| "Server response did not include a session token.".to_string())?;
        let user_id = response
            .pointer("/user/id")
            .and_then(Value::as_str)
            .ok_or_else(|| "Server response did not include a user id.".to_string())?
            .to_string();
        let role = response
            .pointer("/user/role")
            .and_then(Value::as_str)
            .unwrap_or("student")
            .to_string();

        let session = StoredSession {
            user_id,
            role: role.clone(),
            token: token.clone(),
        };
        if let Some(previous) = previous.as_ref()
            && previous.token != session.token
            && let Err(err) = self.revoke_bearer(&previous.token).await
        {
            let _ = self.revoke_bearer(&session.token).await;
            return Err(format!("Could not end the previous session. {err}"));
        }
        if let Err(err) = self.replace_session(&session).await {
            let _ = self.revoke_bearer(&session.token).await;
            if let Some(previous) = previous.as_ref() {
                let _ = self.invalidate_session(previous).await;
            }
            return Err(err);
        }

        if role == "student" {
            let account = self.ensure_student_session(&session).await?;
            if let Ok(snapshot) = account.engine.synchronize(&token).await {
                self.events.emit_sync_updated(&snapshot.into());
            }
        }

        if let Some(obj) = response.as_object_mut() {
            obj.remove("token");
        }
        Ok(response)
    }

    async fn revoke_bearer(&self, token: &str) -> Result<(), String> {
        let response = self
            .http
            .post(format!("{}/api/v1/session/revoke", self.api_base_url))
            .bearer_auth(token)
            .timeout(Duration::from_secs(5))
            .json(&json!({}))
            .send()
            .await
            .map_err(|_| "Session revocation could not reach the server.".to_string())?;
        if response.status().is_success() || response.status() == StatusCode::UNAUTHORIZED {
            Ok(())
        } else {
            Err(format!(
                "Session revocation failed (HTTP {}).",
                response.status()
            ))
        }
    }

    async fn handle_logout(&self) -> Result<Value, String> {
        let _change = self.session_change.lock().await;
        if let Some(session) = self.vault.load() {
            let _ = self
                .proxy_raw("POST", "/logout", Some(&session.token), Some(json!({})))
                .await;
            self.invalidate_session(&session).await?;
        }
        Ok(json!({ "ok": true }))
    }

    async fn handle_bootstrap(&self) -> Result<Value, String> {
        let session = self.require_session()?;
        if session.role != "student" {
            return self
                .proxy_raw("GET", "/bootstrap", Some(&session.token), None)
                .await
                .map_err(String::from);
        }
        let account = self.ensure_student_session(&session).await?;
        if let Some(snapshot) = account
            .store
            .get_bootstrap_json()
            .map_err(|e| e.to_string())?
        {
            account.wake.notify_one();
            return Ok(snapshot);
        }

        match account.engine.synchronize(&session.token).await {
            Ok(status) => {
                self.events.emit_sync_updated(&status.into());
            }
            Err(SyncError::Unauthorized) => {
                let _ = self.invalidate_session(&session).await;
                return Err("Please sign in.".to_string());
            }
            Err(err) => return Err(err.to_string()),
        }

        account
            .store
            .get_bootstrap_json()
            .map_err(|e| e.to_string())?
            .ok_or_else(|| "Workspace could not be loaded.".to_string())
    }

    async fn handle_offline_or_proxy_mutation(
        &self,
        method: &str,
        path: &str,
        route: &str,
        body: Option<Value>,
    ) -> Result<Value, String> {
        let session = self.require_session()?;
        if session.role != "student" {
            return self
                .proxy_raw(method, path, Some(&session.token), body)
                .await
                .map_err(String::from);
        }
        let account = self.ensure_student_session(&session).await?;
        let payload = body.unwrap_or_else(|| json!({}));

        match route {
            "/progress" => {
                let lecture_id = parse_i64_field(&payload, "lectureId")
                    .ok_or_else(|| "Invalid lecture id.".to_string())?;
                let complete = payload
                    .get("complete")
                    .and_then(Value::as_bool)
                    .ok_or_else(|| "Missing completion state.".to_string())?;
                account
                    .store
                    .set_progress(lecture_id, complete)
                    .map_err(|e| e.to_string())?;
            }
            "/profile" => {
                let profile = payload
                    .get("profile")
                    .cloned()
                    .ok_or_else(|| "Profile required.".to_string())?;
                account
                    .store
                    .edit_profile(profile)
                    .map_err(|e| e.to_string())?;
            }
            "/discussions" => {
                let lecture_id = parse_i64_field(&payload, "lectureId")
                    .ok_or_else(|| "Invalid lecture id.".to_string())?;
                let text = payload
                    .get("body")
                    .and_then(Value::as_str)
                    .ok_or_else(|| "Message body required.".to_string())?;
                account
                    .store
                    .post_discussion(lecture_id, text)
                    .map_err(|e| e.to_string())?;
            }
            _ => unreachable!("unexpected mutation route"),
        }

        account.wake.notify_one();
        if let Ok(status) = account.store.status_snapshot() {
            self.events.emit_sync_updated(&status.into());
        }
        Ok(json!({ "ok": true }))
    }

    async fn handle_discussions_get(&self, path: &str) -> Result<Value, String> {
        let session = self.require_session()?;
        if session.role != "student" {
            return self
                .proxy_raw("GET", path, Some(&session.token), None)
                .await
                .map_err(String::from);
        }
        let account = self.ensure_student_session(&session).await?;
        let lecture_id = parse_query_i64(path, "lecture")
            .ok_or_else(|| "Missing lecture query parameter.".to_string())?;

        match self
            .proxy_raw("GET", path, Some(&session.token), None)
            .await
        {
            Ok(Value::Array(mut remote_rows)) => {
                let existing_ids: HashSet<String> = remote_rows
                    .iter()
                    .filter_map(|row| row.get("id").and_then(Value::as_str).map(str::to_string))
                    .collect();
                if let Ok(cached) = account.store.discussions(lecture_id) {
                    for row in cached {
                        let is_pending =
                            row.get("pending").and_then(Value::as_bool).unwrap_or(false);
                        let row_id = row.get("id").and_then(Value::as_str).unwrap_or("");
                        if is_pending && !existing_ids.contains(row_id) {
                            remote_rows.push(row);
                        }
                    }
                }
                Ok(Value::Array(remote_rows))
            }
            Ok(other) => Ok(other),
            Err(ProxyFailure::Unauthorized) => {
                let _ = self.invalidate_session(&session).await;
                Err("Please sign in.".to_string())
            }
            Err(ProxyFailure::Connectivity(_)) => {
                let rows = account
                    .store
                    .discussions(lecture_id)
                    .map_err(|e| e.to_string())?;
                Ok(Value::Array(rows))
            }
            Err(ProxyFailure::ApiError(msg)) => Err(msg),
        }
    }

    async fn handle_coding_evaluate(
        &self,
        path: &str,
        body: Option<Value>,
    ) -> Result<Value, String> {
        let session = self.require_session()?;
        let payload = body.ok_or_else(|| "Evaluation payload required.".to_string())?;
        let action = payload.get("action").and_then(Value::as_str).unwrap_or("");

        match self
            .proxy_raw("POST", path, Some(&session.token), Some(payload.clone()))
            .await
        {
            Ok(result) => {
                if action == "submit" && session.role == "student"
                    && let Ok(account) = self.ensure_student_session(&session).await
                    && let Ok(status) = account.engine.synchronize(&session.token).await
                {
                    self.events.emit_sync_updated(&status.into());
                }
                Ok(result)
            }
            Err(ProxyFailure::Unauthorized) => {
                let _=self.invalidate_session(&session).await;
                Err("Please sign in.".to_string())
            }
            Err(ProxyFailure::Connectivity(_)) if action == "run" && session.role == "student" => {
                let account = self.ensure_student_session(&session).await?;
                let snapshot = account
                    .store
                    .get_bootstrap_json()
                    .map_err(|e| e.to_string())?
                    .ok_or_else(|| "Offline workspace has not been synced yet.".to_string())?;
                let request = build_offline_grade_request(&snapshot, &payload)?;
                let result = self.run_code_local(request).await?;
                serde_json::to_value(result).map_err(|e| e.to_string())
            }
            Err(ProxyFailure::Connectivity(_)) if action == "submit" => Err(
                "You are offline. Official submissions require a connection; use Run to test locally."
                    .to_string(),
            ),
            Err(err) => Err(err.into()),
        }
    }

    async fn handle_coding_history(&self, path: &str) -> Result<Value, String> {
        let session = self.require_session()?;
        match self
            .proxy_raw("GET", path, Some(&session.token), None)
            .await
        {
            Ok(val) => Ok(val),
            Err(ProxyFailure::Unauthorized) => {
                let _ = self.invalidate_session(&session).await;
                Err("Please sign in.".to_string())
            }
            Err(ProxyFailure::Connectivity(_)) => Ok(json!([])),
            Err(ProxyFailure::ApiError(msg)) => Err(msg),
        }
    }

    async fn handle_password_change(
        &self,
        path: &str,
        body: Option<Value>,
    ) -> Result<Value, String> {
        let _change = self.session_change.lock().await;
        let session = self.require_session()?;
        let mut response = self
            .proxy_raw("POST", path, Some(&session.token), body)
            .await
            .map_err(String::from)?;
        let Some(new_token) = response.get("token").and_then(Value::as_str) else {
            let _ = self.invalidate_session(&session).await;
            return Err("The server did not return the rotated session. Please sign in.".into());
        };
        let mut next = session.clone();
        next.token = new_token.to_string();
        if let Err(err) = self.replace_session(&next).await {
            let _ = self.invalidate_session(&session).await;
            return Err(err);
        }
        if next.role == "student" {
            self.ensure_student_session(&next).await?;
        }
        if let Some(obj) = response.as_object_mut() {
            obj.remove("token");
        }
        Ok(response)
    }

    async fn handle_authenticated_proxy(
        &self,
        method: &str,
        path: &str,
        body: Option<Value>,
    ) -> Result<Value, String> {
        let session = self.require_session()?;
        match self
            .proxy_raw(method, path, Some(&session.token), body)
            .await
        {
            Ok(val) => {
                if method != "GET"
                    && !is_contest_attempt_path(path)
                    && session.role == "student"
                    && let Ok(account) = self.ensure_student_session(&session).await
                    && let Ok(status) = account.engine.synchronize(&session.token).await
                {
                    self.events.emit_sync_updated(&status.into());
                }
                Ok(val)
            }
            Err(ProxyFailure::Unauthorized) => {
                let _ = self.invalidate_session(&session).await;
                Err("Please sign in.".to_string())
            }
            Err(err) => Err(err.into()),
        }
    }

    async fn proxy_raw(
        &self,
        method: &str,
        path: &str,
        bearer_token: Option<&str>,
        body: Option<Value>,
    ) -> Result<Value, ProxyFailure> {
        let parsed_method = Method::from_bytes(method.as_bytes())
            .map_err(|_| ProxyFailure::ApiError("Unsupported HTTP method.".into()))?;
        let url = format!("{}/api/v1{}", self.api_base_url, path);
        let mut req = self.http.request(parsed_method, url);
        if cfg!(target_os = "macos") && is_contest_attempt_path(path) {
            req = req
                .header("X-LMS-Contest-Client", "macos")
                .timeout(Duration::from_secs(8));
        }
        if method == "POST" && matches!(path, "/login" | "/register" | "/password") {
            req = req.header("X-LMS-Auth", "bearer");
        }
        if let Some(token) = bearer_token {
            req = req.bearer_auth(token);
        }
        if let Some(payload) = body {
            req = req.json(&payload);
        }
        let resp = req
            .send()
            .await
            .map_err(|e| ProxyFailure::Connectivity(format!("Server unavailable: {e}")))?;
        let status = resp.status();
        if status == StatusCode::UNAUTHORIZED {
            return Err(ProxyFailure::Unauthorized);
        }
        if matches!(
            status,
            StatusCode::BAD_GATEWAY | StatusCode::SERVICE_UNAVAILABLE | StatusCode::GATEWAY_TIMEOUT
        ) {
            let msg = extract_api_error(resp).await;
            return Err(ProxyFailure::Connectivity(msg));
        }
        if !status.is_success() {
            return Err(ProxyFailure::ApiError(extract_api_error(resp).await));
        }
        resp.json::<Value>()
            .await
            .map_err(|e| ProxyFailure::ApiError(format!("Invalid server response: {e}")))
    }

    fn require_session(&self) -> Result<StoredSession, String> {
        self.vault
            .load()
            .ok_or_else(|| "Please sign in.".to_string())
    }

    async fn replace_session(&self, session: &StoredSession) -> Result<(), String> {
        let mut guard = self.active.lock().await;
        self.vault.save(session)?;
        if let Some(previous) = guard.take() {
            previous.stop();
        }
        Ok(())
    }

    async fn invalidate_session(&self, expected: &StoredSession) -> Result<(), String> {
        let mut guard = self.active.lock().await;
        if self
            .vault
            .load()
            .is_none_or(|current| current.token != expected.token)
        {
            return Ok(());
        }
        let clearance = self.vault.clear();
        if let Some(previous) = guard.take() {
            previous.stop();
        }
        self.events.emit_sync_updated(&SyncStatusDto {
            state: SyncState::Offline,
            ..SyncStatusDto::default()
        });
        clearance
    }

    async fn ensure_student_session(
        &self,
        session: &StoredSession,
    ) -> Result<Arc<AccountSession>, String> {
        let mut guard = self.active.lock().await;
        let current = self.require_session()?;
        if current.user_id != session.user_id
            || current.token != session.token
            || current.role != "student"
        {
            return Err("The active student session changed.".into());
        }
        if let Some(existing) = guard.as_ref()
            && existing.account_id == session.user_id
        {
            return Ok(Arc::clone(existing));
        }
        // Drop previous account session first so its background tasks abort cleanly.
        if let Some(previous) = guard.take() {
            previous.stop();
        }

        let safe_id = sanitize_filename(&session.user_id);
        let db_dir = self.data_dir.join("accounts").join(&safe_id);
        std::fs::create_dir_all(&db_dir).map_err(|e| format!("create account db dir: {e}"))?;
        let db_path = db_dir.join("workspace.db");

        let store =
            Arc::new(SqliteStore::open(db_path, &session.user_id).map_err(|e| e.to_string())?);
        let remote = HttpRemote::new(&self.api_base_url).map_err(|e| e.to_string())?;
        let engine =
            Arc::new(SyncEngine::new(Arc::clone(&store), remote).map_err(|e| e.to_string())?);
        let wake = Arc::new(Notify::new());

        let mut tasks = Vec::new();
        {
            let mut rx = engine.subscribe();
            let events = Arc::clone(&self.events);
            tasks.push(tokio::spawn(async move {
                let mut first = true;
                loop {
                    if !first && rx.changed().await.is_err() {
                        break;
                    }
                    first = false;
                    let snapshot = rx.borrow_and_update().clone();
                    events.emit_sync_updated(&snapshot.into());
                }
            }));
        }
        {
            let engine_run = Arc::clone(&engine);
            let wake_run = Arc::clone(&wake);
            let token_run = session.token.clone();
            tasks.push(tokio::spawn(async move {
                let _ = engine_run
                    .run(&token_run, &wake_run, SYNC_POLL_INTERVAL)
                    .await;
            }));
        }
        {
            let sse_client = self.sse_http.clone();
            let base = self.api_base_url.clone();
            let token_sse = session.token.clone();
            let wake_sse = Arc::clone(&wake);
            let events = Arc::clone(&self.events);
            tasks.push(tokio::spawn(async move {
                run_sse_loop(sse_client, base, token_sse, wake_sse, events).await;
            }));
        }

        let created = Arc::new(AccountSession {
            account_id: session.user_id.clone(),
            store,
            engine,
            wake,
            tasks: std::sync::Mutex::new(tasks),
        });
        *guard = Some(Arc::clone(&created));
        Ok(created)
    }
}

async fn run_sse_loop(
    client: Client,
    api_base_url: String,
    token: String,
    wake: Arc<Notify>,
    events: Arc<dyn EventSink>,
) {
    let url = format!("{api_base_url}/api/v1/events");
    let mut backoff = Duration::from_secs(1);
    loop {
        match client.get(&url).bearer_auth(&token).send().await {
            Ok(resp) if resp.status() == StatusCode::UNAUTHORIZED => break,
            Ok(resp) if resp.status().is_success() => {
                backoff = Duration::from_secs(1);
                let mut stream = resp.bytes_stream();
                let mut buf = String::new();
                while let Some(chunk_res) = stream.next().await {
                    let Ok(chunk) = chunk_res else {
                        break;
                    };
                    buf.push_str(&String::from_utf8_lossy(&chunk));
                    while let Some(pos) = buf.find("\n\n") {
                        let frame = buf[..pos].to_string();
                        buf.drain(..pos + 2);
                        handle_sse_frame(&frame, &wake, &events);
                    }
                }
            }
            _ => {}
        }
        tokio::time::sleep(backoff).await;
        backoff = (backoff * 2).min(Duration::from_secs(15));
    }
}

fn handle_sse_frame(frame: &str, wake: &Notify, events: &Arc<dyn EventSink>) {
    let mut event_name = String::new();
    let mut data_lines = Vec::new();
    for raw_line in frame.lines() {
        let line = raw_line.trim_end_matches('\r');
        if line.is_empty() || line.starts_with(':') {
            continue;
        }
        if let Some(rest) = line.strip_prefix("event:") {
            event_name = rest.trim().to_string();
        } else if let Some(rest) = line.strip_prefix("data:") {
            data_lines.push(rest.trim_start());
        }
    }
    if event_name.is_empty() {
        return;
    }
    let data_raw = data_lines.join("\n");
    let payload = serde_json::from_str::<Value>(&data_raw).unwrap_or_else(|_| json!(data_raw));
    wake.notify_one();
    events.emit_server_event(&event_name, &payload);

    match event_name.as_str() {
        "grade.updated" => {
            events.notify(
                "Assignment graded",
                "Your submission has been graded in Vedam LMS.",
            );
        }
        "notice.published" => {
            let tag = payload.get("tag").and_then(Value::as_str).unwrap_or("");
            let title = payload
                .get("title")
                .and_then(Value::as_str)
                .unwrap_or("A new announcement was published.");
            if tag.eq_ignore_ascii_case("URGENT") {
                events.notify("Urgent LMS Notice", title);
            } else {
                events.notify("New LMS Notice", title);
            }
        }
        _ => {}
    }
}

async fn extract_api_error(resp: reqwest::Response) -> String {
    let status = resp.status();
    if let Ok(val) = resp.json::<Value>().await
        && let Some(msg) = val
            .get("error")
            .or_else(|| val.get("message"))
            .and_then(Value::as_str)
    {
        return msg.to_string();
    }
    format!("Request failed (HTTP {status}).")
}

fn split_path_query(path: &str) -> (&str, Option<&str>) {
    match path.split_once('?') {
        Some((route, query)) => (route, Some(query)),
        None => (path, None),
    }
}

fn parse_query_i64(path: &str, key: &str) -> Option<i64> {
    let (_, query) = split_path_query(path);
    query?
        .split('&')
        .filter_map(|pair| pair.split_once('='))
        .find_map(|(k, v)| (k == key).then(|| v.parse::<i64>().ok()).flatten())
}

fn parse_i64_field(value: &Value, key: &str) -> Option<i64> {
    value.get(key).and_then(|v| {
        v.as_i64()
            .or_else(|| v.as_str().and_then(|s| s.parse::<i64>().ok()))
    })
}

fn build_offline_grade_request(snapshot: &Value, payload: &Value) -> Result<GradeRequest, String> {
    let assignment_id = payload
        .get("assignmentId")
        .and_then(|v| {
            v.as_str()
                .map(str::to_string)
                .or_else(|| v.as_i64().map(|n| n.to_string()))
        })
        .ok_or_else(|| "Coding assignment not found.".to_string())?;

    let source = payload
        .get("source")
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_string();

    let language = match payload.get("language").and_then(Value::as_str) {
        Some("python") => Language::Python,
        Some("java") => Language::Java,
        _ => return Err("Choose Java or Python.".to_string()),
    };

    let assignment = find_coding_assignment(snapshot, &assignment_id)
        .ok_or_else(|| "Coding assignment not available offline.".to_string())?;
    let problem = assignment
        .get("problem")
        .ok_or_else(|| "Coding problem definition missing.".to_string())?;

    let entrypoint = match problem.get("entrypoint").and_then(Value::as_str) {
        Some("twoSum") => Entrypoint::TwoSum,
        Some("isValid") => Entrypoint::IsValid,
        Some("solve") => Entrypoint::Solve,
        _ => Entrypoint::Stdin,
    };

    let test_cases = if let Some(custom) = payload.get("customInput").and_then(Value::as_str) {
        vec![TestCaseInput {
            input: custom.to_string(),
            expected: None,
            hidden: false,
        }]
    } else {
        problem
            .get("examples")
            .and_then(Value::as_array)
            .map(|items| {
                items
                    .iter()
                    .filter_map(|case| {
                        let input = case.get("input").and_then(Value::as_str)?.to_string();
                        let expected = case
                            .get("expected")
                            .and_then(Value::as_str)
                            .map(str::to_string);
                        Some(TestCaseInput {
                            input,
                            expected,
                            hidden: false,
                        })
                    })
                    .collect()
            })
            .unwrap_or_default()
    };

    if test_cases.is_empty() {
        return Err(
            "No offline sample cases found. Sync once online or provide custom input.".to_string(),
        );
    }

    Ok(GradeRequest {
        language,
        source,
        entrypoint,
        test_cases,
    })
}

fn find_coding_assignment<'a>(snapshot: &'a Value, target_id: &str) -> Option<&'a Value> {
    let subjects = snapshot.get("lecturesBySubject")?.as_object()?;
    for lectures in subjects.values() {
        let Some(lecture_list) = lectures.as_array() else {
            continue;
        };
        for lecture in lecture_list {
            let Some(assignments) = lecture.get("assignments").and_then(Value::as_array) else {
                continue;
            };
            for assignment in assignments {
                let id_matches = assignment
                    .get("id")
                    .and_then(|v| {
                        v.as_str()
                            .map(|s| s == target_id)
                            .or_else(|| v.as_i64().map(|n| n.to_string() == target_id))
                    })
                    .unwrap_or(false);
                if id_matches && assignment.get("kind").and_then(Value::as_str) == Some("coding") {
                    return Some(assignment);
                }
            }
        }
    }
    None
}

fn sanitize_filename(raw: &str) -> String {
    let cleaned: String = raw
        .chars()
        .map(|c| {
            if c.is_ascii_alphanumeric() || matches!(c, '-' | '_' | '.') {
                c
            } else {
                '_'
            }
        })
        .collect();
    if cleaned.is_empty() {
        "item".to_string()
    } else {
        cleaned
    }
}

fn infer_asset_filename(url: &str) -> String {
    let base = url.split('?').next().unwrap_or(url);
    let tail = base.rsplit('/').next().unwrap_or("download");
    if url.starts_with("/api/receipts/") {
        format!("receipt-{}.html", sanitize_filename(tail))
    } else if url.starts_with("/api/notes/") {
        format!("lecture-{}-notes.md", sanitize_filename(tail))
    } else {
        sanitize_filename(tail)
    }
}

fn parse_content_disposition_filename(header: &str) -> Option<String> {
    for part in header.split(';') {
        let trimmed = part.trim();
        if let Some(value) = trimmed.strip_prefix("filename=") {
            let unquoted = value.trim_matches('"').trim();
            if !unquoted.is_empty() {
                return Path::new(unquoted)
                    .file_name()
                    .and_then(|s| s.to_str())
                    .map(sanitize_filename);
            }
        }
    }
    None
}

fn is_contest_attempt_path(path: &str) -> bool {
    path.strip_prefix("/contests/")
        .and_then(|suffix| suffix.strip_suffix("/attempt"))
        .is_some_and(|id| !id.is_empty() && !id.contains('/'))
}

#[tauri::command]
pub async fn lms_request(
    core: State<'_, Arc<DesktopCore>>,
    path: String,
    method: Option<String>,
    body: Option<Value>,
) -> Result<Value, String> {
    core.request(&path, method.as_deref(), body).await
}

#[tauri::command]
pub async fn lms_sync_now(core: State<'_, Arc<DesktopCore>>) -> Result<SyncStatusDto, String> {
    core.sync_now().await
}

#[tauri::command]
pub async fn lms_sync_status(core: State<'_, Arc<DesktopCore>>) -> Result<SyncStatusDto, String> {
    core.sync_status().await
}

#[tauri::command]
pub async fn lms_save_code_draft(
    core: State<'_, Arc<DesktopCore>>,
    user_id: String,
    assignment_id: String,
    language: String,
    source: String,
) -> Result<(), String> {
    let _change = core.session_change.lock().await;
    if core.require_session()?.user_id != user_id {
        return Err("The active account changed.".into());
    }
    core.save_code_draft(&assignment_id, &language, &source)
        .await
}

#[tauri::command]
pub async fn lms_load_code_draft(
    core: State<'_, Arc<DesktopCore>>,
    user_id: String,
    assignment_id: String,
    language: String,
) -> Result<Option<String>, String> {
    let _change = core.session_change.lock().await;
    if core.require_session()?.user_id != user_id {
        return Err("The active account changed.".into());
    }
    core.load_code_draft(&assignment_id, &language).await
}

#[tauri::command]
pub async fn lms_export_asset(
    app: AppHandle,
    core: State<'_, Arc<DesktopCore>>,
    url: String,
    filename: Option<String>,
) -> Result<bool, String> {
    let (default_name, bytes) = core.fetch_asset_bytes(&url, filename.as_deref()).await?;
    let selected = tokio::task::spawn_blocking(move || {
        app.dialog()
            .file()
            .set_file_name(&default_name)
            .blocking_save_file()
    })
    .await
    .map_err(|e| format!("Save sheet failed: {e}"))?;

    let Some(file_path) = selected else {
        return Ok(false);
    };
    let dest = file_path
        .into_path()
        .map_err(|e| format!("Invalid save location: {e}"))?;
    tokio::fs::write(&dest, bytes)
        .await
        .map_err(|e| format!("Could not write file: {e}"))?;
    Ok(true)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::keychain::MemorySessionVault;
    use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
    use tokio::{
        io::{AsyncReadExt, AsyncWriteExt},
        net::{TcpListener, TcpStream},
    };

    struct MockServer {
        addr: String,
        online: Arc<AtomicBool>,
        push_calls: Arc<AtomicUsize>,
        handle: JoinHandle<()>,
    }

    impl Drop for MockServer {
        fn drop(&mut self) {
            self.handle.abort();
        }
    }

    async fn read_http_request(stream: &mut TcpStream) -> Option<(String, String, String, String)> {
        let mut raw = Vec::new();
        let mut buf = [0u8; 4096];
        let header_end;
        loop {
            let n = stream.read(&mut buf).await.ok()?;
            if n == 0 {
                return None;
            }
            raw.extend_from_slice(&buf[..n]);
            if let Some(pos) = raw.windows(4).position(|w| w == b"\r\n\r\n") {
                header_end = pos + 4;
                break;
            }
        }
        let head = String::from_utf8_lossy(&raw[..header_end]).to_string();
        let mut lines = head.lines();
        let req_line = lines.next()?.split_whitespace().collect::<Vec<_>>();
        if req_line.len() < 2 {
            return None;
        }
        let method = req_line[0].to_string();
        let path = req_line[1].to_string();
        let content_len = lines
            .find_map(|line| {
                let (k, v) = line.split_once(':')?;
                k.eq_ignore_ascii_case("content-length")
                    .then(|| v.trim().parse::<usize>().ok())
                    .flatten()
            })
            .unwrap_or(0);
        while raw.len() < header_end + content_len {
            let n = stream.read(&mut buf).await.ok()?;
            if n == 0 {
                break;
            }
            raw.extend_from_slice(&buf[..n]);
        }
        let body =
            String::from_utf8_lossy(&raw[header_end..raw.len().min(header_end + content_len)])
                .to_string();
        Some((method, path, body, head))
    }

    async fn write_json_response(stream: &mut TcpStream, status: &str, body: &Value) {
        let json = serde_json::to_string(body).unwrap();
        let header = format!(
            "HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
            json.len()
        );
        let _ = stream.write_all(header.as_bytes()).await;
        let _ = stream.write_all(json.as_bytes()).await;
    }

    async fn start_mock_server() -> MockServer {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = format!("http://{}", listener.local_addr().unwrap());
        let online = Arc::new(AtomicBool::new(true));
        let push_calls = Arc::new(AtomicUsize::new(0));
        let online_flag = Arc::clone(&online);
        let push_counter = Arc::clone(&push_calls);
        let cursor = Arc::new(AtomicUsize::new(0));
        let changelog = Arc::new(std::sync::Mutex::new(Vec::<Value>::new()));
        let profile_name = Arc::new(std::sync::Mutex::new("Student Demo".to_string()));
        let progress_list = Arc::new(std::sync::Mutex::new(Vec::<i64>::new()));

        let handle = tokio::spawn(async move {
            loop {
                let Ok((mut stream, _)) = listener.accept().await else {
                    break;
                };
                let online_flag = Arc::clone(&online_flag);
                let push_counter = Arc::clone(&push_counter);
                let cursor = Arc::clone(&cursor);
                let changelog = Arc::clone(&changelog);
                let profile_name = Arc::clone(&profile_name);
                let progress_list = Arc::clone(&progress_list);
                tokio::spawn(async move {
                    let Some((method, path, body, _)) = read_http_request(&mut stream).await else {
                        return;
                    };
                    if !online_flag.load(Ordering::SeqCst) {
                        write_json_response(
                            &mut stream,
                            "503 Service Unavailable",
                            &json!({ "error": "Offline simulation" }),
                        )
                        .await;
                        return;
                    }
                    let route = path.split('?').next().unwrap_or(&path);
                    match (method.as_str(), route) {
                        ("POST", "/api/v1/login") => {
                            write_json_response(
                                &mut stream,
                                "200 OK",
                                &json!({
                                    "token": "tok-live-session",
                                    "user": { "id": "stu-1", "name": "Student Demo", "email": "student@vedam.local", "role": "student" }
                                }),
                            )
                            .await;
                        }
                        ("GET", "/api/v1/bootstrap") => {
                            let name = profile_name.lock().unwrap().clone();
                            let prog = progress_list.lock().unwrap().clone();
                            write_json_response(
                                &mut stream,
                                "200 OK",
                                &json!({
                                    "user": { "id": "stu-1", "name": name, "role": "student", "version": 1, "profile": { "name": name } },
                                    "progress": prog,
                                    "submissions": [],
                                    "semesterSubjects": {},
                                    "lecturesBySubject": {
                                        "CS201": [{
                                            "id": 101,
                                            "assignments": [{
                                                "id": "two-sum",
                                                "kind": "coding",
                                                "title": "Two Sum",
                                                "problem": {
                                                    "entrypoint": "stdin",
                                                    "examples": [{ "input": "2 7\n", "expected": "9" }]
                                                }
                                            }]
                                        }]
                                    },
                                    "contestSummary": {},
                                    "recentContests": [],
                                    "demoWeek": { "start": "2026-10-19", "published": true, "days": {} },
                                    "drawerNotices": [],
                                    "fees": []
                                }),
                            )
                            .await;
                        }
                        ("POST", "/api/v1/sync/push") => {
                            push_counter.fetch_add(1, Ordering::SeqCst);
                            let req: Value = serde_json::from_str(&body).unwrap_or(json!({}));
                            let mut results = Vec::new();
                            for item in req["mutations"].as_array().into_iter().flatten() {
                                let key = item["idempotencyKey"].as_str().unwrap_or("");
                                let kind = item["entityType"].as_str().unwrap_or("");
                                let seq = cursor.fetch_add(1, Ordering::SeqCst) as i64 + 1;
                                if kind == "lecture_progress"
                                    && let Some(id) = item["payload"]["lectureId"].as_i64()
                                {
                                    let complete =
                                        item["payload"]["complete"].as_bool().unwrap_or(false);
                                    let mut guard = progress_list.lock().unwrap();
                                    guard.retain(|&x| x != id);
                                    if complete {
                                        guard.push(id);
                                    }
                                    results.push(json!({
                                        "idempotencyKey": key,
                                        "status": "applied",
                                        "serverVersion": seq,
                                        "serverPayload": item["payload"]
                                    }));
                                    changelog.lock().unwrap().push(json!({
                                        "seq": seq,
                                        "scope": "user:stu-1",
                                        "entityType": "lecture_progress",
                                        "entityId": id.to_string(),
                                        "op": "upsert",
                                        "payload": item["payload"]
                                    }));
                                } else if kind == "profile" {
                                    let next_name = item["payload"]["name"]
                                        .as_str()
                                        .unwrap_or("Student")
                                        .to_string();
                                    *profile_name.lock().unwrap() = next_name;
                                    let profile_payload = json!({ "id": "stu-1", "profile": item["payload"], "version": 2 });
                                    results.push(json!({
                                        "idempotencyKey": key,
                                        "status": "applied",
                                        "serverVersion": 2,
                                        "serverPayload": profile_payload
                                    }));
                                    changelog.lock().unwrap().push(json!({
                                        "seq": seq,
                                        "scope": "user:stu-1",
                                        "entityType": "profile",
                                        "entityId": "stu-1",
                                        "op": "upsert",
                                        "payload": profile_payload
                                    }));
                                } else if kind == "discussion" {
                                    let discussion_payload = json!({
                                        "id": format!("srv-{seq}"),
                                        "lecture_id": item["payload"]["lectureId"],
                                        "user_id": "stu-1",
                                        "name": "Student Demo",
                                        "body": item["payload"]["body"],
                                        "created": "2026-11-25T13:30:00Z"
                                    });
                                    results.push(json!({
                                        "idempotencyKey": key,
                                        "status": "applied",
                                        "serverVersion": seq,
                                        "serverPayload": discussion_payload
                                    }));
                                    changelog.lock().unwrap().push(json!({
                                        "seq": seq,
                                        "scope": "user:stu-1",
                                        "entityType": "discussion",
                                        "entityId": format!("srv-{seq}"),
                                        "op": "upsert",
                                        "payload": discussion_payload
                                    }));
                                }
                            }
                            write_json_response(
                                &mut stream,
                                "200 OK",
                                &json!({ "results": results }),
                            )
                            .await;
                        }
                        ("GET", "/api/v1/sync/pull") => {
                            let since = parse_query_i64(&path, "cursor").unwrap_or(0);
                            let entries: Vec<Value> = changelog
                                .lock()
                                .unwrap()
                                .iter()
                                .filter(|entry| entry["seq"].as_i64().unwrap_or(0) > since)
                                .cloned()
                                .collect();
                            let cur = entries
                                .last()
                                .and_then(|e| e["seq"].as_i64())
                                .unwrap_or(since);
                            write_json_response(
                                &mut stream,
                                "200 OK",
                                &json!({ "cursor": cur, "hasMore": false, "entries": entries }),
                            )
                            .await;
                        }
                        ("GET", "/api/v1/discussions") => {
                            write_json_response(&mut stream, "200 OK", &json!([])).await;
                        }
                        ("POST", "/api/v1/logout") => {
                            write_json_response(&mut stream, "200 OK", &json!({ "ok": true }))
                                .await;
                        }
                        _ => {
                            write_json_response(
                                &mut stream,
                                "404 Not Found",
                                &json!({ "error": "Not found" }),
                            )
                            .await;
                        }
                    }
                });
            }
        });

        MockServer {
            addr,
            online,
            push_calls,
            handle,
        }
    }

    #[tokio::test]
    async fn contest_attempt_uses_authenticated_native_transport() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let url = format!("http://{}", listener.local_addr().unwrap());
        let server = tokio::spawn(async move {
            let (mut stream, _) = listener.accept().await.unwrap();
            let (method, path, body, headers) = read_http_request(&mut stream).await.unwrap();
            assert_eq!(method, "POST");
            assert_eq!(path, "/api/v1/contests/sample/attempt");
            let headers = headers.to_ascii_lowercase();
            assert!(headers.contains("\r\nx-lms-contest-client: macos\r\n"));
            assert!(headers.contains("\r\nauthorization: bearer native-test-token\r\n"));
            assert!(!headers.contains("\r\norigin:"));
            assert_eq!(
                serde_json::from_str::<Value>(&body).unwrap()["action"],
                "state"
            );
            write_json_response(&mut stream, "200 OK", &json!({ "state": "ready" })).await;
        });
        let dir = tempfile::tempdir().unwrap();
        let vault = Arc::new(MemorySessionVault::default());
        vault
            .save(&StoredSession {
                user_id: "student".into(),
                role: "student".into(),
                token: "native-test-token".into(),
            })
            .unwrap();
        let core = DesktopCore::new(dir.path(), &url, vault, Arc::new(NoopEventSink)).unwrap();
        let attempt = core
            .request(
                "/contests/sample/attempt",
                Some("POST"),
                Some(json!({ "action": "state" })),
            )
            .await;
        server.await.unwrap();
        assert_eq!(attempt.unwrap()["state"], "ready");
    }

    #[tokio::test]
    async fn offline_lifecycle_drafts_local_run_and_reconnect_sync() {
        let server = start_mock_server().await;
        let dir = tempfile::tempdir().unwrap();
        let vault: Arc<dyn SessionVault> = Arc::new(MemorySessionVault::default());
        let core = DesktopCore::new(
            dir.path(),
            &server.addr,
            Arc::clone(&vault),
            Arc::new(NoopEventSink),
        )
        .unwrap();

        // 1. Online login stores token only in vault and strips it from JS payload.
        let login_res = core
            .request(
                "/login",
                Some("POST"),
                Some(json!({ "email": "student@vedam.local", "password": "VedamLocal123!" })),
            )
            .await
            .unwrap();
        assert!(login_res.get("token").is_none());
        assert_eq!(login_res["user"]["id"], "stu-1");
        assert_eq!(vault.load().unwrap().token, "tok-live-session");

        // 2. Simulate full offline outage; workspace remains readable from SQLite WAL.
        server.online.store(false, Ordering::SeqCst);
        let bootstrap = core.request("/bootstrap", Some("GET"), None).await.unwrap();
        assert_eq!(bootstrap["user"]["name"], "Student Demo");

        // 3. Offline mutations queue cleanly in sync_outbox and update local projection.
        core.request(
            "/progress",
            Some("POST"),
            Some(json!({ "lectureId": 101, "complete": true })),
        )
        .await
        .unwrap();
        core.request(
            "/profile",
            Some("PUT"),
            Some(json!({ "profile": { "name": "Sirwagya Offline" } })),
        )
        .await
        .unwrap();
        core.request(
            "/discussions",
            Some("POST"),
            Some(json!({ "lectureId": 101, "body": "Offline test note" })),
        )
        .await
        .unwrap();

        let status = core.sync_status().await.unwrap();
        assert_eq!(status.pending_count, 3);

        let offline_bootstrap = core.request("/bootstrap", Some("GET"), None).await.unwrap();
        assert_eq!(offline_bootstrap["progress"], json!([101]));
        assert_eq!(offline_bootstrap["user"]["name"], "Sirwagya Offline");

        let discussions = core
            .request("/discussions?lecture=101", Some("GET"), None)
            .await
            .unwrap();
        assert_eq!(discussions.as_array().unwrap().len(), 1);
        assert_eq!(discussions[0]["body"], "Offline test note");

        // 4. Local code drafts & offline Python sandbox evaluation work cleanly without server.
        core.save_code_draft(
            "two-sum",
            "python",
            "a, b = map(int, input().split())\nprint(a + b)",
        )
        .await
        .unwrap();
        let eval_res = core
            .request(
                "/coding/evaluate",
                Some("POST"),
                Some(json!({
                    "assignmentId": "two-sum",
                    "language": "python",
                    "action": "run",
                    "source": "a, b = map(int, input().split())\nprint(a + b)"
                })),
            )
            .await
            .unwrap();
        assert_eq!(eval_res["verdict"], "Accepted");
        assert_eq!(eval_res["passed"], 1);

        // 5. Simulate desktop restart against same SQLite data directory & vault.
        drop(core);
        let restarted = DesktopCore::new(
            dir.path(),
            &server.addr,
            Arc::clone(&vault),
            Arc::new(NoopEventSink),
        )
        .unwrap();
        let draft = restarted
            .load_code_draft("two-sum", "python")
            .await
            .unwrap()
            .unwrap();
        assert!(draft.contains("print(a + b)"));

        // 6. Reconnect backend and drain outbox cleanly.
        server.online.store(true, Ordering::SeqCst);
        let synced = restarted.sync_now().await.unwrap();
        assert_eq!(synced.state, SyncState::Synced);
        assert_eq!(synced.pending_count, 0);
        assert!(server.push_calls.load(Ordering::SeqCst) >= 1);

        // 7. Logout clears vault and session.
        restarted
            .request("/logout", Some("POST"), None)
            .await
            .unwrap();
        assert_eq!(vault.load(), None);
        let unauth = restarted.request("/bootstrap", Some("GET"), None).await;
        assert_eq!(unauth.unwrap_err(), "Please sign in.");
    }
}
