import { invoke } from "@tauri-apps/api/core";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";

export type SyncLifecycleState = "synced" | "syncing" | "offline" | "error";

export interface SyncStatus {
  state: SyncLifecycleState;
  pendingCount: number;
  conflictCount: number;
  lastSyncedAt: string | null;
  cursor?: number;
}

export interface TransportPort {
  readonly isDesktop: boolean;
  request<T = { ok: boolean }>(
    path: string,
    method?: string,
    data?: unknown,
  ): Promise<T>;
  subscribeEvents(
    onEvent: (eventName: string, payload: unknown) => void,
  ): () => void;
  triggerSyncNow(): Promise<SyncStatus>;
  getSyncStatus(): Promise<SyncStatus>;
  downloadAsset(url: string, filename?: string): Promise<void>;
  saveCodeDraft(
    userId: string,
    assignmentId: string,
    language: string,
    entrypoint: string,
    source: string,
  ): Promise<void>;
  loadCodeDraft(
    userId: string,
    assignmentId: string,
    language: string,
    entrypoint: string,
  ): Promise<string | null>;
}

export function isTauriRuntime(): boolean {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
}

function draftStorageKey(
  userId: string,
  assignmentId: string,
  language: string,
  entrypoint: string,
): string {
  return `vedam-code:${userId}:${assignmentId}:${language}:${entrypoint || "stdin"}`;
}

function readLocalDraft(key: string): string | null {
  try {
    return typeof localStorage === "undefined"
      ? null
      : localStorage.getItem(key);
  } catch {
    return null;
  }
}

function writeLocalDraft(key: string, source: string): void {
  try {
    if (typeof localStorage !== "undefined") {
      localStorage.setItem(key, source);
    }
  } catch {
    /* Draft remains in memory if storage quota is full. */
  }
}

function extractErrorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  if (typeof err === "string" && err) return err;
  if (
    typeof err === "object" &&
    err !== null &&
    "message" in err &&
    typeof (err as { message?: unknown }).message === "string"
  ) {
    return (err as { message: string }).message;
  }
  return "Request failed. Please try again.";
}

export class HttpTransport implements TransportPort {
  readonly isDesktop = false;

  async request<T = { ok: boolean }>(
    path: string,
    method = "GET",
    data?: unknown,
  ): Promise<T> {
    const response = await fetch(`/api${path}`, {
      method,
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: data === undefined ? undefined : JSON.stringify(data),
    });
    const result = await response.json();
    if (
      response.status === 401 &&
      path !== "/login" &&
      path !== "/register" &&
      typeof window !== "undefined"
    ) {
      window.dispatchEvent(new Event("lms:session-expired"));
    }
    if (!response.ok) {
      throw new Error(result.error || "Request failed. Please try again.");
    }
    return result as T;
  }

  subscribeEvents(
    onEvent: (eventName: string, payload: unknown) => void,
  ): () => void {
    if (typeof window === "undefined") return () => {};

    const emitConnectivity = () => {
      onEvent("sync-updated", {
        state: navigator.onLine ? "synced" : "offline",
        pendingCount: 0,
        conflictCount: 0,
        lastSyncedAt: null,
      } satisfies SyncStatus);
    };
    window.addEventListener("online", emitConnectivity);
    window.addEventListener("offline", emitConnectivity);

    let source: EventSource | null = null;
    if (typeof EventSource !== "undefined") {
      source = new EventSource("/api/v1/events", { withCredentials: true });
      const forward = (eventName: string) => (ev: MessageEvent<string>) => {
        let parsed: unknown = ev.data;
        try {
          parsed = ev.data ? JSON.parse(ev.data) : null;
        } catch {
          /* Keep raw string payload if not JSON. */
        }
        onEvent(eventName, parsed);
      };
      source.addEventListener("sync.available", forward("sync.available"));
      source.addEventListener("grade.updated", forward("grade.updated"));
      source.addEventListener("notice.published", forward("notice.published"));
    }

    return () => {
      window.removeEventListener("online", emitConnectivity);
      window.removeEventListener("offline", emitConnectivity);
      source?.close();
    };
  }

  async getSyncStatus(): Promise<SyncStatus> {
    const online = typeof navigator === "undefined" ? true : navigator.onLine;
    return {
      state: online ? "synced" : "offline",
      pendingCount: 0,
      conflictCount: 0,
      lastSyncedAt: null,
    };
  }

