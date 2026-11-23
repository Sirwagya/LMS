import { send } from "../http/response.mjs";
import { publicUser } from "../domain/passwords.mjs";
export function createRoutes({ users, attendance, submissions, fees }) {
  return [
    {
      match: ({ path, method }) => path === "/api/staff" && method === "GET",
      access: "staff",
      handle: async ({ res }) => {
        return send(res, 200, {
          users: users.all().map(publicUser),
          submissions: submissions.all(),
          attendance: attendance.all(),
          fees: fees
            .all()
            .map((f) => ({ ...JSON.parse(f.data), userId: f.user_id })),
        });
      },
    },
  ];
}
