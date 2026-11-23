import { randomBytes, scryptSync, timingSafeEqual } from "node:crypto";
export const hash = (password) => {
  const salt = randomBytes(16).toString("hex");
  return salt + ":" + scryptSync(password, salt, 64).toString("hex");
};
export const verify = (password, stored) => {
  const [salt, h] = stored.split(":");
  return timingSafeEqual(scryptSync(password, salt, 64), Buffer.from(h, "hex"));
};
export const publicUser = (u) => ({
  id: u.id,
  email: u.email,
  name: u.name,
  role: u.role,
  profile: JSON.parse(u.profile),
});
