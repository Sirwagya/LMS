# Phase 5 — Native macOS Tauri v2 App & Frontend `TransportPort`

> **Prerequisite**: [Phase 3](file:///Users/sirwagyashekhar/Projects/LMS/docs/phases/PHASE-3-RUST-CODE-GRADER.md) and [Phase 4](file:///Users/sirwagyashekhar/Projects/LMS/docs/phases/PHASE-4-RUST-OFFLINE-SYNC-ENGINE.md) crates passing tests.
> **Phase Goal**: Wrap [`src/`](file:///Users/sirwagyashekhar/Projects/LMS/src/App.tsx) in a native macOS **Tauri v2** shell (`src-tauri/`), refactor [`src/portal/api.ts`](file:///Users/sirwagyashekhar/Projects/LMS/src/portal/api.ts) onto a platform-agnostic `TransportPort` (Web `fetch` vs. macOS Tauri IPC), store tokens securely in **macOS Keychain**, add native window chrome, menu bar shortcuts, notifications, local grader practice mode, and package the signed `.app` / `.dmg` bundle.

---

## 1. Frontend Dependency Inversion (`TransportPort` in `src/portal/`)

Currently [`src/portal/api.ts`](file:///Users/sirwagyashekhar/Projects/LMS/src/portal/api.ts) directly calls `fetch("/api" + path)`. Introducing a narrow `TransportPort` interface ensures every React screen ([`DashboardScreen.tsx`](file:///Users/sirwagyashekhar/Projects/LMS/src/components/screens/DashboardScreen.tsx), [`AccountScreen.tsx`](file:///Users/sirwagyashekhar/Projects/LMS/src/components/screens/AccountScreen.tsx), [`CodingWorkspace.tsx`](file:///Users/sirwagyashekhar/Projects/LMS/src/portal/coding/CodingWorkspace.tsx)) runs identically on Web and macOS without `if (isTauri)` conditionals scattered through UI components (DIP + OCP).

```ts
// src/portal/transport.ts
export interface SyncStatus {
  state: "synced" | "syncing" | "offline" | "error";
  pendingCount: number;
  conflictCount: number;
  lastSyncedAt: string | null;
}

export interface TransportPort {
  request<T = { ok: boolean }>(path: string, method?: string, data?: unknown): Promise<T>;
  subscribeEvents(onEvent: (eventName: string, payload: unknown) => void): () => void;
  triggerSyncNow(): Promise<SyncStatus>;
  getSyncStatus(): Promise<SyncStatus>;
  downloadAsset(url: string, filename: string): Promise<void>;
}
```

### Runtime Adapter Selection (`src/portal/api.ts`)

- **`HttpTransport`** (Web browser): Calls `fetch("/api" + path, { credentials: "same-origin" })`, listens on `EventSource("/api/v1/events")`, reports `{ state: navigator.onLine ? "synced" : "offline", pendingCount: 0, ... }`.
- **`TauriOfflineTransport`** (detected via `"__TAURI_INTERNALS__" in window`):
  - Routes `/api/bootstrap` and reads through `invoke("lms_request", { path, method, body })` backed by `lms-sync` SQLite WAL (`< 1 ms`).
  - Routes `/api/coding/evaluate` with `action === "run"` locally through `lms-grader` inside `src-tauri` when offline, and routes `action === "submit"` (or online runs) to the Go backend.
  - Listens to native `tauri://sync-updated` events emitted by the Rust background Tokio loop.

---

## 2. Tauri v2 Shell Architecture (`src-tauri/`)

### 2.1 Directory Structure

```text
src-tauri/
├── Cargo.toml                  # Depends on tauri v2, lms-sync, lms-grader, security-framework
├── tauri.conf.json             # macOS bundle ID com.vedam.lms, Overlay titlebar, 1280x840 min window
├── capabilities/
│   └── default.json            # Least-privilege ACLs (notifications, dialog, opener, core events)
├── icons/                      # Generated macOS .icns from Vedam brand mark
└── src/
    ├── main.rs                 # Desktop entrypoint
    ├── lib.rs                  # Window setup, Native macOS Menu, Background Sync + SSE supervisor
    ├── keychain.rs             # macOS Security.framework bearer token storage
    └── commands.rs             # Typed #[tauri::command] handlers exposed to React
```

### 2.2 `src-tauri/Cargo.toml`

```toml
[package]
name = "vedam-lms-desktop"
version = "0.1.0"
edition = "2024"

[build-dependencies]
tauri-build = { version = "2.0", features = [] }

[dependencies]
lms-grader = { path = "../crates/lms-grader" }
lms-sync = { path = "../crates/lms-sync" }
security-framework = "3.0"
serde = { version = "1.0", features = ["derive"] }
serde_json = "1.0"
tauri = { version = "2.1", features = ["macos-private-api"] }
tauri-plugin-dialog = "2.0"
tauri-plugin-notification = "2.0"
tauri-plugin-opener = "2.0"
tokio = { version = "1.41", features = ["full"] }
uuid = { version = "1.11", features = ["v7"] }
```

### 2.3 `src-tauri/tauri.conf.json`

```json
{
  "$schema": "https://schema.tauri.app/config/2",
  "productName": "Vedam LMS",
  "version": "0.1.0",
  "identifier": "com.vedam.lms",
  "build": {
    "beforeDevCommand": "npm run dev:web",
    "devUrl": "http://127.0.0.1:5173",
    "beforeBuildCommand": "npm run build",
    "frontendDist": "../dist"
  },
  "app": {
    "macOSPrivateApi": true,
    "windows": [
      {
        "title": "Vedam LMS",
        "width": 1360,
        "height": 880,
        "minWidth": 1024,
        "minHeight": 700,
        "titleBarStyle": "Overlay",
        "hiddenTitle": true,
        "trafficLightPosition": { "x": 18, "y": 20 }
      }
    ],
    "security": {
      "csp": "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; connect-src 'self' http://127.0.0.1:3001 https://*.vedam.local"
    }
  },
  "bundle": {
    "active": true,
    "targets": ["app", "dmg"],
    "category": "Education",
    "macOS": {
      "minimumSystemVersion": "13.0",
      "entitlements": null
    }
  }
}
```

### 2.4 Least-Privilege Capability ACL (`src-tauri/capabilities/default.json`)

```json
{
  "$schema": "../gen/schemas/desktop-schema.json",
  "identifier": "default",
  "description": "Least-privilege desktop capabilities for Vedam LMS",
  "windows": ["main"],
  "permissions": [
    "core:default",
    "core:event:default",
    "notification:default",
    "dialog:allow-save",
    "opener:allow-open-url"
  ]
}
```

---

## 3. Native macOS Capabilities & Command Bridge (`src-tauri/src/`)

### 3.1 Keychain Token Vault (`keychain.rs`)

Session bearer tokens returned by `/api/login` are never stored in `localStorage` or plaintext files; they live inside macOS Keychain under service `"com.vedam.lms"`:

```rust
use security_framework::passwords::{delete_generic_password, get_generic_password, set_generic_password};

const SERVICE: &str = "com.vedam.lms";
const ACCOUNT: &str = "active_session_token";

pub fn save_session_token(token: &str) -> Result<(), String> {
    set_generic_password(SERVICE, ACCOUNT, token.as_bytes()).map_err(|e| e.to_string())
}

pub fn load_session_token() -> Option<String> {
    get_generic_password(SERVICE, ACCOUNT)
        .ok()
        .and_then(|bytes| String::from_utf8(bytes).ok())
}

pub fn clear_session_token() {
    let _ = delete_generic_password(SERVICE, ACCOUNT);
}
```

### 3.2 Typed `#[tauri::command]` Surface (`commands.rs`)

| Command | Rust Delegate | Responsibility |
| :--- | :--- | :--- |
| `lms_request(path, method, body)` | `lms-sync` + `reqwest` | Serves reads from SQLite WAL (`GET /api/bootstrap`, `/api/discussions`), enqueues offline mutations into `sync_outbox` (`POST /api/progress`, `PUT /api/profile`, `POST /api/discussions`), or proxies strict online actions (`/api/login`, `/api/payments`, `/api/staff/*`) with Keychain Bearer auth |
| `lms_sync_now()` | `SyncEngine::sync_once()` | Immediately drains `sync_outbox` and pulls remote delta cursor; returns `SyncStatusSnapshot` |
| `lms_sync_status()` | `LocalStorePort::status_snapshot()` | Reads pending outbox count, conflict count, and last sync timestamp |
| `lms_run_code_local(req)` | `lms-grader::JudgePipeline` | Runs example test cases or custom input locally inside `sandbox-exec` even when offline on a plane or train |
| `lms_save_code_draft(id, lang, src)` | `local_code_drafts` SQLite | Auto-saves CodeMirror editor buffer every 500 ms without hitting the network |
| `lms_export_asset(url, filename)` | `tauri_plugin_dialog` + cache | Saves receipts, lecture notes, or PDFs using the native macOS NSSavePanel sheet |

### 3.3 Native macOS Polish in React & Rust

1. **Overlay Titlebar & Drag Region** ([`src/components/layout/TopHeader.tsx`](file:///Users/sirwagyashekhar/Projects/LMS/src/components/layout/TopHeader.tsx)):
   - Add a `data-tauri-drag-region` top inset (`padding-left: 76px` only when `data-platform="macos-tauri"` is set on `<html>`) so macOS red/yellow/green traffic lights sit cleanly inline without overlapping the **Vedam** wordmark.
2. **Header Sync Status Pill**:
   - Minimalist status indicator beside the theme toggle displaying `Synced`, `Syncing…`, `Offline (N queued)`, or `Conflict` (clicking opens conflict review or triggers `lms_sync_now()`).
3. **Native macOS Menu Bar & Keyboard Shortcuts**:
   - `Cmd+1`..`Cmd+5` switch workspace tabs (`Dashboard`, `Overview`, `Lectures`, `Schedule`, `Account`).
   - `Cmd+R` / `Cmd+Shift+S` triggers manual background sync (`lms_sync_now`).
   - `Cmd+,` navigates directly to `#/account`.
4. **Native macOS Notifications**:
   - When SSE (`grade.updated` or `notice.published`) or `SyncEngine` pull discovers a newly graded assignment or `URGENT` notice, invoke `tauri-plugin-notification` to post a native macOS Notification Center alert with deep-link focus on click.

---

## 4. Phase 5 Deliverables & Verification Gate

- **Deliverables**:
  - `src/portal/transport.ts` & refactored `src/portal/api.ts`
  - `src/components/layout/TopHeader.tsx` (conditional macOS traffic-light padding & sync indicator)
  - `src-tauri/` (`Cargo.toml`, `tauri.conf.json`, `capabilities/default.json`, `src/{main,lib,keychain,commands}.rs`)
  - `npm` scripts: `"desktop:dev": "tauri dev"`, `"desktop:build": "tauri build"`
- **Verification Matrix**:
  1. **Web Build Parity**: `npm run lint && npm run build` passes cleanly and Web browser mode uses `HttpTransport` with zero Tauri runtime dependencies loaded.
  2. **Rust Bridge Tests**: `cargo test -p vedam-lms-desktop` verifies Keychain round-trip (mocked/test service name) and IPC serialization.
  3. **macOS Offline E2E**:
     - Launch `npm run desktop:dev`, sign in as `student@vedam.local`.
     - Disconnect network, toggle lecture completion, edit coding handle in Account, and run Python/Java in `lms-grader`.
     - Verify local SQLite retains state across app restart (`Cmd+Q` -> reopen).
     - Reconnect network and verify `sync_outbox` automatically drains to Go + PostgreSQL without duplicate records.
  4. **Native Bundle Build**:
     ```bash
     npm run desktop:build
     ls -lh src-tauri/target/release/bundle/macos/Vedam\ LMS.app
     ls -lh src-tauri/target/release/bundle/dmg/
     ```
