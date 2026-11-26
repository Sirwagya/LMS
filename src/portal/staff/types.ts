import type { PortalUser, Submission } from "../session";
export type StaffRecords = "users" | "submissions" | "attendance" | "fees";
export interface StaffData {
  hasMore: Record<StaffRecords, boolean>;
  users: PortalUser[];
  submissions: Submission[];
  attendance: {
    user_id: string;
    course: string;
    attended: number;
    total: number;
  }[];
  fees: {
    id: string;
    userId: string;
    description: string;
    amount: number;
    status: string;
  }[];
}
