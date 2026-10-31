import { createContext } from "react";
import type { Subject, FeeItem } from "../types";

export interface LMSContextType {
  currentSemester: number;
  setSemester: (semester: number) => void;
  theme: "light" | "dark";
  toggleTheme: () => void;
  route: string;
  currentSubjectCode: string;
  selectedLectureId: number | null;
  overviewTab: "attendance" | "contests";
  setOverviewTab: (tab: "attendance" | "contests") => void;
  weekOffset: number;
  changeWeek: (delta: number) => void;
  resetWeek: () => void;
  isPaymentModalOpen: boolean;
  setIsPaymentModalOpen: (open: boolean) => void;
  isNoticesDrawerOpen: boolean;
  setIsNoticesDrawerOpen: (open: boolean) => void;
  toastMessage: string | null;
  showToast: (message: string) => void;
  copyText: (value: string) => Promise<void>;
  allSubjects: Subject[];
  subjectByCode: (code: string) => Subject | undefined;
  fees: FeeItem[];
  hasOverdueFees: boolean;
}

export const LMSContext = createContext<LMSContextType | undefined>(undefined);
