import { randomBytes } from "node:crypto";
import { publicUser } from "../domain/passwords.mjs";
import { send } from "./response.mjs";
export function createSessionTransport({ sessions }) {
  function session(req) {
    const t = (req.headers.cookie || "")
      .split(";")
      .map((s) => s.trim())
      .find((s) => s.startsWith("lms_session="))
      ?.slice(12);
    const row = t && sessions.userForToken(t, Date.now());
    return row;
  }
  function signIn(res, u) {
    const t = randomBytes(32).toString("hex");
    sessions.create(t, u.id, Date.now() + 86400000);
    send(
      res,
      200,
      { user: publicUser(u) },
      {
        "Set-Cookie": `lms_session=${t}; HttpOnly; SameSite=Strict; Path=/; Max-Age=86400`,
      },
    );
  }

  return { session, signIn };
}
