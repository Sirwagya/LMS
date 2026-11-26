export type ContestSummary = {
  id: string;
  title: string;
  status: "draft" | "published" | "cancelled";
  opensAt: string;
  closesAt: string;
  durationMinutes: number;
  attemptState?: "active" | "submitted" | "cancelled";
};

export type ContestPage = { items: ContestSummary[]; hasMore: boolean };
export type ContestTestCase = { input: string; expected: string; hidden: boolean };
export type ContestQuestion =
  | { kind: "mcq"; prompt: string; points: number; choices: string[]; correctChoice: number; explanation?: string }
  | { kind: "coding"; prompt: string; points: number; testCases: ContestTestCase[] };

export type ContestDraft = Pick<ContestSummary, "title" | "opensAt" | "closesAt" | "durationMinutes"> & {
  instructions: string;
  studentIds: string[];
  questions: ContestQuestion[];
};

export type ContestRecord = ContestSummary & ContestDraft & {
  revision: number;
  cancellationReason?: string;
};

const dateFormat = new Intl.DateTimeFormat(undefined, {
  dateStyle: "medium",
  timeStyle: "short",
});

export function contestTime(date: string) {
  return dateFormat.format(new Date(date));
}

export function newQuestion(kind: ContestQuestion["kind"]): ContestQuestion {
  return kind === "mcq"
    ? { kind, prompt: "", points: 10, choices: ["", "", "", ""], correctChoice: 0 }
    : { kind, prompt: "", points: 20, testCases: [
        { input: "", expected: "", hidden: false },
        { input: "", expected: "", hidden: true },
      ] };
}
