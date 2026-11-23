import { send, body } from "../http/response.mjs";
export function createRoutes({ codingService }) {
  return [
    {
      match: ({ path, method }) =>
        path === "/api/coding/evaluate" && method === "POST",
      access: "authenticated",
      handle: async ({ req, res, u }) =>
        send(res, 200, await codingService.evaluate(u, await body(req))),
    },
    {
      match: ({ path, method }) =>
        path === "/api/coding/history" && method === "GET",
      access: "authenticated",
      handle: async ({ res, url, u }) =>
        send(
          res,
          200,
          codingService.history(u.id, url.searchParams.get("assignmentId")),
        ),
    },
  ];
}
