CREATE TABLE sync_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE workspace_snapshot (
    user_id TEXT PRIMARY KEY,
    payload TEXT NOT NULL CHECK (json_valid(payload)),
    version INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);

CREATE TABLE cached_discussions (
    id TEXT PRIMARY KEY,
    lecture_id INTEGER NOT NULL,
    payload TEXT NOT NULL CHECK (json_valid(payload)),
    created_at TEXT NOT NULL,
    pending INTEGER NOT NULL DEFAULT 0 CHECK (pending IN (0, 1))
);
CREATE INDEX cached_discussions_lecture ON cached_discussions(lecture_id, created_at, id);

CREATE TABLE local_code_drafts (
    assignment_id TEXT NOT NULL,
    language TEXT NOT NULL CHECK (language IN ('python', 'java')),
    source TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (assignment_id, language)
);

CREATE TABLE local_written_drafts (
    assignment_id TEXT PRIMARY KEY,
    answer TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE cached_assets (
    asset_id TEXT PRIMARY KEY,
    filename TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    local_path TEXT NOT NULL,
    byte_size INTEGER NOT NULL CHECK (byte_size >= 0),
    cached_at INTEGER NOT NULL
);

CREATE TABLE sync_outbox (
    queue_order INTEGER PRIMARY KEY,
    id TEXT NOT NULL UNIQUE,
    idempotency_key TEXT NOT NULL UNIQUE,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    endpoint_method TEXT NOT NULL,
    endpoint_path TEXT NOT NULL,
    base_version INTEGER NOT NULL CHECK (base_version >= 0),
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at INTEGER,
    last_error TEXT,
    created_at INTEGER NOT NULL
);

CREATE TABLE sync_conflicts (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    local_payload TEXT NOT NULL CHECK (json_valid(local_payload)),
    server_payload TEXT NOT NULL CHECK (json_valid(server_payload)),
    server_version INTEGER,
    reason TEXT NOT NULL,
    resolved INTEGER NOT NULL DEFAULT 0 CHECK (resolved IN (0, 1)),
    created_at INTEGER NOT NULL
);
CREATE INDEX sync_conflicts_unresolved ON sync_conflicts(created_at, id) WHERE resolved = 0;

PRAGMA user_version = 1;
