import { createTransport, type TransportPort } from "./transport";

export type {
  SyncLifecycleState,
  SyncStatus,
  TransportPort,
} from "./transport";

export const transport: TransportPort = createTransport();

export async function api<T = { ok: boolean }>(
  path: string,
  method = "GET",
  data?: unknown,
): Promise<T> {
  return transport.request<T>(path, method, data);
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
