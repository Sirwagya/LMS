export async function api<T = { ok: boolean }>(
  path: string,
  method = "GET",
  data?: unknown,
): Promise<T> {
  const response = await fetch(`/api${path}`, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: data === undefined ? undefined : JSON.stringify(data),
  });
  const result = await response.json();
  if (response.status === 401 && path !== "/login")
    window.dispatchEvent(new Event("lms:session-expired"));
  if (!response.ok)
    throw new Error(result.error || "Request failed. Please try again.");
  return result as T;
}
export function fileBase64(file: File): Promise<string> {
  if (file.size > 10 * 1024 * 1024)
    return Promise.reject(new Error("Choose a file smaller than 10 MB."));
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result).split(",")[1] || "");
    reader.onerror = () => reject(new Error("Could not read file."));
    reader.readAsDataURL(file);
  });
}
