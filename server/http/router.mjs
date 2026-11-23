import http from "node:http";
import { fail } from "../domain/validation.mjs";
import { send } from "./response.mjs";
import { serveStatic } from "./static.mjs";
// Add features by registering routes; authentication stays in one boundary.
export function createServer({ routes, auth, root }) {
  return http.createServer(async (req, res) => {
    try {
      const url = new URL(req.url, "http://localhost"),
        path = url.pathname,
        method = req.method;
      if (!["GET", "HEAD"].includes(method)) {
        const origin = req.headers.origin;
        if (
          origin &&
          !/^http:\/\/(localhost|127\.0\.0\.1)(:\d+)?$/.test(origin)
        )
          fail(403, "Invalid request origin.");
        if (!(req.headers["content-type"] || "").startsWith("application/json"))
          fail(415, "JSON content type required.");
      }
      if (path.startsWith("/api/")) {
        const ctx = { req, res, url, path, method };
        const route = routes.find((r) => r.match(ctx));
        if (!route) fail(404, "Endpoint not found.");
        if (route.access !== "public") {
          ctx.u = auth.session(req);
          if (!ctx.u) fail(401, "Please sign in.");
          if (route.access === "staff") auth.staff(ctx.u);
        }
        return await route.handle(ctx);
      }
      return serveStatic(root, path, res);
    } catch (e) {
      send(res, e.status || 500, {
        error: e.status ? e.message : "Something went wrong. Please try again.",
      });
      if (!e.status) console.error(e);
    }
  });
}
