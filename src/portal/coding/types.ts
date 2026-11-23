export type Language = "python" | "java";
export interface TestResult {
  index: number;
  hidden: boolean;
  status: string;
  durationMs: number;
  input?: string;
  expected?: string | null;
  stdout?: string;
  stderr?: string;
}
export interface JudgeResult {
  verdict: string;
  passed: number;
  total: number;
  compileError?: string;
  results: TestResult[];
  attemptId?: string;
}
export interface CodingAttempt {
  id: string;
  language: Language;
  source: string;
  verdict: string;
  passed: number;
  total: number;
  created: string;
  result: JudgeResult;
}
