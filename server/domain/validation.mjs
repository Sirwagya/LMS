export const fail = (status, message) => {
  throw Object.assign(new Error(message), { status });
};
export const str = (v, label, max = 5000) => {
  if (typeof v !== "string" || !v.trim() || v.length > max)
    fail(400, `Valid ${label} required (max ${max} characters).`);
  return v.trim();
};
export const num = (v, label, min = 0, max = 1e7) => {
  if (!Number.isFinite(v) || v < min || v > max) fail(400, `Invalid ${label}.`);
  return v;
};
