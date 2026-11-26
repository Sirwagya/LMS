import { useCallback, useEffect, useRef, useState } from "react";
import { api, transport } from "../api";
import type { Language } from "../coding/types";
import type { ContestSummary } from "./types";

export type Answer = { choice: number } | { language: Language; source: string };
type Answers = Record<number, Answer>;
export type Question = {
  kind: "mcq" | "coding";
  prompt: string;
  points: number;
  choices?: string[];
  examples?: { input: string; expected: string }[];
};
type Attempt = ContestSummary & {
  instructions: string;
  state: "ready" | "upcoming" | "closed" | "cancelled" | "occupied" | "active" | "submitted";
  serverTime: string;
  attemptId?: string;
  deadlineAt?: string;
  submittedAt?: string;
  finishReason?: "manual" | "expired";
  lease: number;
  revision: number;
  answers: Answers | null;
  questions?: Question[];
};
type Checkpoint = {
  attemptId: string;
  lease: number;
  revision: number;
  answers: Answers;
  pending?: Answers;
};
type Control = "state" | "start" | "takeover";

function localRead(key: string) {
  try { return localStorage.getItem(key); } catch { return null; }
}

function localWrite(key: string, value: string | null) {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
    return true;
  } catch {
    return false;
  }
}

function readCheckpoint(key: string): Checkpoint | null {
  try { return JSON.parse(localRead(key) || "null") as Checkpoint | null; } catch { return null; }
}

function sameAnswers(left: Answers, right: Answers) {
  return Object.keys(left).length === Object.keys(right).length &&
    Object.entries(left).every(([key, a]) => {
      const b = right[Number(key)];
      if (!b) return false;
      if ("choice" in a) return "choice" in b && a.choice === b.choice;
      return "source" in b && a.source === b.source && a.language === b.language;
    });
}

function sourceLimit(answers: Answers) {
  const encoder = new TextEncoder();
  let total = 0;
  for (const answer of Object.values(answers)) {
    if (!("source" in answer)) continue;
    const bytes = encoder.encode(answer.source).length;
    if (bytes > 32 * 1024 || answer.source.includes("\0")) {
      return "Keep source to 32 KiB per question and remove null characters.";
    }
    total += bytes;
  }
  return total > 128 * 1024 ? "Keep combined source to 128 KiB." : "";
}

function secondsUntil(deadline: number) {
  return Math.max(0, Math.ceil((deadline - performance.now()) / 1000));
}

