import { useCallback, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { LMSContext } from "./state";
import {
  semesterSubjects,
  lecturesBySubject,
  feeRecords,
} from "../data/lmsData";

function readRoute() {
  try {
    const hash = window.location.hash.slice(1);
    return hash ? decodeURIComponent(hash) : "/dashboard";
  } catch {
    return "/not-found";
  }
}

function semesterForRoute(route: string) {
  if (!route.startsWith("/subjects/")) return undefined;
  const code = route.split("/")[2];
  return [1, 2, 3].find((sem) =>
    semesterSubjects[sem]?.some((s) => s.code === code),
  );
}

export function LMSProvider({ children }: { children: ReactNode }) {
  const [route, setRoute] = useState(readRoute);
  const [currentSemester, setCurrentSemester] = useState(
    () => semesterForRoute(readRoute()) || 3,
  );
  const [theme, setTheme] = useState<"light" | "dark">(() => {
    try {
      const saved = localStorage.getItem("vedam-theme");
      return saved === "light" || saved === "dark"
        ? saved
        : window.matchMedia("(prefers-color-scheme: dark)").matches
          ? "dark"
          : "light";
    } catch {
      return "light";
    }
  });
  const [overviewTab, setOverviewTab] = useState<"attendance" | "contests">(
    "attendance",
  );
  const [weekOffset, setWeekOffset] = useState(0);
  const [isPaymentModalOpen, setIsPaymentModalOpen] = useState(false);
  const [isNoticesDrawerOpen, setIsNoticesDrawerOpen] = useState(false);
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const currentSubjectCode = route.startsWith("/subjects/")
    ? route.split("/")[2] || ""
    : "";
  const selectedLectureId =
    route.split("/")[3] === "lectures"
      ? Number(route.split("/")[4])
      : (lecturesBySubject[currentSubjectCode]?.[0]?.id ?? null);
  const allSubjects = useMemo(
    () => semesterSubjects[currentSemester] || [],
    [currentSemester],
  );
  const subjectByCode = useCallback(
    (code: string) =>
      Object.values(semesterSubjects)
        .flat()
        .find((s) => s.code === code),
    [],
  );
  const toggleTheme = useCallback(
    () => setTheme((prev) => (prev === "dark" ? "light" : "dark")),
    [],
  );
  const showToast = useCallback(
    (message: string) => setToastMessage(message),
    [],
  );
  const changeWeek = useCallback(
    (delta: number) =>
      setWeekOffset((prev) => Math.max(-2, Math.min(8, prev + delta))),
    [],
  );
  const resetWeek = useCallback(() => setWeekOffset(0), []);
  const setSemester = useCallback(
    (semester: number) => {
      if (![1, 2, 3].includes(semester)) return;
      setCurrentSemester(semester);
      if (
        route.startsWith("/subjects/") &&
        !semesterSubjects[semester]?.some((s) => s.code === currentSubjectCode)
      ) {
        window.location.hash = `/subjects/${semesterSubjects[semester]?.[0]?.code}`;
      }
    },
    [route, currentSubjectCode],
  );

  useEffect(() => {
    document.documentElement.classList.toggle("dark", theme === "dark");
    try {
      localStorage.setItem("vedam-theme", theme);
    } catch {
      /* Storage may be blocked. */
    }
  }, [theme]);

  useEffect(() => {
    const handleHashChange = () => {
      const next = readRoute();
      setRoute(next);
      const semester = semesterForRoute(next);
      if (semester) setCurrentSemester(semester);
      const sameCourse =
        next.startsWith("/subjects/") &&
        route.startsWith("/subjects/") &&
        next.split("/")[2] === route.split("/")[2];
      if (!sameCourse) {
        window.scrollTo({ top: 0, behavior: "instant" });
        document.getElementById("main-content")?.focus({ preventScroll: true });
      }
    };
    window.addEventListener("hashchange", handleHashChange);
    return () => window.removeEventListener("hashchange", handleHashChange);
  }, [route]);

  useEffect(() => {
    if (!toastMessage) return;
    const timer = window.setTimeout(() => setToastMessage(null), 5000);
    return () => window.clearTimeout(timer);
  }, [toastMessage]);

  useEffect(() => {
    const page = route.startsWith("/subjects/")
      ? subjectByCode(currentSubjectCode)?.name || "Course not found"
      : {
          "/dashboard": "Dashboard",
          "/overview": "Attendance & results",
          "/schedule": "Timetable",
          "/fees": "Fees & receipts",
          "/account": "Account",
        }[route] || "Page not found";
    document.title = `${page} · Vedam`;
  }, [route, currentSubjectCode, subjectByCode]);

  const copyText = useCallback(
    async (value: string) => {
      try {
        await navigator.clipboard.writeText(value);
        showToast("Enrollment ID copied to clipboard.");
      } catch {
        showToast(
          "Could not copy. Select the enrollment ID and copy it manually.",
        );
      }
    },
    [showToast],
  );

  return (
    <LMSContext.Provider
      value={{
        currentSemester,
        setSemester,
        theme,
        toggleTheme,
        route,
        currentSubjectCode,
        selectedLectureId,
        overviewTab,
        setOverviewTab,
        weekOffset,
        changeWeek,
        resetWeek,
        isPaymentModalOpen,
        setIsPaymentModalOpen,
        isNoticesDrawerOpen,
        setIsNoticesDrawerOpen,
        toastMessage,
        showToast,
        copyText,
        allSubjects,
        subjectByCode,
        fees: feeRecords,
        hasOverdueFees: feeRecords.some((f) => f.status === "Overdue"),
      }}
    >
      {children}
    </LMSContext.Provider>
  );
}
