import { createContext, useContext } from "react";
export interface PortalUser {
  id: string;
  email: string;
  name: string;
  role: "admin" | "teacher" | "student";
  profile: Record<string, unknown>;
}
export interface Submission {
  id: string;
  user_id?: string;
  name?: string;
  assignment_id: string;
  course: string;
  answer: string;
  filename: string;
  score: string | null;
  feedback: string | null;
  created: string;
}
export interface Discussion {
  id: string;
  name: string;
  body: string;
  created: string;
}
export const SessionContext = createContext<{
  user: PortalUser;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
  submissions: Submission[];
  progress: number[];
} | null>(null);
export function useSession() {
  const s = useContext(SessionContext);
  if (!s) throw new Error("Session required");
  return s;
}
