import { send } from "../http/response.mjs";
export function createRoutes() {
  return [
    {
      match: ({ path }) => path === "/api/health",
      access: "public",
      handle: async ({ res }) => {
        return send(res, 200, { ok: true, mode: "local" });
      },
    },
  ];
}
