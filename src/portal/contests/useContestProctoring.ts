import { useCallback, useEffect, useRef, useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import { api, transport } from "../api";
import type { IntegrityEventType } from "./useContestIntegrity";

export type ProctorApp = {
  name: string;
  bundleId: string;
  pid: number;
};

export type UseContestProctoringOptions = {
  contestId: string;
  attemptId?: string;
  enabled: boolean;
  active: boolean;
  ready: boolean;
  onProctorEvent?: (
    eventType: IntegrityEventType,
    payload?: Record<string, unknown>,
  ) => void;
};

const DEVICE_STORAGE_KEY = "lms:contest-device";
const CAMERA_CHUNK_MS = 10_000;
const SCREEN_SNAPSHOT_MS = 10_000;
const WATCHDOG_POLL_MS = 5_000;

const CANDIDATE_MIME_TYPES = [
  "video/webm;codecs=vp8,opus",
  "video/webm;codecs=vp9,opus",
  "video/webm",
  "video/mp4",
];

function getDeviceId(): string {
  if (!transport.isDesktop) return "";
  try {
    const existing = localStorage.getItem(DEVICE_STORAGE_KEY);
    if (existing) return existing;
    const next = crypto.randomUUID();
    localStorage.setItem(DEVICE_STORAGE_KEY, next);
    return next;
  } catch {
    return "";
  }
}

function extractError(err: unknown, fallback: string): string {
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
  return fallback;
}

function normalizeApps(raw: unknown): ProctorApp[] {
  if (!Array.isArray(raw)) return [];
  const apps: ProctorApp[] = [];
  for (const item of raw) {
    if (!item || typeof item !== "object") continue;
    const record = item as Record<string, unknown>;
    const name = typeof record.name === "string" ? record.name : "Unknown application";
    const bundleId =
      typeof record.bundleId === "string"
        ? record.bundleId
        : typeof record.bundle_id === "string"
          ? record.bundle_id
          : "";
    const pid = typeof record.pid === "number" ? record.pid : 0;
    apps.push({ name, bundleId, pid });
  }
  return apps;
}

function extractScreenCapture(raw: unknown): { dataBase64: string; contentType: string } {
  if (typeof raw === "string") {
    return { dataBase64: raw, contentType: "image/png" };
  }
  if (raw && typeof raw === "object") {
    const record = raw as Record<string, unknown>;
    const dataBase64 =
      (typeof record.dataBase64 === "string" && record.dataBase64) ||
      (typeof record.data_base64 === "string" && record.data_base64) ||
      (typeof record.base64 === "string" && record.base64) ||
      (typeof record.data === "string" && record.data) ||
      "";
    const contentType =
      (typeof record.contentType === "string" && record.contentType) ||
      (typeof record.content_type === "string" && record.content_type) ||
      (typeof record.mimeType === "string" && record.mimeType) ||
      "image/png";
    return { dataBase64, contentType };
  }
  return { dataBase64: "", contentType: "image/png" };
}

function pickRecorderMimeType(): string | undefined {
  if (
    typeof MediaRecorder === "undefined" ||
    typeof MediaRecorder.isTypeSupported !== "function"
  ) {
    return undefined;
  }
  return CANDIDATE_MIME_TYPES.find((mime) => MediaRecorder.isTypeSupported(mime));
}

async function blobToBase64(blob: Blob): Promise<string> {
  const bytes = new Uint8Array(await blob.arrayBuffer());
  if (bytes.byteLength === 0) return "";
  const chunkSize = 0x2000;
  const parts: string[] = [];
  for (let i = 0; i < bytes.byteLength; i += chunkSize) {
    const slice = bytes.subarray(i, Math.min(i + chunkSize, bytes.byteLength));
    parts.push(String.fromCharCode(...slice));
  }
  return btoa(parts.join(""));
}

function releaseMediaStream(
  stream: MediaStream | null,
  cleanups: Array<() => void>,
): void {
  while (cleanups.length > 0) {
    const fn = cleanups.pop();
    fn?.();
  }
  if (stream) {
    for (const track of stream.getTracks()) {
      track.stop();
    }
  }
}

/**
 * Coordinates macOS desktop pre-contest lockdown checks, live camera/mic + screen
 * recording, and mid-contest watchdog enforcement when integrity is enabled.
 */
export function useContestProctoring({
  contestId,
  attemptId,
  enabled,
  active,
  ready,
  onProctorEvent,
}: UseContestProctoringOptions) {
  const desktop = transport.isDesktop;
  const isProctored = desktop && enabled;

  const [openApps, setOpenApps] = useState<ProctorApp[]>([]);
  const [appsBusy, setAppsBusy] = useState(false);
  const [appsError, setAppsError] = useState("");

  const [cameraReady, setCameraReady] = useState(false);
  const [micReady, setMicReady] = useState(false);
  const [mediaError, setMediaError] = useState("");
  const [mediaStream, setMediaStream] = useState<MediaStream | null>(null);

  const [screenReady, setScreenReady] = useState(false);
  const [screenError, setScreenError] = useState("");

  const [isFullscreen, setIsFullscreen] = useState(false);
  const [fullscreenBusy, setFullscreenBusy] = useState(false);

  const [cameraChunksUploaded, setCameraChunksUploaded] = useState(0);
  const [screenShotsUploaded, setScreenShotsUploaded] = useState(0);
  const [lastUploadError, setLastUploadError] = useState("");

  const mountedRef = useRef(true);
  const activeRef = useRef(active);
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const trackCleanupsRef = useRef<Array<() => void>>([]);
  const recorderRef = useRef<MediaRecorder | null>(null);
  const isFullscreenRef = useRef(isFullscreen);
  const screenReadyRef = useRef(screenReady);
  const onProctorEventRef = useRef(onProctorEvent);
  const cameraSeqRef = useRef(0);
  const screenSeqRef = useRef(0);
  const seqAttemptIdRef = useRef<string | undefined>(undefined);
  const seenAppsRef = useRef<Set<string>>(new Set());
  const wasActiveRef = useRef(false);

  useEffect(() => {
    activeRef.current = active;
  }, [active]);

  useEffect(() => {
    isFullscreenRef.current = isFullscreen;
  }, [isFullscreen]);

  useEffect(() => {
    screenReadyRef.current = screenReady;
  }, [screenReady]);

  useEffect(() => {
    onProctorEventRef.current = onProctorEvent;
  }, [onProctorEvent]);

  useEffect(() => {
    if (seqAttemptIdRef.current !== attemptId) {
      seqAttemptIdRef.current = attemptId;
      cameraSeqRef.current = 0;
      screenSeqRef.current = 0;
    }
  }, [attemptId]);

  const refreshApps = useCallback(async () => {
    if (!isProctored) return;
    setAppsBusy(true);
    setAppsError("");
    try {
      const raw = await invoke<unknown>("lms_proctor_list_apps");
      if (!mountedRef.current) return;
      setOpenApps(normalizeApps(raw));
    } catch (err) {
      if (!mountedRef.current) return;
      setAppsError(extractError(err, "Could not inspect running applications."));
    } finally {
      if (mountedRef.current) {
        setAppsBusy(false);
      }
    }
  }, [isProctored]);

  const quitOtherApps = useCallback(async () => {
    if (!isProctored) return;
    setAppsBusy(true);
    setAppsError("");
    try {
      await invoke("lms_proctor_quit_other_apps");
      const raw = await invoke<unknown>("lms_proctor_list_apps");
      if (!mountedRef.current) return;
      const remaining = normalizeApps(raw);
      setOpenApps(remaining);
      if (remaining.length > 0) {
        setAppsError("Some applications could not be closed automatically. Close them manually and recheck.");
      }
    } catch (err) {
      if (!mountedRef.current) return;
      setAppsError(extractError(err, "Could not close other applications."));
    } finally {
      if (mountedRef.current) {
        setAppsBusy(false);
      }
    }
  }, [isProctored]);

  const enableCameraAndMic = useCallback(async () => {
    if (!isProctored) return;
    if (
      typeof navigator === "undefined" ||
      !navigator.mediaDevices ||
      typeof navigator.mediaDevices.getUserMedia !== "function"
    ) {
      setMediaError("Camera and microphone capture is not supported in this runtime.");
      setCameraReady(false);
      setMicReady(false);
      return;
    }

    setMediaError("");
    releaseMediaStream(mediaStreamRef.current, trackCleanupsRef.current);
    mediaStreamRef.current = null;

    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: { width: { ideal: 640 }, height: { ideal: 360 }, frameRate: { ideal: 15 } },
        audio: true,
      });

      if (!mountedRef.current) {
        releaseMediaStream(stream, []);
        return;
      }

      const videoTracks = stream.getVideoTracks();
      const audioTracks = stream.getAudioTracks();
      const hasLiveVideo = videoTracks.some((t) => t.readyState === "live" && t.enabled);
      const hasLiveAudio = audioTracks.some((t) => t.readyState === "live" && t.enabled);

      const cleanups: Array<() => void> = [];
      for (const track of videoTracks) {
        const onEnded = () => {
          if (!mountedRef.current) return;
          setCameraReady(false);
          setMediaError("Camera feed ended. Re-enable camera & mic to continue.");
          if (activeRef.current) {
            onProctorEventRef.current?.("proctor.camera_lost", {
              reason: "ended",
              label: track.label,
            });
          }
        };
        const onMute = () => {
          if (!mountedRef.current) return;
          setCameraReady(false);
          setMediaError("Camera feed was muted or interrupted.");
          if (activeRef.current) {
            onProctorEventRef.current?.("proctor.camera_lost", {
              reason: "mute",
              label: track.label,
            });
          }
        };
        const onUnmute = () => {
          if (!mountedRef.current || track.readyState !== "live") return;
          setCameraReady(true);
        };
        track.addEventListener("ended", onEnded);
        track.addEventListener("mute", onMute);
        track.addEventListener("unmute", onUnmute);
        cleanups.push(() => {
          track.removeEventListener("ended", onEnded);
          track.removeEventListener("mute", onMute);
          track.removeEventListener("unmute", onUnmute);
        });
      }

      for (const track of audioTracks) {
        const onEnded = () => {
          if (!mountedRef.current) return;
          setMicReady(false);
          setMediaError("Microphone feed ended. Re-enable camera & mic to continue.");
          if (activeRef.current) {
            onProctorEventRef.current?.("proctor.mic_lost", {
              reason: "ended",
              label: track.label,
            });
          }
        };
        const onMute = () => {
          if (!mountedRef.current) return;
          setMicReady(false);
          setMediaError("Microphone feed was muted or interrupted.");
          if (activeRef.current) {
            onProctorEventRef.current?.("proctor.mic_lost", {
              reason: "mute",
              label: track.label,
            });
          }
        };
        const onUnmute = () => {
          if (!mountedRef.current || track.readyState !== "live") return;
          setMicReady(true);
        };
        track.addEventListener("ended", onEnded);
        track.addEventListener("mute", onMute);
        track.addEventListener("unmute", onUnmute);
        cleanups.push(() => {
          track.removeEventListener("ended", onEnded);
          track.removeEventListener("mute", onMute);
          track.removeEventListener("unmute", onUnmute);
        });
      }

      mediaStreamRef.current = stream;
      trackCleanupsRef.current = cleanups;
      setMediaStream(stream);
      setCameraReady(hasLiveVideo);
      setMicReady(hasLiveAudio);
      if (!hasLiveVideo || !hasLiveAudio) {
        setMediaError(
          !hasLiveVideo && !hasLiveAudio
            ? "Both camera and microphone tracks are unavailable."
            : !hasLiveVideo
              ? "Camera track is unavailable."
              : "Microphone track is unavailable.",
        );
      }
    } catch (err) {
      if (!mountedRef.current) return;
      setMediaStream(null);
      setCameraReady(false);
      setMicReady(false);
      setMediaError(
        extractError(
          err,
          "Camera and microphone access was denied. Grant permissions in System Settings and try again.",
        ),
      );
    }
  }, [isProctored]);

  const verifyScreenCapture = useCallback(async () => {
    if (!isProctored) return;
    setScreenError("");
    try {
      const raw = await invoke<unknown>("lms_proctor_capture_screen");
      const { dataBase64 } = extractScreenCapture(raw);
      if (!dataBase64) {
        throw new Error("Screen capture returned no image data.");
      }
      if (!mountedRef.current) return;
      setScreenReady(true);
      setScreenError("");
    } catch (err) {
      if (!mountedRef.current) return;
      setScreenReady(false);
      setScreenError(
        extractError(
          err,
          "Screen capture permission is required. Enable Screen Recording in System Settings → Privacy & Security.",
        ),
      );
    }
  }, [isProctored]);

  const enterFullscreen = useCallback(async () => {
    if (!isProctored) return;
    setFullscreenBusy(true);
    try {
      await invoke("lms_proctor_set_fullscreen", { enabled: true });
      const activeFs = await invoke<boolean>("lms_proctor_get_fullscreen").catch(() => true);
      if (!mountedRef.current) return;
      setIsFullscreen(Boolean(activeFs));
    } catch {
      try {
        if (typeof document !== "undefined" && document.documentElement.requestFullscreen) {
          await document.documentElement.requestFullscreen();
          if (mountedRef.current) {
            setIsFullscreen(Boolean(document.fullscreenElement));
          }
        }
      } catch {
        if (mountedRef.current) {
          setIsFullscreen(false);
        }
      }
    } finally {
      if (mountedRef.current) {
        setFullscreenBusy(false);
      }
    }
  }, [isProctored]);

  const exitFullscreen = useCallback(async () => {
    if (!desktop) return;
    setFullscreenBusy(true);
    try {
      await invoke("lms_proctor_set_fullscreen", { enabled: false });
    } catch {
      /* Fallback to DOM fullscreen exit below. */
    }
    try {
      if (typeof document !== "undefined" && document.fullscreenElement) {
        await document.exitFullscreen();
      }
    } catch {
      /* Ignore DOM fullscreen exit rejection. */
    }
    if (mountedRef.current) {
      setIsFullscreen(false);
      setFullscreenBusy(false);
    }
  }, [desktop]);

  // Initial pre-contest hardware/lockdown state check when entering ready or active view.
  useEffect(() => {
    if (!isProctored || (!ready && !active)) return;
    let cancelled = false;
    void Promise.allSettled([
      invoke<unknown>("lms_proctor_list_apps"),
      invoke<boolean>("lms_proctor_get_fullscreen"),
    ]).then(([appsResult, fsResult]) => {
      if (cancelled || !mountedRef.current) return;
      if (appsResult.status === "fulfilled") {
        setOpenApps(normalizeApps(appsResult.value));
        setAppsError("");
      } else {
        setAppsError(extractError(appsResult.reason, "Could not inspect running applications."));
      }
      if (fsResult.status === "fulfilled") {
        setIsFullscreen(Boolean(fsResult.value));
      } else if (typeof document !== "undefined") {
        setIsFullscreen(Boolean(document.fullscreenElement));
      }
    });
    return () => {
      cancelled = true;
    };
  }, [isProctored, ready, active]);

  // Keep DOM fullscreenchange events reflected in fullscreen state as a fallback.
  useEffect(() => {
    if (!isProctored || typeof document === "undefined") return;
    const onFullscreenChange = () => {
      if (!mountedRef.current) return;
      if (document.fullscreenElement) {
        setIsFullscreen(true);
      } else {
        void invoke<boolean>("lms_proctor_get_fullscreen")
          .then((nativeFs) => {
            if (mountedRef.current) setIsFullscreen(Boolean(nativeFs));
          })
          .catch(() => {
            if (mountedRef.current) setIsFullscreen(false);
          });
      }
    };
    document.addEventListener("fullscreenchange", onFullscreenChange);
    return () => document.removeEventListener("fullscreenchange", onFullscreenChange);
  }, [isProctored]);

  // Camera + microphone MediaRecorder chunk upload loop (every 15 seconds).
  useEffect(() => {
    if (!isProctored || !active || !attemptId || !mediaStream) return;
    if (typeof MediaRecorder === "undefined") return;

    const mimeType = pickRecorderMimeType();
    let recorder: MediaRecorder;
    try {
      recorder = new MediaRecorder(mediaStream, {
        ...(mimeType ? { mimeType } : {}),
        videoBitsPerSecond: 450_000,
        audioBitsPerSecond: 64_000,
      });
    } catch {
      try {
        recorder = mimeType
          ? new MediaRecorder(mediaStream, { mimeType })
          : new MediaRecorder(mediaStream);
      } catch (err) {
        Promise.resolve().then(() => {
          if (mountedRef.current) {
            setLastUploadError(extractError(err, "Could not start camera/audio recorder."));
          }
        });
        return;
      }
    }

    recorderRef.current = recorder;

    recorder.ondataavailable = (event: BlobEvent) => {
      if (!event.data || event.data.size === 0) return;
      const seq = ++cameraSeqRef.current;
      const contentType = event.data.type || mimeType || "video/webm";
      const capturedAt = new Date().toISOString();
      const deviceId = getDeviceId();

      void blobToBase64(event.data)
        .then((dataBase64) => {
          if (!dataBase64) return;
          return api(`/contests/${contestId}/media`, "POST", {
            attemptId,
            deviceId,
            kind: "camera_audio",
            seq,
            contentType,
            durationMs: CAMERA_CHUNK_MS,
            capturedAt,
            dataBase64,
          });
        })
        .then(() => {
          if (!mountedRef.current) return;
          setCameraChunksUploaded((count) => count + 1);
          setLastUploadError("");
        })
        .catch((err) => {
          if (!mountedRef.current) return;
          setLastUploadError(extractError(err, "Failed to upload camera recording chunk."));
        });
    };

    recorder.start(CAMERA_CHUNK_MS);

    return () => {
      if (recorderRef.current === recorder) {
        recorderRef.current = null;
      }
      if (recorder.state !== "inactive") {
        try {
          if (recorder.state === "recording") {
            recorder.requestData();
          }
          recorder.stop();
        } catch {
          /* Ignore MediaRecorder teardown errors if tracks already stopped. */
        }
      }
    };
  }, [isProctored, active, attemptId, contestId, mediaStream]);

  // Periodic screen snapshot capture & upload loop (every 10 seconds).
  useEffect(() => {
    if (!isProctored || !active || !attemptId) return;

    const captureAndUpload = async () => {
      try {
        const raw = await invoke<unknown>("lms_proctor_capture_screen");
        const { dataBase64, contentType } = extractScreenCapture(raw);
        if (!dataBase64) {
          throw new Error("Screen capture returned no image data.");
        }
        if (mountedRef.current && !screenReadyRef.current) {
          setScreenReady(true);
          setScreenError("");
        }
        const seq = ++screenSeqRef.current;
        try {
          await api(`/contests/${contestId}/media`, "POST", {
            attemptId,
            deviceId: getDeviceId(),
            kind: "screen_snapshot",
            seq,
            contentType,
            capturedAt: new Date().toISOString(),
            dataBase64,
          });
          if (mountedRef.current) {
            setScreenShotsUploaded((count) => count + 1);
            setLastUploadError("");
          }
        } catch (uploadErr) {
          if (mountedRef.current) {
            setLastUploadError(extractError(uploadErr, "Failed to upload screen snapshot."));
          }
        }
      } catch (captureErr) {
        const message = extractError(
          captureErr,
          "Screen capture failed during contest.",
        );
        const wasReady = screenReadyRef.current;
        if (mountedRef.current) {
          setScreenReady(false);
          setScreenError(message);
        }
        if (wasReady) {
          onProctorEventRef.current?.("proctor.screen_lost", { error: message });
        }
      }
    };

    const timer = window.setInterval(() => {
      void captureAndUpload();
    }, SCREEN_SNAPSHOT_MS);

    return () => {
      window.clearInterval(timer);
    };
  }, [isProctored, active, attemptId, contestId]);

  // Mid-contest app & fullscreen watchdog loop (every 5 seconds).
  useEffect(() => {
    if (!isProctored || !active || !attemptId) return;
    seenAppsRef.current = new Set();

    const pollWatchdog = async () => {
      const [appsResult, fsResult] = await Promise.allSettled([
        invoke<unknown>("lms_proctor_list_apps"),
        invoke<boolean>("lms_proctor_get_fullscreen"),
      ]);

      if (appsResult.status === "fulfilled") {
        const apps = normalizeApps(appsResult.value);
        if (mountedRef.current) {
          setOpenApps(apps);
          setAppsError("");
        }
        const currentKeys = new Set<string>();
        const newlySeen: ProctorApp[] = [];
        for (const app of apps) {
          const key = `${app.bundleId || app.name}:${app.pid}`;
          currentKeys.add(key);
          if (!seenAppsRef.current.has(key)) {
            newlySeen.push(app);
          }
        }
        seenAppsRef.current = currentKeys;
        if (newlySeen.length > 0) {
          onProctorEventRef.current?.("app.opened", {
            apps: newlySeen.map((app) => ({
              name: app.name,
              bundleId: app.bundleId,
              pid: app.pid,
            })),
          });
        }
      }

      if (fsResult.status === "fulfilled") {
        const nextFs = Boolean(fsResult.value);
        const prevFs = isFullscreenRef.current;
        if (mountedRef.current) {
          setIsFullscreen(nextFs);
        }
        if (prevFs && !nextFs) {
          onProctorEventRef.current?.("fullscreen.exit", { source: "watchdog" });
        }
      }
    };

    const timer = window.setInterval(() => {
      void pollWatchdog();
    }, WATCHDOG_POLL_MS);

    return () => {
      window.clearInterval(timer);
    };
  }, [isProctored, active, attemptId]);

  // Tear down recording, media tracks, and fullscreen when an active contest ends (e.g. submitted).
  useEffect(() => {
    if (active && isProctored) {
      wasActiveRef.current = true;
      return;
    }
    if (!active && wasActiveRef.current) {
      wasActiveRef.current = false;
      const recorder = recorderRef.current;
      if (recorder && recorder.state !== "inactive") {
        try {
          if (recorder.state === "recording") recorder.requestData();
          recorder.stop();
        } catch {
          /* Ignore recorder stop error during teardown. */
        }
      }
      releaseMediaStream(mediaStreamRef.current, trackCleanupsRef.current);
      mediaStreamRef.current = null;
      void exitFullscreen().finally(() => {
        if (!mountedRef.current) return;
        setMediaStream(null);
        setCameraReady(false);
        setMicReady(false);
        setScreenReady(false);
      });
    }
  }, [active, isProctored, exitFullscreen]);

  // Final cleanup on component unmount.
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      const recorder = recorderRef.current;
      if (recorder && recorder.state !== "inactive") {
        try {
          if (recorder.state === "recording") recorder.requestData();
          recorder.stop();
        } catch {
          /* Ignore recorder stop error on unmount. */
        }
      }
      releaseMediaStream(mediaStreamRef.current, trackCleanupsRef.current);
      mediaStreamRef.current = null;
      if (desktop && (wasActiveRef.current || isFullscreenRef.current)) {
        void invoke("lms_proctor_set_fullscreen", { enabled: false }).catch(() => {});
        if (typeof document !== "undefined" && document.fullscreenElement) {
          void document.exitFullscreen().catch(() => {});
        }
      }
    };
  }, [desktop]);

  const allChecksPassed =
    openApps.length === 0 && cameraReady && micReady && screenReady && isFullscreen;

  const lockdownViolation =
    isProctored &&
    active &&
    (openApps.length > 0 || !isFullscreen || !cameraReady || !micReady || !screenReady);

  return {
    openApps,
    appsBusy,
    appsError,
    refreshApps,
    quitOtherApps,
    cameraReady,
    micReady,
    mediaError,
    mediaStream,
    enableCameraAndMic,
    screenReady,
    screenError,
    verifyScreenCapture,
    isFullscreen,
    fullscreenBusy,
    enterFullscreen,
    exitFullscreen,
    allChecksPassed,
    lockdownViolation,
    cameraChunksUploaded,
    screenShotsUploaded,
    lastUploadError,
  };
}
