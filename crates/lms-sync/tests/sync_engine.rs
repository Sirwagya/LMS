use std::sync::{
    Arc,
    atomic::{AtomicUsize, Ordering},
};

use async_trait::async_trait;
use chrono::Utc;
use lms_sync::{
    ChangelogEntry, LocalStorePort, OutboxMutation, PullResponse, PushItemOutcome, PushStatus,
    RemoteSyncPort, SqliteStore, SyncEngine, SyncError, SyncState, compute_backoff_ms,
};
use serde_json::{Value, json};

fn snapshot() -> Value {
    json!({"user":{"id":"student","name":"Before","profile":{"name":"Before"},"version":1},"progress":[],"fees":[]})
}

struct Fixture {
    dir: tempfile::TempDir,
    store: Arc<SqliteStore>,
}

fn fixture() -> Fixture {
    let dir = tempfile::tempdir().unwrap();
    let store = Arc::new(SqliteStore::open(dir.path().join("store.sqlite"), "student").unwrap());
    Fixture { dir, store }
}

fn ready(store: &SqliteStore) {
    store.save_bootstrap_json(&snapshot()).unwrap();
    store
        .apply_changelog(0, &page(0, vec![]), Some(&snapshot()))
        .unwrap();
}

fn page(cursor: i64, entries: Vec<ChangelogEntry>) -> PullResponse {
    PullResponse {
        cursor,
        has_more: false,
        entries,
    }
}

fn change(seq: i64, kind: &str, payload: Value) -> ChangelogEntry {
    ChangelogEntry {
        seq,
        scope: "user:student".into(),
        entity_type: kind.into(),
        entity_id: "42".into(),
        op: "upsert".into(),
        payload,
    }
}

fn progress_outcome(mutation: &OutboxMutation) -> PushItemOutcome {
    PushItemOutcome {
        idempotency_key: mutation.idempotency_key.clone(),
        status: PushStatus::Applied,
        server_version: Some(1),
        server_payload: Some(json!({"lectureId":42,"complete":true,"version":1})),
        reason: None,
    }
}

#[test]
fn offline_toggle_progress_updates_local_snapshot_and_enqueues_outbox() {
    let Fixture { dir, store } = fixture();
    ready(&store);
    let queued = store.set_progress(42, true).unwrap();
    store
        .save_code_draft("assignment", "python", "def solve(): pass")
        .unwrap();
    store.save_written_draft("report", "Saved offline").unwrap();
    drop(store);

    let path = dir.path().join("store.sqlite");
    assert!(matches!(
        SqliteStore::open(&path, "other"),
        Err(SyncError::AccountMismatch)
    ));
    let reopened = SqliteStore::open(path, "student").unwrap();
    assert_eq!(
        reopened.get_bootstrap_json().unwrap().unwrap()["progress"],
        json!([42])
    );
    assert_eq!(
        reopened.written_draft("report").unwrap().as_deref(),
        Some("Saved offline")
    );
    let edits = reopened.pending_mutations(50, i64::MAX).unwrap();
    assert_eq!(edits.len(), 1);
    assert_eq!(edits[0].idempotency_key, queued.idempotency_key);
    assert_eq!(edits[0].payload_json, queued.payload_json);
    assert_eq!(
        uuid::Uuid::parse_str(&queued.idempotency_key)
            .unwrap()
            .get_version_num(),
        7
    );
    assert_eq!(
        reopened
            .code_draft("assignment", "python")
            .unwrap()
            .as_deref(),
        Some("def solve(): pass")
    );
}

#[test]
fn push_replay_is_idempotent_under_duplicate_acks() {
    let fixture = fixture();
    let store = fixture.store;
    ready(&store);
    let mutation = store.set_progress(42, true).unwrap();
    let outcome = progress_outcome(&mutation);
    let batch = [mutation];
    store
        .apply_outcomes(&batch, &[outcome.clone(), outcome.clone()])
        .unwrap();
    store.apply_outcomes(&batch, &[outcome]).unwrap();
    assert_eq!(store.status_snapshot().unwrap().pending_count, 0);
    assert_eq!(
        store.get_bootstrap_json().unwrap().unwrap()["progress"],
        json!([42])
    );
}

#[test]
fn version_conflict_records_conflict_row_without_discarding_local_edit() {
    let fixture = fixture();
    let store = fixture.store;
    ready(&store);
    let local = json!({"name":"Offline edit"});
    let mutation = store.edit_profile(local.clone()).unwrap();
    assert_eq!(mutation.base_version, 1);
    assert_eq!(
        store.get_bootstrap_json().unwrap().unwrap()["user"]["name"],
        "Offline edit"
    );
    let server = json!({"id":"student","profile":{"name":"Remote edit"},"version":2});
    let outcome = PushItemOutcome {
        idempotency_key: mutation.idempotency_key.clone(),
        status: PushStatus::Conflict,
        server_version: Some(2),
        server_payload: Some(server.clone()),
        reason: None,
    };
    store.apply_outcomes(&[mutation], &[outcome]).unwrap();
    assert_eq!(
        store.get_bootstrap_json().unwrap().unwrap()["user"]["name"],
        "Remote edit"
    );
    let conflicts = store.conflicts().unwrap();
    assert_eq!(conflicts.len(), 1);
    assert_eq!(conflicts[0].local_payload, local);
    assert_eq!(conflicts[0].server_payload, server);
    assert_eq!(
        store
            .edit_profile(json!({"name":"Merged edit"}))
            .unwrap()
            .base_version,
        2
    );
    store.resolve_conflict(&conflicts[0].id).unwrap();
    assert!(store.conflicts().unwrap().is_empty());
}

