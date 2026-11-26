import { useCallback, useEffect, useRef, useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";

const MAX_WARNINGS = 3;
type WarningReason = "left_contest" | "paste_blocked";

export function useContestLockdown(
  enabled: boolean,
  active: boolean,
  serverWarnings: number,
  warn: (reason: WarningReason) => void,
) {
  const [localWarnings, setLocalWarnings] = useState(serverWarnings);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const count = useRef(serverWarnings);
  const warnRef = useRef(warn);
  const prepared = useRef(false);
  const enabling = useRef(false);
  const generation = useRef(0);

  useEffect(() => { warnRef.current = warn; }, [warn]);
  useEffect(() => {
    count.current = Math.max(serverWarnings, count.current);
  }, [serverWarnings]);

  const prepare = useCallback(async (clearClipboard: boolean) => {
    if (!enabled) return true;
    if (enabling.current) return false;
    enabling.current = true;
    const currentGeneration = generation.current;
    setBusy(true);
    try {
      await invoke("lms_proctor_lockdown", { enabled: true, clearClipboard });
      if (currentGeneration !== generation.current) {
        void invoke("lms_proctor_lockdown", { enabled: false, clearClipboard: false }).catch(() => {});
        return false;
      }
      prepared.current = true;
      setNotice("");
      return true;
    } catch {
      setNotice("Could not apply macOS contest lockdown. Reopen fullscreen and try again.");
      return false;
    } finally {
      enabling.current = false;
      setBusy(false);
    }
  }, [enabled]);

  const release = useCallback(() => {
    generation.current++;
    if (!prepared.current) return;
    prepared.current = false;
    void invoke("lms_proctor_lockdown", { enabled: false, clearClipboard: false }).catch(() => {});
  }, []);

  useEffect(() => () => release(), [release]);

  useEffect(() => {
    if (!enabled || !active) return;
    if (!prepared.current) void prepare(true);
    let disposed = false;
    let unlisten: (() => void) | undefined;
    let away = false;
    let checking = false;
    let restore: number | undefined;
    const issue = (reason: WarningReason) => {
      if (count.current >= MAX_WARNINGS) return;
      count.current++;
      setLocalWarnings(count.current);
      setNotice(reason === "paste_blocked" ? "Paste is blocked during this contest." : "Return to the fullscreen contest window.");
      warnRef.current(reason);
    };
    const leave = () => {
      if (!disposed && !away) {
        away = true;
        issue("left_contest");
      }
    };
    const inspect = async () => {
      if (checking || disposed) return;
      checking = true;
      try {
        const [focused, fullscreen] = await Promise.all([
          getCurrentWindow().isFocused(),
          invoke<boolean>("lms_proctor_get_fullscreen"),
        ]);
        if (disposed) return;
        if (!focused || !fullscreen) leave();
        else if (away) {
          away = false;
          void prepare(false);
        }
      } catch {
        // A desktop API failure alone is not evidence of a rule violation.
      } finally {
        checking = false;
      }
    };
    void getCurrentWindow().onFocusChanged(({ payload: focused }) => {
      window.clearTimeout(restore);
      if (!focused) restore = window.setTimeout(leave, 400);
      else void inspect();
    }).then((stop) => {
      if (disposed) stop();
      else unlisten = stop;
    }).catch(() => {});
    const timer = window.setInterval(() => void inspect(), 1000);

    const blockPaste = (event: Event) => {
      event.preventDefault();
      event.stopImmediatePropagation();
      issue("paste_blocked");
    };
    const blockKeyboardPaste = (event: KeyboardEvent) => {
      if (((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "v") ||
          (event.shiftKey && event.key === "Insert")) {
        if (!event.repeat) blockPaste(event);
        else event.preventDefault();
      }
    };
    window.addEventListener("keydown", blockKeyboardPaste, true);
    window.addEventListener("paste", blockPaste, true);
    window.addEventListener("drop", blockPaste, true);
    return () => {
      disposed = true;
      window.clearInterval(timer);
      window.clearTimeout(restore);
      unlisten?.();
      window.removeEventListener("keydown", blockKeyboardPaste, true);
      window.removeEventListener("paste", blockPaste, true);
      window.removeEventListener("drop", blockPaste, true);
      release();
    };
  }, [active, enabled, prepare, release]);

  const warnings = Math.max(serverWarnings, localWarnings);
  return { warnings, closing: warnings >= MAX_WARNINGS, notice, busy, prepare, release };
}
