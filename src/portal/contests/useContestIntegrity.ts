import { useCallback, useEffect, useRef, useState, type ClipboardEvent } from "react";
import { api, transport } from "../api";
import type { Language } from "../coding/types";

export type IntegrityEventType =
  | "nav.question"
  | "mcq.selected"
  | "code.snapshot"
  | "code.delta"
  | "editor.paste"
  | "window.focus"
  | "window.blur"
  | "fullscreen.enter"
  | "fullscreen.exit"
  | "heartbeat.gap"
  | "client.reconnect"
  | "app.opened"
  | "proctor.camera_lost"
  | "proctor.mic_lost"
  | "proctor.screen_lost"
  | "lockdown.warning";

export type IntegrityEvent = {
  seq: number;
  eventType: IntegrityEventType;
  clientAt: string;
  questionIndex?: number;
  payload: Record<string, unknown>;
};

export type UseContestIntegrityOptions = {
  contestId: string;
  userId: string;
  attemptId?: string;
  enabled: boolean;
  selectedQuestion: number;
  reviewMode: boolean;
  attemptError?: string;
  onLockdownStatus?: (warnings: number, submitted: boolean) => void;
};

const MAX_CODE_PREVIEW = 4000;
const MAX_BATCH_SIZE = 25;
const MAX_QUEUE_SIZE = 500;
const FLUSH_INTERVAL_MS = 5000;
const HEARTBEAT_CHECK_MS = 2000;
const HEARTBEAT_GAP_MS = 15_000;
const CODE_DEBOUNCE_MS = 1200;
const CODE_PERIODIC_MS = 15_000;

function localRead(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function localWrite(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* Ignore storage quota failures; seq stays in memory for this session. */
  }
}

function readDeviceId(): string {
  if (!transport.isDesktop) return "";
  const key = "lms:contest-device";
  const existing = localRead(key);
  if (existing) return existing;
  const next = crypto.randomUUID();
  localWrite(key, next);
  return next;
}

function seqStorageKey(userId: string, attemptId: string): string {
  return `vedam-contest-seq:${userId}:${attemptId}`;
}

function lockdownStorageKey(userId: string, attemptId: string, deviceId: string): string {
  return `vedam-lockdown-warnings:${userId}:${attemptId}:${deviceId}`;
}

function readLockdownWarnings(key: string): IntegrityEvent[] {
  try {
    const events = JSON.parse(localRead(key) || "[]") as IntegrityEvent[];
    return Array.isArray(events) ? events.filter((event) => event.eventType === "lockdown.warning").slice(0, 3) : [];
  } catch {
    return [];
  }
}

function readPersistedSeq(userId: string, attemptId: string): number {
  const raw = localRead(seqStorageKey(userId, attemptId));
  if (!raw) return 0;
  const parsed = Number.parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
}

function isFullscreenApiSupported(): boolean {
  if (typeof document === "undefined") return false;
  const root = document.documentElement as HTMLElement & {
    requestFullscreen?: () => Promise<void>;
  };
  return typeof root?.requestFullscreen === "function" && document.fullscreenEnabled !== false;
}

type CodeTrackState = {
  language: Language;
  source: string;
  lastEmittedSource: string;
  lastEmittedAt: number;
  editsSinceSnapshot: number;
  timer: number | null;
};

