export interface Subject {
  code: string;
  credits: number;
  name: string;
  instructor?: string;
  next: string;
  att: string;
  attendedClasses?: number;
  totalClasses?: number;
  skips: string;
}

export interface Notice {
  id?: string;
  tag: string;
  tagBg: string;
  time: string;
  title: string;
  desc: string;
  actionLabel?: string;
  actionRoute?: string;
}

export interface CodingProblem {
  entrypoint?: "twoSum" | "isValid" | "solve";
  description: string;
  inputFormat: string;
  outputFormat: string;
  constraints: string;
  difficulty: "Easy" | "Medium" | "Hard";
  starterCode: { python: string; java: string };
  examples?: { input: string; expected: string }[];
  testCases?: { input: string; expected: string; hidden: boolean }[];
  totalTests?: number;
}
export interface Assignment {
  kind?: "coding" | "written";
  problem?: CodingProblem;
  id: string | number;
  title: string;
  dueDate?: string;
  status?: "Submitted" | "Pending" | "Graded";
  score?: string;
  uploadedAt?: string;
  url?: string | null;
}

export interface Resource {
  title: string;
  url?: string | null;
  type?: string;
  size?: string;
}

export interface Lecture {
  id: number;
  code: string;
  date: string;
  month: string;
  unit: number;
  title: string;
  rawTitle: string;
  attendance: "Present" | "Awaiting" | "Excused";
  summary: string;
  slides: string;
  duration?: string;
  assignments: Assignment[];
  resources: Resource[];
  recordingUrl?: string | null;
}

export interface ScheduleClass {
  from: string;
  to: string;
  code: string;
  room: string;
  topic: string;
  type?: "Lecture" | "Lab" | "Workshop";
}

export interface WeekSchedule {
  start: string;
  published: boolean;
  days: Record<string, ScheduleClass[]>;
}

export interface ContestAttempt {
  label: string;
  title: string;
  date: string;
  score: string;
  percent: number;
  rank: string;
}

export interface ContestSummaryItem {
  percent: number;
  detail: string;
  completed?: number;
  total?: number;
  bestRank?: string;
  history?: ContestAttempt[];
}

export interface ContestRecord {
  id: string;
  title: string;
  subjectCode: string;
  date: string;
  score: string;
  percent: number;
  rank: string;
  status: "Completed" | "Reviewed";
}

export interface FeeItem {
  id: string;
  invoiceNo?: string;
  description: string;
  semester: string;
  amount: number;
  dueDate: string;
  paidDate?: string;
  status: "Paid" | "Overdue" | "Pending";
  receiptAvailable: boolean;
}