  async triggerSyncNow(): Promise<SyncStatus> {
    return this.getSyncStatus();
  }

  async downloadAsset(url: string, filename?: string): Promise<void> {
    if (typeof document === "undefined") return;
    const anchor = document.createElement("a");
    anchor.href = url;
    if (filename) anchor.download = filename;
    anchor.style.display = "none";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  }

  async saveCodeDraft(
    userId: string,
    assignmentId: string,
    language: string,
    entrypoint: string,
    source: string,
  ): Promise<void> {
    writeLocalDraft(
      draftStorageKey(userId, assignmentId, language, entrypoint),
      source,
    );
  }

  async loadCodeDraft(
    userId: string,
    assignmentId: string,
    language: string,
    entrypoint: string,
  ): Promise<string | null> {
    return readLocalDraft(
      draftStorageKey(userId, assignmentId, language, entrypoint),
    );
  }
}

export class TauriOfflineTransport implements TransportPort {
  readonly isDesktop = true;

  constructor() {
    if (typeof document !== "undefined") {
      document.documentElement.dataset.platform = "macos-tauri";
    }
  }

  async request<T = { ok: boolean }>(
    path: string,
    method = "GET",
    data?: unknown,
  ): Promise<T> {
    try {
      return await invoke<T>("lms_request", {
        path,
        method,
        body: data === undefined ? null : data,
      });
    } catch (err) {
      const message = extractErrorMessage(err);
      if (
        message === "Please sign in." &&
        path !== "/login" &&
        path !== "/register" &&
        typeof window !== "undefined"
      ) {
        window.dispatchEvent(new Event("lms:session-expired"));
      }
      throw new Error(message);
    }
  }

  subscribeEvents(
    onEvent: (eventName: string, payload: unknown) => void,
  ): () => void {
    let active = true;
    const unlisteners: UnlistenFn[] = [];

    const register = async () => {
      const handlers = await Promise.all([
        listen<SyncStatus>("lms://sync-updated", (event) => {
          onEvent("sync-updated", event.payload);
        }),
        listen<{ name: string; payload: unknown }>("lms://event", (event) => {
          if (event.payload?.name) {
            onEvent(event.payload.name, event.payload.payload);
          }
        }),
        listen<{ hash?: string }>("lms://navigate", (event) => {
          const hash = event.payload?.hash;
          if (hash && typeof window !== "undefined") {
            window.location.hash = hash.startsWith("#") ? hash : `#${hash}`;
          }
        }),
      ]);
      if (!active) {
        handlers.forEach((fn) => fn());
        return;
      }
      unlisteners.push(...handlers);
    };

    void register();

    return () => {
      active = false;
      unlisteners.forEach((fn) => fn());
    };
  }

  async triggerSyncNow(): Promise<SyncStatus> {
    return invoke<SyncStatus>("lms_sync_now");
  }

  async getSyncStatus(): Promise<SyncStatus> {
    return invoke<SyncStatus>("lms_sync_status");
  }

  async downloadAsset(url: string, filename?: string): Promise<void> {
    await invoke("lms_export_asset", {
      url,
      filename: filename ?? null,
    });
  }

  async saveCodeDraft(
    userId: string,
    assignmentId: string,
    language: string,
    entrypoint: string,
    source: string,
  ): Promise<void> {
    writeLocalDraft(
      draftStorageKey(userId, assignmentId, language, entrypoint),
      source,
    );
    await invoke("lms_save_code_draft", {
      assignmentId,
      language,
      source,
    });
  }

  async loadCodeDraft(
    userId: string,
    assignmentId: string,
    language: string,
    entrypoint: string,
  ): Promise<string | null> {
    try {
      const stored = await invoke<string | null>("lms_load_code_draft", {
        assignmentId,
        language,
      });
      if (stored !== null) return stored;
    } catch {
      /* Fall back to browser cache if desktop session isn't hydrated yet. */
    }
    return readLocalDraft(
      draftStorageKey(userId, assignmentId, language, entrypoint),
    );
  }
}

export function createTransport(): TransportPort {
  return isTauriRuntime() ? new TauriOfflineTransport() : new HttpTransport();
}
