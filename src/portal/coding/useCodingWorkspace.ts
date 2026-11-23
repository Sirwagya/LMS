import { useEffect, useState } from "react";
import { api } from "../api";
import { useSession } from "../session";
import type { CodingProblem } from "../../types";
import type { CodingAttempt, JudgeResult, Language } from "./types";
export function useCodingWorkspace(id: string, problem: CodingProblem) {
  const { user, refresh } = useSession(),
    [language, setLanguage] = useState<Language>("python");
  const key = (lang: Language) =>
    `vedam-code:${user.id}:${id}:${lang}:${problem.entrypoint || "stdin"}`;
  const load = (lang: Language) => {
    try {
      return localStorage.getItem(key(lang)) ?? problem.starterCode[lang];
    } catch {
      return problem.starterCode[lang];
    }
  };
  const [source, setSource] = useState(() => load("python")),
    [busy, setBusy] = useState<null | "run" | "submit">(null),
    [error, setError] = useState(""),
    [result, setResult] = useState<JudgeResult | null>(null),
    [history, setHistory] = useState<CodingAttempt[]>([]);
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
  function edit(value: string) {
    setSource(value);
    try {
      localStorage.setItem(key(language), value);
    } catch {
      /* Draft remains in memory. */
    }
  }
  function changeLanguage(lang: Language) {
    setLanguage(lang);
    setSource(load(lang));
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
      setLanguage(attempt.language);
      setSource(attempt.source);
      setResult(null);
      try {
        localStorage.setItem(key(attempt.language), attempt.source);
      } catch {
        /* Keep code in memory. */
      }
    },
    reset: () => edit(problem.starterCode[language]),
  };
}
