use crate::SyncError;
use serde_json::{Value, json};

pub(crate) fn invalid(message: &str) -> SyncError {
    SyncError::Protocol(message.into())
}

pub(crate) fn account_snapshot(snapshot: &Value, account: &str) -> Result<(), SyncError> {
    if snapshot.pointer("/user/id").and_then(Value::as_str) != Some(account) {
        return Err(SyncError::AccountMismatch);
    }
    if !snapshot.get("progress").is_some_and(Value::is_array) {
        return Err(invalid("bootstrap is missing lecture progress"));
    }
    Ok(())
}

pub(crate) fn profile(snapshot: &mut Value, data: &Value, version: i64) -> Result<(), SyncError> {
    let user = snapshot
        .get_mut("user")
        .and_then(Value::as_object_mut)
        .ok_or_else(|| invalid("bootstrap is missing its user"))?;
    if user
        .get("version")
        .and_then(Value::as_i64)
        .unwrap_or_default()
        > version
    {
        return Ok(());
    }
    let object = data
        .as_object()
        .ok_or_else(|| invalid("profile must be an object"))?;
    if let Some(name) = object.get("name").and_then(Value::as_str) {
        user.insert("name".into(), json!(name.trim()));
    }
    user.insert("profile".into(), data.clone());
    user.insert("version".into(), json!(version));
    Ok(())
}

pub(crate) fn progress(snapshot: &mut Value, lecture_id: i64, done: bool) -> Result<(), SyncError> {
    let progress = snapshot
        .get_mut("progress")
        .and_then(Value::as_array_mut)
        .ok_or_else(|| invalid("bootstrap is missing lecture progress"))?;
    progress.retain(|item| item.as_i64() != Some(lecture_id));
    if done {
        progress.push(json!(lecture_id));
    }
    Ok(())
}

pub(crate) fn progress_values(payload: &Value) -> Result<(i64, bool), SyncError> {
    let id = payload
        .get("lectureId")
        .and_then(Value::as_i64)
        .filter(|id| *id > 0);
    let complete = payload.get("complete").and_then(Value::as_bool);
    id.zip(complete)
        .ok_or_else(|| invalid("invalid lecture progress payload"))
}

pub(crate) fn refreshes_workspace(entity_type: &str) -> bool {
    !matches!(entity_type, "profile" | "lecture_progress" | "discussion")
}
