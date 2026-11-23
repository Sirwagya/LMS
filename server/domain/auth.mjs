import { fail } from "./validation.mjs";
export function createAuth({ users }) {
  const rates = new Map();
  function rate(key) {
    const now = Date.now();
    let r = rates.get(key);
    if (!r || r.reset < now) r = { n: 0, reset: now + 60000 };
    if (++r.n > 30) fail(429, "Too many attempts. Try again in a minute.");
    rates.set(key, r);
  }

  function student(id) {
    const u = users.findByRole(id, "student");
    if (!u) fail(404, "Student not found.");
    return u;
  }
  function staff(u) {
    if (!["admin", "teacher"].includes(u.role))
      fail(403, "Staff access required.");
  }
  function admin(u) {
    if (u.role !== "admin") fail(403, "Administrator access required.");
  }

  return { rate, student, staff, admin };
}
