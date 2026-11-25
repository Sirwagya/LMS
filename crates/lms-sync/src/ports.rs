use async_trait::async_trait;
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OutboxMutation {
    pub id: String,
    pub idempotency_key: String,
    pub entity_type: String,
    pub entity_id: String,
    pub endpoint_method: String,
    pub endpoint_path: String,
    pub base_version: i64,
    pub payload_json: Value,
    pub retry_count: u32,
}

#[derive(Clone, Copy, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum PushStatus {
    Applied,
    Conflict,
    Rejected,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PushItemOutcome {
    pub idempotency_key: String,
    pub status: PushStatus,
    pub server_version: Option<i64>,
    pub server_payload: Option<Value>,
    pub reason: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ChangelogEntry {
    pub seq: i64,
    pub scope: String,
    pub entity_type: String,
    pub entity_id: String,
    pub op: String,
    pub payload: Value,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PullResponse {
    pub cursor: i64,
    pub has_more: bool,
    pub entries: Vec<ChangelogEntry>,
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum SyncState {
    #[default]
    Offline,
    Synced,
    Syncing,
    Error,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncStatusSnapshot {
    pub state: SyncState,
    pub pending_count: usize,
    pub conflict_count: usize,
    pub last_synced_at: Option<DateTime<Utc>>,
    pub cursor: i64,
}

#[derive(Debug, thiserror::Error)]
pub enum SyncError {
    #[error("SQLite storage error: {0}")]
    Storage(#[from] rusqlite::Error),
    #[error("SQLite writer lock was poisoned")]
    StorageLock,
    #[error("network unavailable: {0}")]
    Network(String),
    #[error("unauthorized session")]
    Unauthorized,
    #[error("HTTP {status}")]
    Http {
        status: u16,
        retry_after_ms: Option<u64>,
    },
    #[error("invalid sync data: {0}")]
    Protocol(String),
    #[error("JSON error: {0}")]
    Json(#[from] serde_json::Error),
    #[error("account does not match the local database")]
    AccountMismatch,
}

impl SyncError {
    pub fn is_transient(&self) -> bool {
        matches!(
            self,
            Self::Network(_)
                | Self::Http {
                    status: 408 | 429 | 500..=599,
                    ..
                }
        )
    }
}

pub trait LocalStorePort: Send + Sync + 'static {
    fn account_id(&self) -> &str;
    fn has_bootstrap(&self) -> Result<bool, SyncError>;
    fn save_bootstrap_json(&self, snapshot: &Value) -> Result<(), SyncError>;
    fn pending_mutations(
        &self,
        limit: usize,
        now_ms: i64,
    ) -> Result<Vec<OutboxMutation>, SyncError>;
    fn apply_outcomes(
        &self,
        batch: &[OutboxMutation],
        outcomes: &[PushItemOutcome],
    ) -> Result<(), SyncError>;
    fn schedule_retry(
        &self,
        batch: &[OutboxMutation],
        next_ms: i64,
        error: &str,
    ) -> Result<(), SyncError>;
    fn needs_bootstrap_refresh(&self) -> Result<bool, SyncError>;
    fn apply_changelog(
        &self,
        expected_cursor: i64,
        page: &PullResponse,
        refreshed_snapshot: Option<&Value>,
    ) -> Result<(), SyncError>;
    fn status_snapshot(&self) -> Result<SyncStatusSnapshot, SyncError>;
    fn record_sync_success(&self, now_ms: i64) -> Result<(), SyncError>;
    fn next_retry_at(&self) -> Result<Option<i64>, SyncError>;
}

#[async_trait]
pub trait RemoteSyncPort: Send + Sync {
    async fn fetch_bootstrap(&self, bearer_token: &str) -> Result<Value, SyncError>;
    async fn push_mutations(
        &self,
        bearer_token: &str,
        batch: &[OutboxMutation],
    ) -> Result<Vec<PushItemOutcome>, SyncError>;
    async fn pull_delta(
        &self,
        bearer_token: &str,
        cursor: i64,
        limit: usize,
    ) -> Result<PullResponse, SyncError>;
}
