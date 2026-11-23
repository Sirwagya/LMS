import { send } from "../http/response.mjs";
export function createRoutes({ bootstrap }) {
  return [
    {
      match: ({ path, method }) =>
        path === "/api/bootstrap" && method === "GET",
      access: "authenticated",
      handle: async ({ res, u }) => {
        return send(res, 200, bootstrap(u));
      },
    },
  ];
}
