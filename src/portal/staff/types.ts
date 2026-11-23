import type { PortalUser, Submission } from "../session";
export interface StaffData {
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
