import { useEffect, useRef, useState } from "react";
import { api, transport } from "../api";
import { useSession } from "../session";
import type { CodingProblem } from "../../types";
import type { CodingAttempt, JudgeResult, Language } from "./types";

export function useCodingWorkspace(id: string, problem: CodingProblem) {
  const { user, refresh } = useSession(),
    [language, setLanguage] = useState<Language>("python");
  const entrypoint = problem.entrypoint || "stdin";
  const key = (lang: Language) =>
    `vedam-code:${user.id}:${id}:${lang}:${entrypoint}`;
  const loadLocal = (lang: Language) => {
    if (transport.isDesktop) return problem.starterCode[lang];
    try {
      return localStorage.getItem(key(lang)) ?? problem.starterCode[lang];
    } catch {
      return problem.starterCode[lang];
    }
  };
  const [source, setSource] = useState(() => loadLocal("python")),
    [busy, setBusy] = useState<null | "run" | "submit">(null),
    [error, setError] = useState(""),
    [result, setResult] = useState<JudgeResult | null>(null),
    [history, setHistory] = useState<CodingAttempt[]>([]);
  const dirtyRef = useRef(false);
  const saveTimerRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (saveTimerRef.current !== null) {
        window.clearTimeout(saveTimerRef.current);
      }
    };
  }, []);

  useEffect(() => {
    let active = true;
    dirtyRef.current = false;
    void transport
      .loadCodeDraft(user.id, id, language, entrypoint)
      .then((draft) => {
        if (active && !dirtyRef.current && draft !== null) {
          setSource(draft);
        }
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [user.id, id, language, entrypoint]);

  useEffect(() => {
    let current = true;
    api<CodingAttempt[]>(
      `/coding/history?assignmentId=${encodeURIComponent(id)}`,
    )
      .then((h) => {
        if (current) setHistory(h);
      })
      .catch((e) => {
        if (current) setError(e.message);
      });
    return () => {
      current = false;
    };
  }, [id]);

  function persistDraftImmediate(lang: Language, value: string) {
    if (saveTimerRef.current !== null) {
      window.clearTimeout(saveTimerRef.current);
      saveTimerRef.current = null;
    }
    void transport
      .saveCodeDraft(user.id, id, lang, entrypoint, value)
      .catch(() => {});
  }

  function edit(value: string) {
    dirtyRef.current = true;
    setSource(value);
    try {
      if (!transport.isDesktop) localStorage.setItem(key(language), value);
    } catch {
      /* Draft remains in memory. */
    }
    if (saveTimerRef.current !== null) {
      window.clearTimeout(saveTimerRef.current);
    }
    const activeLanguage = language;
    saveTimerRef.current = window.setTimeout(() => {
      saveTimerRef.current = null;
      void transport
        .saveCodeDraft(user.id, id, activeLanguage, entrypoint, value)
        .catch(() => {});
    }, 500);
  }

  function changeLanguage(lang: Language) {
    if (saveTimerRef.current !== null) {
      window.clearTimeout(saveTimerRef.current);
      saveTimerRef.current = null;
      void transport
        .saveCodeDraft(user.id, id, language, entrypoint, source)
        .catch(() => {});
    }
    dirtyRef.current = false;
    setLanguage(lang);
    setSource(loadLocal(lang));
    setResult(null);
  }

  async function evaluate(action: "run" | "submit", customInput?: string) {
    setBusy(action);
    setError("");
    setResult(null);
    try {
      const r = await api<JudgeResult>("/coding/evaluate", "POST", {
        assignmentId: id,
        source,
        language,
        action,
        ...(action === "run" && customInput !== undefined
          ? { customInput }
          : {}),
      });
      setResult(r);
      if (action === "submit") {
        setHistory(
          await api<CodingAttempt[]>(
            `/coding/history?assignmentId=${encodeURIComponent(id)}`,
          ),
        );
        await refresh();
      }
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(null);
    }
  }

  return {
    language,
    source,
    edit,
    changeLanguage,
    busy,
    error,
    result,
    history,
    evaluate,
    restore: (attempt: CodingAttempt) => {
      dirtyRef.current = true;
      setLanguage(attempt.language);
      setSource(attempt.source);
      setResult(null);
      persistDraftImmediate(attempt.language, attempt.source);
    },
    reset: () => {
      dirtyRef.current = true;
      const starter = problem.starterCode[language];
      setSource(starter);
      persistDraftImmediate(language, starter);
    },
  };
}
