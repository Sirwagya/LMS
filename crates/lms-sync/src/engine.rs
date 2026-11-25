use chrono::Utc;
use std::{sync::Arc, time::Duration};
use tokio::sync::{Mutex, Notify, watch};

use crate::projection::{account_snapshot, invalid, refreshes_workspace};
use crate::{
    LocalStorePort, RemoteSyncPort, SyncError, SyncState, SyncStatusSnapshot, compute_backoff_ms,
};

const PUSH_BATCH: usize = 50;
const PULL_BATCH: usize = 200;
const MAX_RETRY_MS: u64 = 300_000;

/// Serializes sync passes for one account and exposes status changes to its host.
pub struct SyncEngine<S, R> {
    store: Arc<S>,
    remote: R,
    validated_token: Mutex<Option<String>>,
    status: watch::Sender<SyncStatusSnapshot>,
}

impl<S: LocalStorePort, R: RemoteSyncPort> SyncEngine<S, R> {
    pub fn new(store: Arc<S>, remote: R) -> Result<Self, SyncError> {
        let (status, _) = watch::channel(store.status_snapshot()?);
        Ok(Self {
            store,
            remote,
            validated_token: Mutex::new(None),
            status,
        })
    }

    pub fn subscribe(&self) -> watch::Receiver<SyncStatusSnapshot> {
        self.status.subscribe()
    }

    /// Run a push/pull pass. A caller may trigger this after reconnect, an
    /// offline edit, an SSE event or a refresh gesture.
    pub async fn synchronize(&self, bearer_token: &str) -> Result<SyncStatusSnapshot, SyncError> {
        let mut validated = self.validated_token.lock().await;
        self.publish(SyncState::Syncing)?;
        let result = self.pass(bearer_token, &mut validated).await;
        let state = match &result {
            Ok(()) => SyncState::Synced,
            Err(err) if err.is_transient() => SyncState::Offline,
            Err(_) => SyncState::Error,
        };
        let published = self.publish(state);
        result?;
        published
    }

    /// Run continuously with a periodic pull as a fallback for missed events.
    /// Notify after an edit, reconnect or sync.available SSE event. The host
    /// can cancel this future when its session ends.
    pub async fn run(
        &self,
        bearer_token: &str,
        wake: &Notify,
        poll_interval: Duration,
    ) -> Result<(), SyncError> {
        let poll_interval = poll_interval.max(Duration::from_secs(1));
        let mut retry = 0_u32;
        loop {
            let delay = match self.synchronize(bearer_token).await {
                Ok(_) => {
                    retry = 0;
                    let next = self.store.next_retry_at()?.map(|time| {
                        Duration::from_millis(time.saturating_sub(now_ms()).max(0) as u64)
                    });
                    next.map_or(poll_interval, |delay| delay.min(poll_interval))
                        .max(Duration::from_millis(20))
                }
                Err(err) if err.is_transient() => {
                    let delay = retry_delay(&err, retry);
                    retry = retry.saturating_add(1);
                    Duration::from_millis(delay)
                }
                Err(err) => return Err(err),
            };
            tokio::select! {
                _ = wake.notified() => {}
                _ = tokio::time::sleep(delay) => {}
            }
        }
    }

    fn publish(&self, state: SyncState) -> Result<SyncStatusSnapshot, SyncError> {
        let mut snapshot = self.store.status_snapshot()?;
        snapshot.state = if state == SyncState::Synced && snapshot.pending_count > 0 {
            SyncState::Syncing
        } else {
            state
        };
        self.status.send_replace(snapshot.clone());
        Ok(snapshot)
    }

    async fn pass(&self, token: &str, validated: &mut Option<String>) -> Result<(), SyncError> {
        let has_bootstrap = self.store.has_bootstrap()?;
        if !has_bootstrap || validated.as_deref() != Some(token) {
            let snapshot = self.remote.fetch_bootstrap(token).await?;
            account_snapshot(&snapshot, self.store.account_id())?;
            if !has_bootstrap {
                self.store.save_bootstrap_json(&snapshot)?;
            }
            *validated = Some(token.to_owned());
        }

        loop {
            let batch = self.store.pending_mutations(PUSH_BATCH, now_ms())?;
            if batch.is_empty() {
                break;
            }
            match self.remote.push_mutations(token, &batch).await {
                Ok(outcomes) => self.store.apply_outcomes(&batch, &outcomes)?,
                Err(err) => {
                    if err.is_transient() {
                        let attempt = batch.iter().map(|edit| edit.retry_count).max().unwrap_or(0);
                        let next = now_ms().saturating_add(retry_delay(&err, attempt) as i64);
                        self.store.schedule_retry(&batch, next, &err.to_string())?;
                    }
                    return Err(err);
                }
            }
        }

        loop {
            let cursor = self.store.status_snapshot()?.cursor;
            let page = self.remote.pull_delta(token, cursor, PULL_BATCH).await?;
            if page.cursor < cursor || (page.has_more && page.cursor == cursor) {
                return Err(invalid("pull cursor failed to advance"));
            }
            let refresh = !page.has_more
                && (self.store.needs_bootstrap_refresh()?
                    || page.entries.iter().any(|entry| {
                        refreshes_workspace(&entry.entity_type)
                            || entry.op == "delete" && entry.entity_type == "profile"
                    }));
            let snapshot = if refresh {
                Some(self.remote.fetch_bootstrap(token).await?)
            } else {
                None
            };
            self.store
                .apply_changelog(cursor, &page, snapshot.as_ref())?;
            if !page.has_more {
                break;
            }
        }
        self.store.record_sync_success(now_ms())
    }
}

fn now_ms() -> i64 {
    Utc::now().timestamp_millis()
}

fn retry_delay(err: &SyncError, attempt: u32) -> u64 {
    let backoff = compute_backoff_ms(attempt, rand::random());
    if let SyncError::Http {
        retry_after_ms: Some(server),
        ..
    } = err
    {
        return backoff.max((*server).min(MAX_RETRY_MS));
    }
    backoff
}
