export class ApiError extends Error {
  status: number;
  body: unknown;
  constructor(status: number, body: unknown, message?: string) {
    super(message ?? `HTTP ${status}`);
    this.status = status;
    this.body = body;
  }
}

function readCookie(name: string): string {
  const match = document.cookie
    .split(";")
    .map(s => s.trim())
    .find(s => s.startsWith(name + "="));
  return match ? decodeURIComponent(match.slice(name.length + 1)) : "";
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  const method = (init?.method ?? "GET").toUpperCase();
  const unsafe = !["GET", "HEAD", "OPTIONS"].includes(method);

  if (unsafe) {
    const token = readCookie("csrf_token");
    if (token) headers.set("X-CSRF-Token", token);
    if (init?.body && !headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }
  }

  const res = await fetch(path, { ...init, credentials: "include", headers });
  const text = await res.text();
  const parsed = text ? safeJSON(text) : null;

  if (!res.ok) {
    throw new ApiError(
      res.status,
      parsed,
      typeof parsed === "object" && parsed && "error" in parsed
        ? String((parsed as { error: unknown }).error)
        : undefined,
    );
  }
  return parsed as T;
}

function safeJSON(s: string): unknown {
  try { return JSON.parse(s); } catch { return s; }
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: "POST", body: body == null ? undefined : JSON.stringify(body) }),
};
