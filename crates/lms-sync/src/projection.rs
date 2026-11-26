use crate::SyncError;
use base64::{Engine as _, engine::general_purpose::STANDARD};
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

pub(crate) fn validate_profile(profile: &Value) -> Result<(), SyncError> {
    if profile
        .get("name")
        .and_then(Value::as_str)
        .is_none_or(|name| name.trim().is_empty() || name.encode_utf16().count() > 100)
    {
        return Err(invalid("profile requires a name of at most 100 characters"));
    }
    const MAX_IMAGE: usize = 5 << 20;
    const MAX_IMAGE_ENCODING: usize = MAX_IMAGE.div_ceil(3) * 4;
    for key in ["avatarUrl", "bannerUrl"] {
        let Some(field) = profile.get(key).filter(|value| !value.is_null()) else {
            continue;
        };
        let image = field
            .as_str()
            .ok_or_else(|| invalid("invalid profile image"))?;
        if image.is_empty() {
            continue;
        }
        let (media, data) = image
            .split_once(',')
            .ok_or_else(|| invalid("invalid profile image"))?;
        if !matches!(
            media,
            "data:image/png;base64"
                | "data:image/jpeg;base64"
                | "data:image/webp;base64"
                | "data:image/gif;base64"
        ) || data.is_empty()
            || data.len() > MAX_IMAGE_ENCODING
            || !STANDARD
                .decode(data)
                .is_ok_and(|decoded| decoded.len() <= MAX_IMAGE)
        {
            return Err(invalid("upload a PNG, JPEG, WebP or GIF image up to 5 MB"));
        }
    }
    if let Some(platforms) = profile.get("platforms").filter(|value| !value.is_null()) {
        let records = platforms
            .as_array()
            .filter(|records| records.len() <= 32)
            .ok_or_else(|| invalid("invalid profile platforms"))?;
        for platform in records {
            if platform.is_null() {
                continue;
            }
            if !platform.is_object() {
                return Err(invalid("invalid profile platform"));
            }
            let Some(field) = platform.get("url").filter(|value| !value.is_null()) else {
                continue;
            };
            let link = field
                .as_str()
                .ok_or_else(|| invalid("invalid profile link"))?;
            if link.is_empty() {
                continue;
            }
            if link.len() > 2048 || !(link.starts_with("http://") || link.starts_with("https://")) {
                return Err(invalid("profile links must begin with https:// or http://"));
            }
            let authority = link
                .split_once("://")
                .map(|(_, rest)| rest.split(['/', '?', '#']).next().unwrap_or_default())
                .unwrap_or_default();
            if authority.is_empty() || authority.contains('\\') {
                return Err(invalid("invalid profile link"));
            }
            let parsed = reqwest::Url::parse(link).map_err(|_| invalid("invalid profile link"))?;
            if parsed.host().is_none()
                || !parsed.username().is_empty()
                || parsed.password().is_some()
            {
                return Err(invalid("invalid profile link"));
            }
        }
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
