import { fail } from "../domain/validation.mjs";
export async function body(req) {
  let size = 0;
  const chunks = [];
  for await (const c of req) {
    size += c.length;
    if (size > 16 * 1024 * 1024)
      fail(413, "Upload too large. Maximum request size is 16 MB.");
    chunks.push(c);
  }
  try {
    return JSON.parse(Buffer.concat(chunks).toString() || "{}");
  } catch {
    fail(400, "Invalid JSON.");
  }
}
export function send(res, status, data, headers = {}) {
  res.writeHead(status, {
    "Content-Type": "application/json",
    "Cache-Control": "no-store",
    "X-Content-Type-Options": "nosniff",
    ...headers,
  });
  res.end(JSON.stringify(data));
}
export function file(
  res,
  bytes,
  name,
  mime = "application/octet-stream",
  download = true,
) {
  res.writeHead(200, {
    "Content-Type": mime,
    "Content-Disposition": `${download ? "attachment" : "inline"}; filename="${name.replace(/[^a-zA-Z0-9._ -]/g, "_")}"`,
    "X-Content-Type-Options": "nosniff",
    "Cache-Control": "private, no-store",
  });
  res.end(bytes);
}
