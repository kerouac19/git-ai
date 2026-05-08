import { Link, useSearchParams } from "react-router-dom";

const COPY: Record<string, { title: string; message: string; tone: "ok" | "error" }> = {
  ok: { title: "Device approved", message: "CLI authorization completed. You can return to your terminal.", tone: "ok" },
  denied: { title: "Device denied", message: "CLI authorization was denied. You can close this tab.", tone: "error" },
  error: { title: "Authorization error", message: "The device code could not be processed.", tone: "error" },
};

export default function DeviceResult() {
  const [params] = useSearchParams();
  const status = params.get("status") ?? "error";
  const c = COPY[status] ?? COPY.error;

  return (
    <main style={{ maxWidth: 480, margin: "80px auto", padding: 32, textAlign: "center", background: "var(--panel)", border: "1px solid var(--border)", borderRadius: 16 }}>
      <h1 style={{ color: c.tone === "error" ? "var(--danger)" : "var(--success)", marginTop: 0 }}>{c.title}</h1>
      <p style={{ color: "var(--muted)" }}>{c.message}</p>
      <Link to="/me" style={{ display: "inline-block", marginTop: 16 }}>Open dashboard</Link>
    </main>
  );
}
