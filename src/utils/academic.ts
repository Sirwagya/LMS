import type { Subject } from "../types";
import { contestSummary, demoWeek } from "../data/lmsData";

export function attendance(subject: Subject) {
  const attended = subject.attendedClasses ?? 0;
  const total = subject.totalClasses ?? 0;
  return { attended, total, percent: total ? (attended / total) * 100 : 0 };
}

export function semesterStats(subjects: Subject[]) {
  const attended = subjects.reduce((sum, s) => sum + attendance(s).attended, 0);
  const total = subjects.reduce((sum, s) => sum + attendance(s).total, 0);
  const results = subjects.flatMap((s) =>
    contestSummary[s.code] ? [contestSummary[s.code].percent] : [],
  );
  return {
    attended,
    total,
    percent: total ? (attended / total) * 100 : 0,
    contestAverage: results.length
      ? Math.round(results.reduce((a, b) => a + b, 0) / results.length)
      : null,
  };
}

export const formatCurrency = (amount: number) =>
  new Intl.NumberFormat("en-IN", {
    style: "currency",
    currency: "INR",
    maximumFractionDigits: 0,
  }).format(amount);
export const localISO = (date: Date) =>
  `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
export const formatDate = (
  date: Date,
  options: Intl.DateTimeFormatOptions = { day: "numeric", month: "short" },
) => new Intl.DateTimeFormat("en-GB", options).format(date);
export const publishedMonday = () => new Date(`${demoWeek.start}T12:00:00`);
