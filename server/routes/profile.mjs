import { fail, str } from "../domain/validation.mjs";
import { send, body } from "../http/response.mjs";
export function createRoutes({ users }) {
  return [
    {
      match: ({ path, method }) => path === "/api/profile" && method === "PUT",
      access: "authenticated",
      handle: async ({ req, res, u }) => {
        const b = await body(req);
        const profile = b.profile;
        if (!profile || typeof profile !== "object")
          fail(400, "Profile required.");
        const name = str(profile.name, "name", 100);
        for (const key of ["avatarUrl", "bannerUrl"])
          if (
            profile[key] &&
            !/^data:image\/(png|jpeg|webp|gif);base64,/.test(profile[key])
          )
            fail(400, "Upload a PNG, JPEG, WebP or GIF image.");
        for (const p of profile.platforms || [])
          if (p.url && !/^https?:\/\//.test(p.url))
            fail(400, "Profile links must begin with https:// or http://.");
        users.saveProfile(name, JSON.stringify(profile), u.id);
        return send(res, 200, { ok: true });
      },
    },
  ];
}