export function useContestAttempt(id: string, userId: string) {
  const storageKey = `lms:contest-work:${userId}:${id}`;
  const [deviceId] = useState(() => {
    if (!transport.isDesktop) return "";
    const key = "lms:contest-device";
    const existing = localRead(key);
    if (existing) return existing;
    const next = crypto.randomUUID();
    localWrite(key, next);
    return next;
  });
  const [view, setView] = useState<Attempt | null>(null);
  const [answers, setAnswers] = useState<Answers>({});
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [busy, setBusy] = useState(false);
  const [localDurable, setLocalDurable] = useState(true);
  const [error, setError] = useState("");
  const [remaining, setRemaining] = useState(0);
  const current = useRef<Attempt | null>(null);
  const checkpoint = useRef<Checkpoint | null>(null);
  const inFlight = useRef(false);
  const expires = useRef(0);
  const mounted = useRef(false);
  const sizeError = sourceLimit(answers);

  const saveCheckpoint = useCallback((next: Checkpoint | null, saved: Answers = {}) => {
    checkpoint.current = next;
    const changed = !!next && (!!next.pending || !sameAnswers(next.answers, saved));
    setDirty(changed);
    setLocalDurable(localWrite(storageKey, changed ? JSON.stringify(next) : null));
    setAnswers(next?.answers ?? saved);
  }, [storageKey]);

  const apply = useCallback((result: Attempt, sentAt: number) => {
    const merged = result.state === "active"
      ? { ...result, questions: result.questions ?? current.current?.questions }
      : result;
    current.current = merged;
    setView(merged);
    if (result.deadlineAt) {
      const serverDelta = Date.parse(result.deadlineAt) - Date.parse(result.serverTime);
      expires.current = (sentAt + performance.now()) / 2 + serverDelta;
      setRemaining(secondsUntil(expires.current));
    }
  }, []);

  const request = useCallback(async (action: string, extra?: object) => {
    const started = performance.now();
    const result = await api<Attempt>(`/contests/${id}/attempt`, "POST", { action, deviceId, ...extra });
    if (mounted.current) apply(result, started);
    return result;
  }, [apply, deviceId, id]);

  const open = useCallback(async (action: Control, discard = false) => {
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    try {
      const result = await request(action, { lease: current.current?.lease });
      if (!mounted.current) return;
      const saved = result.answers ?? {};
      if (result.state === "active" && result.attemptId) {
        const local = discard ? null : checkpoint.current ?? readCheckpoint(storageKey);
        const matches = local?.attemptId === result.attemptId && local.lease === result.lease;
        const sameRevision = matches && local.revision === result.revision;
        const previousWriteAccepted = matches && local.revision + 1 === result.revision &&
          !!local.pending && sameAnswers(local.pending, saved);
        if (matches && !sameRevision && !previousWriteAccepted && !sameAnswers(local.answers, saved)) {
          setError("Saved answers changed. Reload server answers to continue on this device.");
          return;
        }
        saveCheckpoint({
          attemptId: result.attemptId, lease: result.lease, revision: result.revision,
          answers: sameRevision || previousWriteAccepted ? local.answers : saved,
          pending: sameRevision ? local.pending : undefined,
        }, saved);
      } else if (result.state === "submitted" || result.state === "cancelled") {
        saveCheckpoint(null, saved);
      }
    } catch (e) {
      setError((e as Error).message);
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }, [request, saveCheckpoint, storageKey]);

  useEffect(() => {
    mounted.current = true;
    if (transport.isDesktop) void open("state");
    return () => { mounted.current = false; };
  }, [open]);

  const flush = useCallback(async (submit: boolean) => {
    let draft = checkpoint.current;
    if (inFlight.current || !draft || current.current?.state !== "active" || sourceLimit(draft.answers)) return;
    inFlight.current = true;
    setSaving(true);
    if (submit) setBusy(true);
    setError("");
    try {
      const write = async (action: "save" | "submit", next: Checkpoint, snapshot: Answers) => {
        saveCheckpoint({ ...next, pending: snapshot }, current.current?.answers ?? {});
        const result = await request(action, { lease: next.lease, revision: next.revision, answers: snapshot });
        if (!mounted.current) return;
        if (result.state === "active" && checkpoint.current) {
          saveCheckpoint({ ...checkpoint.current, revision: result.revision, pending: undefined }, result.answers ?? {});
        } else if (result.state === "submitted" || result.state === "cancelled") {
          saveCheckpoint(null, result.answers ?? {});
        } else if (result.state === "occupied") {
          setError("This attempt moved to another device. Unsynced edits remain on this device.");
        }
      };
      if (submit && draft.pending) {
        await write("save", draft, draft.pending);
        draft = checkpoint.current;
      }
      if (mounted.current && draft && current.current?.state === "active") {
        await write(submit ? "submit" : "save", draft, (!submit && draft.pending) || draft.answers);
      }
    } catch (e) {
      setError((e as Error).message);
    } finally {
      inFlight.current = false;
      setSaving(false);
      if (submit) setBusy(false);
    }
  }, [request, saveCheckpoint]);

  useEffect(() => {
    if (view?.state !== "active") return;
    const tick = window.setInterval(() => setRemaining(secondsUntil(expires.current)), 250);
    return () => window.clearInterval(tick);
  }, [view?.state]);

  useEffect(() => {
    if (view?.state !== "active" || busy || saving || error) return;
    if (remaining === 0) {
      const refresh = window.setTimeout(() => void open("state"), 500);
      return () => window.clearTimeout(refresh);
    }
  }, [view, busy, saving, error, remaining, open]);

  useEffect(() => {
    if (view?.state !== "active" || !dirty || saving || busy || error || sizeError) return;
    const timeout = window.setTimeout(() => {
      if (secondsUntil(expires.current) > 0) void flush(false);
    }, 850);
    return () => window.clearTimeout(timeout);
  }, [view?.state, dirty, saving, busy, error, sizeError, flush]);

  useEffect(() => {
    if (view?.state !== "active" || busy || saving || !error.startsWith("Server unavailable")) return;
    const retry = window.setTimeout(() => void open("state"), 5000);
    return () => window.clearTimeout(retry);
  }, [view?.state, busy, saving, error, open]);

  useEffect(() => {
    const reconnect = () => {
      if (current.current?.state === "active") void open("state");
    };
    window.addEventListener("online", reconnect);
    return () => window.removeEventListener("online", reconnect);
  }, [open]);

  function edit(index: number, answer: Answer) {
    const draft = checkpoint.current;
    if (!draft || busy || current.current?.state !== "active" || secondsUntil(expires.current) === 0) return;
    const next = { ...draft.answers, [index]: answer };
    saveCheckpoint({ ...draft, answers: next }, current.current?.answers ?? {});
  }

  return { view, answers, dirty, saving, busy, localDurable, error, sizeError, remaining, open, flush, edit };
}
