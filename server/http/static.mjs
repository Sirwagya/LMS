import { existsSync, readFileSync } from "node:fs";
import { resolve, extname } from "node:path";
import { send } from "./response.mjs";
export function serveStatic(root, path, res) {
  // Serve the compiled SPA locally; traversal cannot escape dist.
  const dist = resolve(root, "dist");
  let filename = resolve(dist, "." + decodeURIComponent(path));
  if (!filename.startsWith(dist + "/")) filename = resolve(dist, "index.html");
  if (!existsSync(filename)) filename = resolve(dist, "index.html");
  if (!existsSync(filename))
    return send(res, 503, { error: "Run npm run build first." });
  const mime =
    {
      ".html": "text/html",
      ".js": "text/javascript",
      ".css": "text/css",
      ".png": "image/png",
      ".svg": "image/svg+xml",
      ".ico": "image/x-icon",
    }[extname(filename)] || "application/octet-stream";
  res.writeHead(200, {
    "Content-Type": mime,
    "X-Content-Type-Options": "nosniff",
  });
  res.end(readFileSync(filename));
}
