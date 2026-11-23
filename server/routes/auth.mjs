import { fail, str } from "../domain/validation.mjs";
import { send, body } from "../http/response.mjs";
import { randomUUID } from "node:crypto";
import { hash, verify } from "../domain/passwords.mjs";
export function createRoutes({ users, sessions, rate, signIn }) {
  return [
    {
      match: ({ path, method }) => path === "/api/login" && method === "POST",
      access: "public",
      handle: async ({ req, res }) => {
        rate(req.socket.remoteAddress);
        const b = await body(req);
        const email = str(b.email, "email", 254).toLowerCase();
        const password = str(b.password, "password", 128);
        const u = users.byEmail(email);
        if (!u || !verify(password, u.password))
          fail(401, "Email or password is incorrect.");
        return signIn(res, u);
      },
    },
    {
      match: ({ path, method }) =>
        path === "/api/register" && method === "POST",
      access: "public",
      handle: async ({ req, res }) => {
        rate(req.socket.remoteAddress);
        const b = await body(req);
        const name = str(b.name, "name", 100),
          email = str(b.email, "email", 254).toLowerCase(),
          password = str(b.password, "password", 128);
        if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email) || password.length < 10)
          fail(
            400,
            "Use a valid email and a password with at least 10 characters.",
          );
        if (users.emailExists(email)) fail(409, "Email already registered.");
        const id = randomUUID();
        users.create(id, email, name, "student", hash(password));
        return signIn(res, users.byId(id));
      },
    },
    {
      match: ({ path, method }) => path === "/api/logout" && method === "POST",
      access: "authenticated",
      handle: async ({ res, u }) => {
        sessions.revoke(u.id);
        return send(
          res,
          200,
          { ok: true },
          {
            "Set-Cookie":
              "lms_session=; HttpOnly; SameSite=Strict; Path=/; Max-Age=0",
          },
        );
      },
    },
    {
      match: ({ path, method }) =>
        path === "/api/password" && method === "POST",
      access: "authenticated",
      handle: async ({ req, res, u }) => {
        const b = await body(req);
        if (!verify(str(b.current, "current password", 128), u.password))
          fail(400, "Current password is incorrect.");
        const next = str(b.next, "new password", 128);
        if (next.length < 10) fail(400, "Use at least 10 characters.");
        users.savePassword(hash(next), u.id);
        sessions.revoke(u.id);
        return signIn(res, u);
      },
    },
  ];
}