export function useContestIntegrity({
  contestId,
  userId,
  attemptId,
  enabled,
  selectedQuestion,
  reviewMode,
  attemptError = "",
  onLockdownStatus,
}: UseContestIntegrityOptions) {
  const [deviceId] = useState(readDeviceId);
  const [isFullscreen, setIsFullscreen] = useState<boolean>(() =>
    typeof document !== "undefined" ? Boolean(document.fullscreenElement) : false,
  );
  const fullscreenSupported = isFullscreenApiSupported();

  const seqRef = useRef(0);
  const queueRef = useRef<IntegrityEvent[]>([]);
  const pendingBatchRef = useRef<{ batchId: string; events: IntegrityEvent[] } | null>(null);
  const flushingRef = useRef(false);
  const retryCountRef = useRef(0);
  const retryTimerRef = useRef<number | null>(null);
  const codeTracksRef = useRef(new Map<number, CodeTrackState>());
  const lastNavRef = useRef<string | null>(null);
  const lastErrorRef = useRef(attemptError);
  const lastHeartbeatRef = useRef(0);
  const sessionRef = useRef<{ contestId: string; userId: string; attemptId: string; deviceId: string } | null>(null);
  const flushBatchRef = useRef<() => Promise<void>>(async () => {});

  const lockdownStatusRef = useRef(onLockdownStatus);
  useEffect(() => { lockdownStatusRef.current = onLockdownStatus; }, [onLockdownStatus]);

  const clearRetryTimer = useCallback(() => {
    if (retryTimerRef.current !== null) {
      window.clearTimeout(retryTimerRef.current);
      retryTimerRef.current = null;
    }
  }, []);

  const flushBatch = useCallback(async () => {
    const active = sessionRef.current;
    if (!active || flushingRef.current) return;
    if (!pendingBatchRef.current && queueRef.current.length === 0) return;

    clearRetryTimer();
    flushingRef.current = true;
    try {
      if (!pendingBatchRef.current) {
        const firstOptional = queueRef.current.findIndex(({ eventType }) => eventType !== "lockdown.warning");
        const firstWarning = queueRef.current[0]?.eventType === "lockdown.warning";
        const size = firstWarning && firstOptional > 0 ? firstOptional : MAX_BATCH_SIZE;
        const events = queueRef.current.splice(0, size);
        pendingBatchRef.current = {
          batchId: crypto.randomUUID(),
          events,
        };
      }
      const batch = pendingBatchRef.current;
      const result = await api<{ lockdownWarnings?: number; lockdownSubmitted?: boolean }>(
        `/contests/${active.contestId}/events`, "POST", {
        attemptId: active.attemptId,
        deviceId: active.deviceId,
        batchId: batch.batchId,
        events: batch.events,
      });
      if (batch.events.some((event) => event.eventType === "lockdown.warning")) {
        const key = lockdownStorageKey(active.userId, active.attemptId, active.deviceId);
        const acknowledged = new Set(batch.events.map(({ seq }) => seq));
        localWrite(key, JSON.stringify(readLockdownWarnings(key).filter(({ seq }) => !acknowledged.has(seq))));
      }
      if (sessionRef.current !== active) return;
      lockdownStatusRef.current?.(result.lockdownWarnings ?? 0, result.lockdownSubmitted ?? false);
      pendingBatchRef.current = null;
      retryCountRef.current = 0;

      if (queueRef.current.length > 0) {
        window.setTimeout(() => {
          void flushBatchRef.current();
        }, 50);
      }
    } catch (error) {
      if (sessionRef.current !== active) return;
      if (error instanceof Error && error.message === "Client event quota exceeded for this attempt." &&
          pendingBatchRef.current?.events.every(({ eventType }) => eventType !== "lockdown.warning")) {
        pendingBatchRef.current = null;
        window.setTimeout(() => void flushBatchRef.current(), 50);
        return;
      }
      retryCountRef.current += 1;
      const delay = Math.min(30_000, 2000 * Math.pow(2, Math.min(4, retryCountRef.current - 1)));
      retryTimerRef.current = window.setTimeout(() => {
        retryTimerRef.current = null;
        void flushBatchRef.current();
      }, delay);
    } finally {
      flushingRef.current = false;
    }
  }, [clearRetryTimer]);

  useEffect(() => {
    flushBatchRef.current = flushBatch;
  }, [flushBatch]);

  const enqueue = useCallback(
    (
      type: IntegrityEventType,
      payload: Record<string, unknown> = {},
      questionIndex?: number,
    ) => {
      const active = sessionRef.current;
      if (!active) return;

      const nextSeq = seqRef.current + 1;
      seqRef.current = nextSeq;
      localWrite(seqStorageKey(active.userId, active.attemptId), String(nextSeq));

      const event: IntegrityEvent = {
        seq: nextSeq,
        eventType: type,
        clientAt: new Date().toISOString(),
        ...(questionIndex !== undefined ? { questionIndex } : {}),
        payload,
      };
      if (queueRef.current.length >= MAX_QUEUE_SIZE) {
        const firstOptional = queueRef.current.findIndex((queued) => queued.eventType !== "lockdown.warning");
        if (firstOptional >= 0) queueRef.current.splice(firstOptional, 1);
      }
      if (type === "lockdown.warning") {
        const key = lockdownStorageKey(active.userId, active.attemptId, active.deviceId);
        localWrite(key, JSON.stringify([...readLockdownWarnings(key), event].slice(0, 3)));
        const firstOptional = queueRef.current.findIndex((queued) => queued.eventType !== "lockdown.warning");
        if (firstOptional < 0) queueRef.current.push(event);
        else queueRef.current.splice(firstOptional, 0, event);
      } else {
        queueRef.current.push(event);
      }

      if (queueRef.current.length >= 10) {
        void flushBatch();
      }
    },
    [flushBatch],
  );

  const flushCodeTrack = useCallback(
    (questionIndex: number) => {
      const track = codeTracksRef.current.get(questionIndex);
      if (!track) return;
      if (track.timer !== null) {
        window.clearTimeout(track.timer);
        track.timer = null;
      }
      if (track.source === track.lastEmittedSource) return;

      const charCount = track.source.length;
      const deltaChars = charCount - track.lastEmittedSource.length;
      const shouldSnapshot =
        track.lastEmittedAt === 0 ||
        track.editsSinceSnapshot >= 5 ||
        Math.abs(deltaChars) >= 200;

      const preview = track.source.slice(0, MAX_CODE_PREVIEW);
      const basePayload: Record<string, unknown> = {
        questionIndex,
        language: track.language,
        charCount,
        preview,
        truncated: charCount > MAX_CODE_PREVIEW,
      };

      if (shouldSnapshot) {
        enqueue("code.snapshot", basePayload, questionIndex);
        track.editsSinceSnapshot = 0;
      } else {
        enqueue(
          "code.delta",
          {
            ...basePayload,
            deltaChars,
          },
          questionIndex,
        );
        track.editsSinceSnapshot += 1;
      }
      track.lastEmittedSource = track.source;
      track.lastEmittedAt = Date.now();
    },
    [enqueue],
  );

  useEffect(() => {
    if (!enabled || !attemptId) {
      sessionRef.current = null;
      lastNavRef.current = null;
      return;
    }

    sessionRef.current = { contestId, userId, attemptId, deviceId };
    const queuedWarnings = readLockdownWarnings(lockdownStorageKey(userId, attemptId, deviceId));
    seqRef.current = Math.max(readPersistedSeq(userId, attemptId), ...queuedWarnings.map(({ seq }) => seq));
    queueRef.current = queuedWarnings;
    pendingBatchRef.current = null;
    retryCountRef.current = 0;
    lastHeartbeatRef.current = Date.now();
    lastNavRef.current = null;
    const tracks = codeTracksRef.current;
    if (queuedWarnings.length) void flushBatch();

    const flushTimer = window.setInterval(() => {
      void flushBatch();
    }, FLUSH_INTERVAL_MS);

    const heartbeatTimer = window.setInterval(() => {
      const now = Date.now();
      const gapMs = now - lastHeartbeatRef.current;
      lastHeartbeatRef.current = now;
      if (gapMs > HEARTBEAT_GAP_MS) {
        enqueue("heartbeat.gap", {
          gapMs,
          gapSeconds: Math.round(gapMs / 1000),
        });
      }
    }, HEARTBEAT_CHECK_MS);

    const onWindowFocus = () => enqueue("window.focus");
    const onWindowBlur = () => enqueue("window.blur");
    const onFullscreenChange = () => {
      const active = Boolean(document.fullscreenElement);
      setIsFullscreen(active);
      enqueue(active ? "fullscreen.enter" : "fullscreen.exit");
    };
    const onVisibilityChange = () => {
      if (document.visibilityState === "hidden") {
        for (const qIndex of tracks.keys()) {
          flushCodeTrack(qIndex);
        }
        void flushBatch();
      } else if (document.visibilityState === "visible") {
        const now = Date.now();
        const gapMs = now - lastHeartbeatRef.current;
        lastHeartbeatRef.current = now;
        if (gapMs > HEARTBEAT_GAP_MS) {
          enqueue("heartbeat.gap", {
            gapMs,
            gapSeconds: Math.round(gapMs / 1000),
            source: "visibility",
          });
        }
      }
    };
    const onPageHide = () => {
      for (const qIndex of tracks.keys()) {
        flushCodeTrack(qIndex);
      }
      void flushBatch();
    };
    const onOnline = () => {
      enqueue("client.reconnect", { trigger: "online" });
      void flushBatch();
    };

    window.addEventListener("focus", onWindowFocus);
    window.addEventListener("blur", onWindowBlur);
    window.addEventListener("online", onOnline);
    window.addEventListener("pagehide", onPageHide);
    document.addEventListener("fullscreenchange", onFullscreenChange);
    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      window.clearInterval(flushTimer);
      window.clearInterval(heartbeatTimer);
      clearRetryTimer();
      window.removeEventListener("focus", onWindowFocus);
      window.removeEventListener("blur", onWindowBlur);
      window.removeEventListener("online", onOnline);
      window.removeEventListener("pagehide", onPageHide);
      document.removeEventListener("fullscreenchange", onFullscreenChange);
      document.removeEventListener("visibilitychange", onVisibilityChange);

      for (const [qIndex, track] of tracks.entries()) {
        if (track.timer !== null) window.clearTimeout(track.timer);
        if (track.source !== track.lastEmittedSource) {
          flushCodeTrack(qIndex);
        }
      }
      tracks.clear();
      void flushBatch();
      sessionRef.current = null;
    };
  }, [attemptId, clearRetryTimer, contestId, deviceId, enabled, enqueue, flushBatch, flushCodeTrack, userId]);

  useEffect(() => {
    if (!enabled || !attemptId) return;
    const key = `${selectedQuestion}:${reviewMode ? "review" : "question"}`;
    if (lastNavRef.current === key) return;
    lastNavRef.current = key;
    enqueue(
      "nav.question",
      { questionIndex: selectedQuestion, review: reviewMode },
      selectedQuestion,
    );
  }, [attemptId, enabled, enqueue, reviewMode, selectedQuestion]);

  useEffect(() => {
    const previous = lastErrorRef.current;
    lastErrorRef.current = attemptError;
    if (enabled && attemptId && previous && !attemptError) {
      enqueue("client.reconnect", { trigger: "recovery" });
      void flushBatch();
    }
  }, [attemptError, attemptId, enabled, enqueue, flushBatch]);

  const recordMcqSelect = useCallback(
    (questionIndex: number, choice: number) => {
      if (!enabled || !attemptId) return;
      enqueue("mcq.selected", { questionIndex, choice }, questionIndex);
    },
    [attemptId, enabled, enqueue],
  );

  const recordCodeChange = useCallback(
    (questionIndex: number, language: Language, source: string) => {
      if (!enabled || !attemptId) return;
      const existing = codeTracksRef.current.get(questionIndex);
      if (!existing) {
        const track: CodeTrackState = {
          language,
          source,
          lastEmittedSource: "",
          lastEmittedAt: 0,
          editsSinceSnapshot: 0,
          timer: null,
        };
        track.timer = window.setTimeout(() => flushCodeTrack(questionIndex), CODE_DEBOUNCE_MS);
        codeTracksRef.current.set(questionIndex, track);
        return;
      }

      existing.language = language;
      existing.source = source;
      if (existing.timer !== null) {
        window.clearTimeout(existing.timer);
      }
      const elapsed = Date.now() - existing.lastEmittedAt;
      if (elapsed >= CODE_PERIODIC_MS) {
        flushCodeTrack(questionIndex);
      } else {
        existing.timer = window.setTimeout(() => flushCodeTrack(questionIndex), CODE_DEBOUNCE_MS);
      }
    },
    [attemptId, enabled, flushCodeTrack],
  );

  const recordPaste = useCallback(
    (questionIndex: number, charCount: number) => {
      if (!enabled || !attemptId) return;
      // Only paste length is ever recorded; clipboard text stays outside collection.
      enqueue("editor.paste", { questionIndex, charCount }, questionIndex);
    },
    [attemptId, enabled, enqueue],
  );

  const recordProctorEvent = useCallback(
    (eventType: IntegrityEventType, payload: Record<string, unknown> = {}) => {
      if (!enabled || !attemptId) return;
      enqueue(eventType, payload);
      void flushBatch();
    },
    [attemptId, enabled, enqueue, flushBatch],
  );

  const handleEditorPasteCapture = useCallback(
    (questionIndex: number) => (event: ClipboardEvent<HTMLElement>) => {
      if (!enabled || !attemptId) return;
      const text =
        event.clipboardData?.getData("text/plain") ??
        event.clipboardData?.getData("text") ??
        "";
      recordPaste(questionIndex, text.length);
    },
    [attemptId, enabled, recordPaste],
  );

  const enterFullscreen = useCallback(async () => {
    if (!isFullscreenApiSupported()) return;
    try {
      await document.documentElement.requestFullscreen();
    } catch {
      /* Gracefully no-op when fullscreen is blocked by browser or headless runtime. */
    }
  }, []);

  const exitFullscreen = useCallback(async () => {
    if (typeof document === "undefined" || !document.fullscreenElement) return;
    try {
      await document.exitFullscreen();
    } catch {
      /* Ignore fullscreen exit rejections. */
    }
  }, []);

  return {
    isFullscreen,
    fullscreenSupported,
    enterFullscreen,
    exitFullscreen,
    recordMcqSelect,
    recordCodeChange,
    recordPaste,
    recordProctorEvent,
    handleEditorPasteCapture,
  };
}