#[test]
fn pull_delta_advances_cursor_atomically() {
    let fixture = fixture();
    let store = fixture.store;
    ready(&store);
    let progress = change(
        1,
        "lecture_progress",
        json!({"lectureId":42,"complete":true}),
    );
    let invalid = change(2, "discussion", json!({"id":"without-lecture"}));
    assert!(
        store
            .apply_changelog(0, &page(2, vec![progress.clone(), invalid]), None)
            .is_err()
    );
    assert_eq!(store.status_snapshot().unwrap().cursor, 0);
    assert_eq!(
        store.get_bootstrap_json().unwrap().unwrap()["progress"],
        json!([])
    );
    store
        .apply_changelog(0, &page(1, vec![progress]), None)
        .unwrap();
    assert_eq!(store.status_snapshot().unwrap().cursor, 1);
    assert_eq!(
        store.get_bootstrap_json().unwrap().unwrap()["progress"],
        json!([42])
    );
}

#[test]
fn discussion_optimistic_row_is_replaced_by_server_id_and_deduplicated_on_pull() {
    let fixture = fixture();
    let store = fixture.store;
    ready(&store);
    let mutation = store.post_discussion(42, "Hello").unwrap();
    let local = store.discussions(42).unwrap();
    assert_eq!(local[0]["id"], mutation.entity_id);
    assert_eq!(local[0]["pending"], true);
    let server = json!({"id":"server-discussion","lecture_id":42,"body":"Hello","created":"2026-11-25T00:00:00Z"});
    let outcome = PushItemOutcome {
        idempotency_key: mutation.idempotency_key.clone(),
        status: PushStatus::Applied,
        server_version: None,
        server_payload: Some(server.clone()),
        reason: None,
    };
    store.apply_outcomes(&[mutation], &[outcome]).unwrap();
    store
        .apply_changelog(0, &page(1, vec![change(1, "discussion", server)]), None)
        .unwrap();
    let discussions = store.discussions(42).unwrap();
    assert_eq!(discussions.len(), 1);
    assert_eq!(discussions[0]["id"], "server-discussion");
    assert_eq!(discussions[0]["pending"], false);
}

#[derive(Default)]
struct FailingRemote {
    pushes: AtomicUsize,
}

#[async_trait]
impl RemoteSyncPort for FailingRemote {
    async fn fetch_bootstrap(&self, _: &str) -> Result<Value, SyncError> {
        Ok(snapshot())
    }

    async fn push_mutations(
        &self,
        _: &str,
        _: &[OutboxMutation],
    ) -> Result<Vec<PushItemOutcome>, SyncError> {
        self.pushes.fetch_add(1, Ordering::SeqCst);
        Err(SyncError::Http {
            status: 503,
            retry_after_ms: Some(2_000),
        })
    }

    async fn pull_delta(&self, _: &str, cursor: i64, _: usize) -> Result<PullResponse, SyncError> {
        Ok(page(cursor, vec![]))
    }
}

#[tokio::test]
async fn transient_503_schedules_bounded_exponential_backoff() {
    let fixture = fixture();
    let store = fixture.store;
    ready(&store);
    let mutation = store.set_progress(42, true).unwrap();
    let engine = SyncEngine::new(Arc::clone(&store), FailingRemote::default()).unwrap();
    let status = engine.subscribe();
    let before = Utc::now().timestamp_millis();
    assert!(matches!(
        engine.synchronize("token").await,
        Err(SyncError::Http { status: 503, .. })
    ));
    assert_eq!(status.borrow().state, SyncState::Offline);
    let retry = store.next_retry_at().unwrap().unwrap();
    assert!(retry >= before + 2_000);
    assert!(store.pending_mutations(50, retry - 1).unwrap().is_empty());
    let pending = store.pending_mutations(50, retry).unwrap();
    assert_eq!(pending[0].retry_count, 1);
    assert_eq!(pending[0].idempotency_key, mutation.idempotency_key);
    assert_eq!(compute_backoff_ms(400, 1.0), 300_000);
}

#[test]
fn partial_catalog_page_persists_refresh_requirement_across_reopen() {
    let Fixture { dir, store } = fixture();
    ready(&store);
    let partial = PullResponse {
        cursor: 2,
        has_more: true,
        entries: vec![change(2, "course", json!({"id":"course"}))],
    };
    store.apply_changelog(0, &partial, None).unwrap();
    assert!(store.needs_bootstrap_refresh().unwrap());
    drop(store);
    let reopened = SqliteStore::open(dir.path().join("store.sqlite"), "student").unwrap();
    assert!(reopened.needs_bootstrap_refresh().unwrap());
    assert!(reopened.apply_changelog(2, &page(2, vec![]), None).is_err());
    let mut refreshed = snapshot();
    refreshed["fees"] = json!([{"id":"123","status":"Paid"}]);
    reopened
        .apply_changelog(2, &page(2, vec![]), Some(&refreshed))
        .unwrap();
    assert!(!reopened.needs_bootstrap_refresh().unwrap());
    assert_eq!(reopened.get_bootstrap_json().unwrap(), Some(refreshed));
}
