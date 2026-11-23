import { randomUUID } from "node:crypto";
import { hash } from "../domain/passwords.mjs";
export function seedAccounts({ users, attendance, fees }, academic, seed) {
  if (!users.first()) {
    for (const [role, name] of [
      ["admin", "Academic Admin"],
      ["teacher", "Dr. Nandini Kulkarni"],
      ["student", "Sirwagya Shekhar"],
    ]) {
      const id = randomUUID();
      users.create(
        id,
        `${role}@vedam.local`,
        name,
        role,
        hash("VedamLocal123!"),
      );
      if (role === "student") {
        for (const s of Object.values(academic.semesterSubjects).flat())
          attendance.seed(
            id,
            s.code,
            s.attendedClasses || 0,
            s.totalClasses || 0,
          );
        for (const f of seed.feeRecords)
          fees.create(f.id, id, JSON.stringify(f));
      }
    }
  }
}
