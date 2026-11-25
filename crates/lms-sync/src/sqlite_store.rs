use std::{
    path::Path,
    sync::{Mutex, MutexGuard},
    time::Duration,
};

use chrono::{DateTime, Utc};
use rusqlite::{Connection, OptionalExtension, Transaction, TransactionBehavior, params};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use uuid::Uuid;

use crate::conflict::acknowledge_by_key;
use crate::projection::{self, invalid};
use crate::{
    LocalStorePort, OutboxMutation, PullResponse, PushItemOutcome, PushStatus, SyncError,
    SyncStatusSnapshot,
};

const MIGRATION: &str = include_str!("../migrations/001_local_sync.sql");
const OUTBOX_FIELDS: &str = "id,idempotency_key,entity_type,entity_id,endpoint_method,endpoint_path,base_version,payload_json,retry_count";
// The Go API caps the entire JSON request at 16 MiB. Leave room for the
// envelope and mutation metadata in a batch of up to 200 edits.
const MAX_BATCH_PAYLOAD: usize = 15 << 20;
const CURSOR: &str = "cursor";
const NEEDS_REFRESH: &str = "needs_bootstrap_refresh";

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Conflict {
    pub id: String,
    pub entity_type: String,
    pub entity_id: String,
    pub local_payload: Value,
    pub server_payload: Value,
    pub server_version: Option<i64>,
    pub reason: String,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CachedAsset {
    pub id: String,
    pub filename: String,
    pub mime_type: String,
    pub local_path: String,
    pub byte_size: i64,
}

/// Open one file per account. A file is permanently bound to the first account
/// that opens it, including its drafts, conflicts and unapplied outbox entries.
pub struct SqliteStore {
    db: Mutex<Connection>,
    account_id: String,
}

impl SqliteStore {
    pub fn open(path: impl AsRef<Path>, account_id: impl Into<String>) -> Result<Self, SyncError> {
        let account_id = account_id.into();
        if account_id.trim().is_empty() {
            return Err(invalid("account id cannot be empty"));
        }
        let mut db = Connection::open(path)?;
        db.busy_timeout(Duration::from_secs(5))?;
        db.execute_batch(
            "PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA foreign_keys=ON;",
        )?;
        let tx = db.transaction_with_behavior(TransactionBehavior::Immediate)?;
        let version: i64 = tx.pragma_query_value(None, "user_version", |row| row.get(0))?;
        match version {
            0 => tx.execute_batch(MIGRATION)?,
            1 => {}
            _ => return Err(invalid("unsupported local schema version")),
        }
        match meta(&tx, "account")? {
            Some(stored) if stored != account_id => return Err(SyncError::AccountMismatch),
            None => set_meta(&tx, "account", &account_id)?,
            _ => {}
        }
        tx.commit()?;
        Ok(Self {
            db: Mutex::new(db),
            account_id,
        })
    }

    fn db(&self) -> Result<MutexGuard<'_, Connection>, SyncError> {
        self.db.lock().map_err(|_| SyncError::StorageLock)
    }

    fn write<T>(
        &self,
        operation: impl FnOnce(&Transaction<'_>) -> Result<T, SyncError>,
    ) -> Result<T, SyncError> {
        let mut db = self.db()?;
        let tx = db.transaction_with_behavior(TransactionBehavior::Immediate)?;
        let result = operation(&tx)?;
        tx.commit()?;
        Ok(result)
    }

    /// Returns the canonical workspace with pending edits applied in queue order.
    pub fn get_bootstrap_json(&self) -> Result<Option<Value>, SyncError> {
        let mut db = self.db()?;
        let tx = db.transaction()?;
        let Some(mut snapshot) = read_snapshot(&tx, &self.account_id)? else {
            return Ok(None);
        };
        let mut statement = tx.prepare("SELECT entity_type, base_version, payload_json FROM sync_outbox WHERE entity_type IN ('profile','lecture_progress') ORDER BY queue_order")?;
        let mut edits = statement.query([])?;
        while let Some(row) = edits.next()? {
            let kind: String = row.get(0)?;
            let base: i64 = row.get(1)?;
            let payload: Value = serde_json::from_str(&row.get::<_, String>(2)?)?;
            if kind == "profile" {
                // A pulled update can arrive while this optimistic edit is still
                // queued. Keep the edit visible until its server outcome is known.
                let shown_version = snapshot
                    .pointer("/user/version")
                    .and_then(Value::as_i64)
                    .unwrap_or(0);
                projection::profile(
                    &mut snapshot,
                    &payload,
                    shown_version.max(base.saturating_add(1)),
                )?;
            } else {
                let (lecture, complete) = projection::progress_values(&payload)?;
                projection::progress(&mut snapshot, lecture, complete)?;
            }
        }
        Ok(Some(snapshot))
    }

    pub fn edit_profile(&self, profile: Value) -> Result<OutboxMutation, SyncError> {
        if profile
            .get("name")
            .and_then(Value::as_str)
            .is_none_or(|name| name.trim().is_empty())
        {
            return Err(invalid("profile requires a name"));
        }
        self.write(|tx| {
            let snapshot = required_snapshot(tx, &self.account_id)?;
            let version = snapshot
                .pointer("/user/version")
                .and_then(Value::as_i64)
                .ok_or_else(|| invalid("bootstrap is missing the profile version"))?;
            let queued: Option<i64> = tx.query_row(
                "SELECT MAX(base_version) FROM sync_outbox WHERE entity_type='profile'",
                [],
                |row| row.get(0),
            )?;
            let base = queued.map_or(version, |previous| version.max(previous.saturating_add(1)));
            enqueue(tx, "profile", &self.account_id, base, profile)
        })
    }

    pub fn set_progress(
        &self,
        lecture_id: i64,
        complete: bool,
    ) -> Result<OutboxMutation, SyncError> {
        if lecture_id <= 0 {
            return Err(invalid("invalid lecture id"));
        }
        self.write(|tx| {
            required_snapshot(tx, &self.account_id)?;
            enqueue(
                tx,
                "lecture_progress",
                &lecture_id.to_string(),
                0,
                json!({
                    "lectureId": lecture_id, "complete": complete, "clientTimestamp": now_ms()
                }),
            )
        })
    }

    pub fn post_discussion(
        &self,
        lecture_id: i64,
        body: &str,
    ) -> Result<OutboxMutation, SyncError> {
        let body = body.trim();
        if lecture_id <= 0 || body.is_empty() || body.encode_utf16().count() > 2_000 {
            return Err(invalid("invalid discussion message"));
        }
        self.write(|tx| {
            let snapshot = required_snapshot(tx, &self.account_id)?;
            let mutation_id = Uuid::now_v7().to_string();
            let mutation = enqueue(tx, "discussion", &mutation_id, 0, json!({
                "lectureId": lecture_id, "body": body
            }))?;
            write_discussion(tx, &json!({
                "id": mutation.entity_id, "lecture_id": lecture_id, "user_id": self.account_id,
                "name": snapshot["user"]["name"], "body": body, "created": Utc::now().to_rfc3339()
            }), true)?;
            Ok(mutation)
        })
    }

    pub fn discussions(&self, lecture_id: i64) -> Result<Vec<Value>, SyncError> {
        let db = self.db()?;
        let mut query = db.prepare_cached("SELECT payload,pending FROM cached_discussions WHERE lecture_id=? ORDER BY created_at,id")?;
        let mut rows = query.query([lecture_id])?;
        let mut discussions = Vec::new();
        while let Some(row) = rows.next()? {
            let mut payload: Value = serde_json::from_str(&row.get::<_, String>(0)?)?;
            payload["pending"] = json!(row.get::<_, bool>(1)?);
            discussions.push(payload);
        }
        Ok(discussions)
    }

    pub fn save_code_draft(
        &self,
        assignment: &str,
        language: &str,
        source: &str,
    ) -> Result<(), SyncError> {
        self.db()?.execute(
            "INSERT INTO local_code_drafts VALUES (?,?,?,?) ON CONFLICT(assignment_id,language) DO UPDATE SET source=excluded.source,updated_at=excluded.updated_at",
            params![assignment, language, source, now_ms()],
        )?;
        Ok(())
    }

    pub fn code_draft(
        &self,
        assignment: &str,
        language: &str,
    ) -> Result<Option<String>, SyncError> {
        Ok(self
            .db()?
            .query_row(
                "SELECT source FROM local_code_drafts WHERE assignment_id=? AND language=?",
                params![assignment, language],
                |row| row.get(0),
            )
            .optional()?)
    }

    pub fn save_written_draft(&self, assignment: &str, answer: &str) -> Result<(), SyncError> {
        self.db()?.execute(
            "INSERT INTO local_written_drafts VALUES (?,?,?) ON CONFLICT(assignment_id) DO UPDATE SET answer=excluded.answer,updated_at=excluded.updated_at",
            params![assignment,answer,now_ms()],
        )?;
        Ok(())
    }

    pub fn written_draft(&self, assignment: &str) -> Result<Option<String>, SyncError> {
        Ok(self
            .db()?
            .query_row(
                "SELECT answer FROM local_written_drafts WHERE assignment_id=?",
                [assignment],
                |row| row.get(0),
            )
            .optional()?)
    }

    pub fn save_asset(&self, asset: &CachedAsset) -> Result<(), SyncError> {
        self.db()?.execute(
            "INSERT INTO cached_assets VALUES (?,?,?,?,?,?) ON CONFLICT(asset_id) DO UPDATE SET filename=excluded.filename,mime_type=excluded.mime_type,local_path=excluded.local_path,byte_size=excluded.byte_size,cached_at=excluded.cached_at",
            params![asset.id, asset.filename, asset.mime_type, asset.local_path, asset.byte_size, now_ms()],
        )?;
        Ok(())
    }

    pub fn asset(&self, id: &str) -> Result<Option<CachedAsset>, SyncError> {
        Ok(self.db()?.query_row(
            "SELECT asset_id,filename,mime_type,local_path,byte_size FROM cached_assets WHERE asset_id=?", [id],
            |row| Ok(CachedAsset { id: row.get(0)?, filename: row.get(1)?, mime_type: row.get(2)?, local_path: row.get(3)?, byte_size: row.get(4)? }),
        ).optional()?)
    }

    pub fn conflicts(&self) -> Result<Vec<Conflict>, SyncError> {
        let db = self.db()?;
        let mut stmt = db.prepare_cached("SELECT id,entity_type,entity_id,local_payload,server_payload,server_version,reason FROM sync_conflicts WHERE resolved=0 ORDER BY created_at,id")?;
        let mut rows = stmt.query([])?;
        let mut conflicts = Vec::new();
        while let Some(row) = rows.next()? {
            conflicts.push(Conflict {
                id: row.get(0)?,
                entity_type: row.get(1)?,
                entity_id: row.get(2)?,
                local_payload: serde_json::from_str(&row.get::<_, String>(3)?)?,
                server_payload: serde_json::from_str(&row.get::<_, String>(4)?)?,
                server_version: row.get(5)?,
                reason: row.get(6)?,
            });
        }
        Ok(conflicts)
    }

    /// Mark a conflict resolved after the user discards it or queues a merged edit.
    pub fn resolve_conflict(&self, conflict_id: &str) -> Result<(), SyncError> {
        self.db()?.execute(
            "UPDATE sync_conflicts SET resolved=1 WHERE id=?",
            [conflict_id],
        )?;
        Ok(())
    }
}

fn now_ms() -> i64 {
    Utc::now().timestamp_millis()
}

fn meta(db: &Connection, key: &str) -> Result<Option<String>, SyncError> {
    Ok(db
        .query_row("SELECT value FROM sync_meta WHERE key=?", [key], |row| {
            row.get(0)
        })
        .optional()?)
}

fn meta_number(db: &Connection, key: &str) -> Result<Option<i64>, SyncError> {
    meta(db, key)?
        .map(|value| {
            value
                .parse()
                .map_err(|_| invalid("invalid local sync metadata"))
        })
        .transpose()
}

fn set_meta(db: &Connection, key: &str, value: impl ToString) -> Result<(), SyncError> {
    db.execute(
        "INSERT INTO sync_meta VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
        params![key, value.to_string()],
    )?;
    Ok(())
}

fn read_snapshot(db: &Connection, account: &str) -> Result<Option<Value>, SyncError> {
    let snapshot: Option<String> = db
        .query_row(
            "SELECT payload FROM workspace_snapshot WHERE user_id=?",
            [account],
            |row| row.get(0),
        )
        .optional()?;
    Ok(snapshot.map(|raw| serde_json::from_str(&raw)).transpose()?)
}

fn required_snapshot(db: &Connection, account: &str) -> Result<Value, SyncError> {
    read_snapshot(db, account)?.ok_or_else(|| invalid("workspace must be hydrated first"))
}

fn write_snapshot(
    db: &Connection,
    account: &str,
    snapshot: &Value,
    version: i64,
) -> Result<(), SyncError> {
    projection::account_snapshot(snapshot, account)?;
    db.execute(
        "INSERT INTO workspace_snapshot VALUES (?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET payload=excluded.payload,version=excluded.version,updated_at=excluded.updated_at",
        params![account,serde_json::to_string(snapshot)?,version,now_ms()],
    )?;
    Ok(())
}

fn enqueue(
    db: &Connection,
    kind: &str,
    entity: &str,
    base: i64,
    payload: Value,
) -> Result<OutboxMutation, SyncError> {
    let id = Uuid::now_v7().to_string();
    let mutation = OutboxMutation {
        id: id.clone(),
        idempotency_key: id,
        entity_type: kind.into(),
        entity_id: entity.into(),
        endpoint_method: "POST".into(),
        endpoint_path: "/api/v1/sync/push".into(),
        base_version: base,
        payload_json: payload,
        retry_count: 0,
    };
    let payload = json_string(&mutation.payload_json)?;
    if payload.len() > MAX_BATCH_PAYLOAD {
        return Err(invalid(
            "offline edit exceeds the maximum sync request size",
        ));
    }
    db.execute(
        "INSERT INTO sync_outbox(id,idempotency_key,entity_type,entity_id,endpoint_method,endpoint_path,base_version,payload_json,created_at) VALUES(?,?,?,?,?,?,?,?,?)",
        params![mutation.id,mutation.idempotency_key,kind,entity,mutation.endpoint_method,mutation.endpoint_path,base,payload,now_ms()],
    )?;
    Ok(mutation)
}

fn json_string(value: &Value) -> Result<String, SyncError> {
    Ok(serde_json::to_string(value)?)
}

fn write_discussion(db: &Connection, value: &Value, pending: bool) -> Result<(), SyncError> {
    let id = value
        .get("id")
        .and_then(Value::as_str)
        .filter(|id| !id.is_empty());
    let lecture = value
        .get("lecture_id")
        .and_then(Value::as_i64)
        .filter(|id| *id > 0);
    let created = value.get("created").and_then(Value::as_str);
    let Some(((id, lecture), created)) = id.zip(lecture).zip(created) else {
        return Err(invalid("invalid discussion payload"));
    };
    db.execute(
        "INSERT INTO cached_discussions VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET lecture_id=excluded.lecture_id,payload=excluded.payload,created_at=excluded.created_at,pending=excluded.pending",
        params![id,lecture,json_string(value)?,created,pending],
    )?;
    Ok(())
}

impl LocalStorePort for SqliteStore {
    fn account_id(&self) -> &str {
        &self.account_id
    }

    fn has_bootstrap(&self) -> Result<bool, SyncError> {
        Ok(self.db()?.query_row(
            "SELECT EXISTS(SELECT 1 FROM workspace_snapshot WHERE user_id=?)",
            [&self.account_id],
            |row| row.get(0),
        )?)
    }

    fn save_bootstrap_json(&self, snapshot: &Value) -> Result<(), SyncError> {
        self.write(|tx| {
            if read_snapshot(tx, &self.account_id)?.is_none() {
                write_snapshot(tx, &self.account_id, snapshot, 0)?;
                set_meta(tx, NEEDS_REFRESH, 1)?;
            }
            Ok(())
        })
    }

    fn pending_mutations(
        &self,
        limit: usize,
        now_ms: i64,
    ) -> Result<Vec<OutboxMutation>, SyncError> {
        let db = self.db()?;
        // Stop at the earliest deferred edit so a subsequent edit cannot
        // overtake a replay of an older profile or discussion mutation.
        let sql = format!(
            "SELECT {OUTBOX_FIELDS},next_retry_at FROM sync_outbox ORDER BY queue_order LIMIT ?"
        );
        let mut query = db.prepare_cached(&sql)?;
        let mut rows = query.query([limit.min(200) as i64])?;
        let mut outbox = Vec::new();
        let mut payload_bytes = 0;
        while let Some(row) = rows.next()? {
            if row
                .get::<_, Option<i64>>(9)?
                .is_some_and(|due| due > now_ms)
            {
                break;
            }
            let payload: String = row.get(7)?;
            payload_bytes += payload.len();
            if payload_bytes > MAX_BATCH_PAYLOAD {
                break;
            }
            outbox.push(OutboxMutation {
                id: row.get(0)?,
                idempotency_key: row.get(1)?,
                entity_type: row.get(2)?,
                entity_id: row.get(3)?,
                endpoint_method: row.get(4)?,
                endpoint_path: row.get(5)?,
                base_version: row.get(6)?,
                payload_json: serde_json::from_str(&payload)?,
                retry_count: row.get(8)?,
            });
        }
        Ok(outbox)
    }

    fn apply_outcomes(
        &self,
        batch: &[OutboxMutation],
        outcomes: &[PushItemOutcome],
    ) -> Result<(), SyncError> {
        let outcomes = acknowledge_by_key(outcomes)?;
        if outcomes.len() != batch.len()
            || batch
                .iter()
                .any(|item| !outcomes.contains_key(item.idempotency_key.as_str()))
        {
            return Err(invalid("acknowledgements do not match the submitted batch"));
        }
        self.write(|tx| {
            let mut snapshot = required_snapshot(tx, &self.account_id)?;
            for mutation in batch {
                if tx.execute("DELETE FROM sync_outbox WHERE idempotency_key=?", [&mutation.idempotency_key])? == 0 {
                    continue;
                }
                let outcome = outcomes[mutation.idempotency_key.as_str()];
                if mutation.entity_type == "discussion" {
                    tx.execute("DELETE FROM cached_discussions WHERE id=? AND pending=1", [&mutation.entity_id])?;
                }
                if let Some(payload) = &outcome.server_payload {
                    match mutation.entity_type.as_str() {
                        "profile" => apply_profile(&mut snapshot, &self.account_id, payload)?,
                        "lecture_progress" => {
                            let (lecture, complete) = projection::progress_values(payload)?;
                            update_progress(tx, &mut snapshot, lecture, complete, outcome.server_version)?;
                        }
                        "discussion" => write_discussion(tx, payload, false)?,
                        _ => {}
                    }
                } else if outcome.status == PushStatus::Applied {
                    return Err(invalid("applied edit is missing its authoritative payload"));
                }
                if outcome.status != PushStatus::Applied {
                    let reason = outcome.reason.as_deref().unwrap_or(match outcome.status {
                        PushStatus::Conflict => "This record changed on the server.",
                        _ => "The server rejected this edit.",
                    });
                    let server = outcome.server_payload.as_ref().unwrap_or(&Value::Null);
                    tx.execute(
                        "INSERT INTO sync_conflicts(id,entity_type,entity_id,local_payload,server_payload,server_version,reason,created_at) VALUES (?,?,?,?,?,?,?,?)",
                        params![mutation.id,mutation.entity_type,mutation.entity_id,json_string(&mutation.payload_json)?,
                            json_string(server)?,outcome.server_version,reason,now_ms()],
                    )?;
                }
            }
            write_snapshot(tx,&self.account_id,&snapshot,meta_number(tx,CURSOR)?.unwrap_or(0))
        })
    }

    fn schedule_retry(
        &self,
        batch: &[OutboxMutation],
        next_ms: i64,
        error: &str,
    ) -> Result<(), SyncError> {
        self.write(|tx| {
            for mutation in batch {
                tx.execute(
                    "UPDATE sync_outbox SET retry_count=retry_count+1,next_retry_at=?,last_error=? WHERE idempotency_key=? AND retry_count=?",
                    params![next_ms,error,mutation.idempotency_key,mutation.retry_count],
                )?;
            }
            Ok(())
        })
    }

    fn needs_bootstrap_refresh(&self) -> Result<bool, SyncError> {
        let db = self.db()?;
        Ok(meta_number(&db, NEEDS_REFRESH)? == Some(1))
    }

    fn apply_changelog(
        &self,
        expected_cursor: i64,
        page: &PullResponse,
        refreshed_snapshot: Option<&Value>,
    ) -> Result<(), SyncError> {
        let mut previous = expected_cursor;
        for entry in &page.entries {
            if entry.seq <= previous
                || entry.seq > page.cursor
                || !matches!(entry.op.as_str(), "delete" | "upsert")
            {
                return Err(invalid("invalid changelog sequence or operation"));
            }
            if entry.scope != "global"
                && entry.scope.strip_prefix("user:") != Some(self.account_id.as_str())
            {
                return Err(SyncError::AccountMismatch);
            }
            previous = entry.seq;
        }
        if previous != page.cursor || (page.has_more && previous == expected_cursor) {
            return Err(invalid("invalid changelog watermark"));
        }
        self.write(|tx| {
            if meta_number(tx, CURSOR)?.unwrap_or(0) != expected_cursor {
                return Ok(()); // Another connection already applied a page.
            }
            let refresh_needed = meta_number(tx, NEEDS_REFRESH)? == Some(1)
                || page.entries.iter().any(|entry| {
                    projection::refreshes_workspace(&entry.entity_type)
                        || entry.op == "delete" && entry.entity_type == "profile"
                });
            if refresh_needed && !page.has_more && refreshed_snapshot.is_none() {
                return Err(invalid("the refreshed workspace is missing"));
            }
            let mut snapshot = if let Some(fresh) = refreshed_snapshot {
                fresh.clone()
            } else {
                required_snapshot(tx, &self.account_id)?
            };
            for entry in &page.entries {
                if entry.entity_type == "discussion" {
                    if entry.op == "delete" {
                        tx.execute(
                            "DELETE FROM cached_discussions WHERE id=? AND pending=0",
                            [&entry.entity_id],
                        )?;
                    } else {
                        write_discussion(tx, &entry.payload, false)?;
                    }
                    continue;
                }
                // Leave the canonical workspace in place until the tail
                // refresh commits. This avoids replaying obsolete catalog edits.
                if refresh_needed || refreshed_snapshot.is_some() {
                    continue;
                }
                match entry.entity_type.as_str() {
                    "profile" => apply_profile(&mut snapshot, &self.account_id, &entry.payload)?,
                    "lecture_progress" => {
                        let (lecture, complete) = if entry.op == "delete" {
                            (
                                entry
                                    .entity_id
                                    .parse()
                                    .map_err(|_| invalid("invalid lecture id"))?,
                                false,
                            )
                        } else {
                            projection::progress_values(&entry.payload)?
                        };
                        update_progress(tx, &mut snapshot, lecture, complete, Some(entry.seq))?;
                    }
                    _ => {}
                }
            }
            if !page.entries.is_empty() || refreshed_snapshot.is_some() {
                write_snapshot(tx, &self.account_id, &snapshot, page.cursor)?;
                set_meta(tx, CURSOR, page.cursor)?;
            }
            set_meta(
                tx,
                NEEDS_REFRESH,
                i64::from(refresh_needed && refreshed_snapshot.is_none()),
            )
        })
    }

    fn status_snapshot(&self) -> Result<SyncStatusSnapshot, SyncError> {
        let mut db = self.db()?;
        let tx = db.transaction()?;
        Ok(SyncStatusSnapshot {
            cursor: meta_number(&tx, CURSOR)?.unwrap_or(0),
            pending_count: tx
                .query_row("SELECT COUNT(*) FROM sync_outbox", [], |row| row.get(0))?,
            conflict_count: tx.query_row(
                "SELECT COUNT(*) FROM sync_conflicts WHERE resolved=0",
                [],
                |row| row.get(0),
            )?,
            last_synced_at: meta_number(&tx, "last_synced_at")?
                .and_then(DateTime::from_timestamp_millis),
            ..SyncStatusSnapshot::default()
        })
    }

    fn record_sync_success(&self, timestamp: i64) -> Result<(), SyncError> {
        let db = self.db()?;
        set_meta(&db, "last_synced_at", timestamp)
    }

    fn next_retry_at(&self) -> Result<Option<i64>, SyncError> {
        Ok(self
            .db()?
            .query_row(
                "SELECT next_retry_at FROM sync_outbox ORDER BY queue_order LIMIT 1",
                [],
                |row| row.get::<_, Option<i64>>(0),
            )
            .optional()?
            .flatten())
    }
}

fn apply_profile(snapshot: &mut Value, account: &str, payload: &Value) -> Result<(), SyncError> {
    if payload.get("id").and_then(Value::as_str) != Some(account) {
        return Err(SyncError::AccountMismatch);
    }
    let profile = payload
        .get("profile")
        .ok_or_else(|| invalid("invalid profile payload"))?;
    let version = payload
        .get("version")
        .and_then(Value::as_i64)
        .ok_or_else(|| invalid("invalid profile version"))?;
    projection::profile(snapshot, profile, version)
}

fn update_progress(
    db: &Connection,
    snapshot: &mut Value,
    lecture: i64,
    complete: bool,
    version: Option<i64>,
) -> Result<(), SyncError> {
    let key = format!("progress:{lecture}");
    if let Some(version) = version {
        if meta_number(db, &key)?.is_some_and(|current| current > version) {
            return Ok(());
        }
        set_meta(db, &key, version)?;
    }
    projection::progress(snapshot, lecture, complete)
}
