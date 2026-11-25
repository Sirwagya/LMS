use std::sync::Arc;

use lms_sync::{HttpRemote, LocalStorePort, SqliteStore, SyncEngine, SyncError, SyncState};
use serde_json::{Value, json};

#[tokio::test]
#[ignore = "requires LMS_SYNC_TEST_URL pointing to a seeded disposable Go API"]
async fn offline_edits_reconnect_to_go_api() {
    let base = std::env::var("LMS_SYNC_TEST_URL").unwrap();
    let client = reqwest::Client::new();
    let auth: Value = client
        .post(format!("{base}/api/v1/register"))
        .json(&json!({
            "name":"Phase Four", "password":"LocalSyncTestPassword123!",
            "email":format!("sync-{}@vedam.local", uuid::Uuid::now_v7())
        }))
        .send()
        .await
        .unwrap()
        .error_for_status()
        .unwrap()
        .json()
        .await
        .unwrap();
    let token = auth["token"].as_str().unwrap();
    let account = auth["user"]["id"].as_str().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("offline.sqlite");
    let store = Arc::new(SqliteStore::open(&path, account).unwrap());
    let engine = SyncEngine::new(Arc::clone(&store), HttpRemote::new(&base).unwrap()).unwrap();
    assert_eq!(
        engine.synchronize(token).await.unwrap().state,
        SyncState::Synced
    );
    let workspace = store.get_bootstrap_json().unwrap().unwrap();
    let lecture = workspace["lecturesBySubject"]
        .as_object()
        .unwrap()
        .values()
        .filter_map(|rows| rows.as_array())
        .flatten()
        .find_map(|lecture| lecture["id"].as_i64())
        .unwrap();

    store
        .edit_profile(json!({"name":"Offline Profile"}))
        .unwrap();
    store.set_progress(lecture, true).unwrap();
    let discussion = store.post_discussion(lecture, "Written offline").unwrap();
    assert_eq!(
        store.get_bootstrap_json().unwrap().unwrap()["progress"],
        json!([lecture])
    );
    assert_eq!(store.status_snapshot().unwrap().pending_count, 3);
    assert!(
        store
            .discussions(lecture)
            .unwrap()
            .iter()
            .any(|row| row["id"] == discussion.entity_id && row["pending"] == true)
    );
    drop(engine);
    drop(store);

    client
        .put(format!("{base}/api/v1/profile"))
        .bearer_auth(token)
        .json(&json!({"profile":{"name":"Remote Profile"}}))
        .send()
        .await
        .unwrap()
        .error_for_status()
        .unwrap();

    let store = Arc::new(SqliteStore::open(path, account).unwrap());
    let engine = SyncEngine::new(Arc::clone(&store), HttpRemote::new(&base).unwrap()).unwrap();
    let status = engine.synchronize(token).await.unwrap();
    assert!(status.cursor > 0);
    assert_eq!((status.pending_count, status.conflict_count), (0, 1));
    assert_eq!(
        store.conflicts().unwrap()[0].local_payload["name"],
        "Offline Profile"
    );
    let projection = store.get_bootstrap_json().unwrap().unwrap();
    assert_eq!(projection["user"]["name"], "Remote Profile");
    assert_eq!(projection["progress"], json!([lecture]));
    let discussions = store.discussions(lecture).unwrap();
    let posted: Vec<_> = discussions
        .iter()
        .filter(|row| row["body"] == "Written offline")
        .collect();
    assert_eq!(posted.len(), 1);
    assert_eq!(posted[0]["pending"], false);
    assert_ne!(posted[0]["id"], discussion.entity_id);
    assert_eq!(
        engine.synchronize(token).await.unwrap().cursor,
        status.cursor
    );
    assert!(matches!(
        engine.synchronize("invalid").await,
        Err(SyncError::Unauthorized)
    ));
}
