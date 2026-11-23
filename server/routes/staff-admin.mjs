import { fail, str, num } from "../domain/validation.mjs";
import { send, body } from "../http/response.mjs";
import { randomUUID } from "node:crypto";
import { hash } from "../domain/passwords.mjs";
export function createRoutes({ users, fees, student, admin }) {
  return [
    {
      match: ({ path, method }) =>
        path === "/api/staff/users" && method === "POST",
      access: "staff",
      handle: async ({ req, res, u }) => {
        const b = await body(req);
        admin(u);
        const name = str(b.name, "name", 100),
          email = str(b.email, "email", 254).toLowerCase(),
          password = str(b.password, "password", 128);
        if (
          !["student", "teacher"].includes(b.role) ||
          !/^\S+@\S+\.\S+$/.test(email) ||
          password.length < 10
        )
          fail(400, "Valid email, role and a 10-character password required.");
        if (users.emailExists(email)) fail(409, "Email already registered.");
        users.create(randomUUID(), email, name, b.role, hash(password));
        return send(res, 201, { ok: true });
      },
    },
    {
      match: ({ path, method }) =>
        path === "/api/staff/fees" && method === "POST",
      access: "staff",
      handle: async ({ req, res, u }) => {
        const b = await body(req);
        admin(u);
        student(b.userId);
        const id = randomUUID(),
          amount = num(b.amount, "amount", 1);
        const date = str(b.dueDate, "due date", 10);
        if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || Number.isNaN(Date.parse(date)))
          fail(400, "Use a valid date.");
        const data = {
          id,
          invoiceNo: `LOCAL-${Date.now()}`,
          description: str(b.description, "description", 200),
          semester: `Semester ${num(b.semester, "semester", 1, 3)}`,
          amount,
          dueDate: date,
          status:
            new Date(date + "T23:59:59") < new Date() ? "Overdue" : "Pending",
          receiptAvailable: false,
        };
        fees.create(id, b.userId, JSON.stringify(data));
        return send(res, 201, { ok: true });
      },
    },
  ];
}
