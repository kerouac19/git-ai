import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { ApiError } from "../api/client";
import { deviceApi } from "../api/device";
import type { DeviceFlowInfo } from "../types/api";

export default function DeviceFlow() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const userCode = params.get("user_code") ?? "";

  const [info, setInfo] = useState<DeviceFlowInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Fetch device-code info on mount.
  useEffect(() => {
    if (!userCode) {
      setError("Missing user_code in URL.");
      return;
    }
    let cancelled = false;
    deviceApi.info(userCode)
      .then(res => { if (!cancelled) setInfo(res); })
      .catch(err => {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 404) {
          navigate("/oauth/device/result?status=error&reason=not_found", { replace: true });
        } else {
          setError(String(err));
        }
      });
    return () => { cancelled = true; };
  }, [userCode, navigate]);

  // If we resolved info but the user isn't logged in, redirect to /login.
  useEffect(() => {
    if (info && !info.authenticated) {
      const redirect = encodeURIComponent(`/oauth/device?user_code=${userCode}`);
      navigate(`/login?redirect=${redirect}`, { replace: true });
    }
  }, [info, userCode, navigate]);

  async function handle(action: "approve" | "deny") {
    setBusy(true);
    try {
      if (action === "approve") await deviceApi.approve(userCode);
      else await deviceApi.deny(userCode);
      navigate(`/oauth/device/result?status=${action === "approve" ? "ok" : "denied"}`, { replace: true });
    } catch (err) {
      setError(err instanceof ApiError ? `${err.status}: ${err.message}` : String(err));
    } finally {
      setBusy(false);
    }
  }

  if (error) return <main style={{ padding: 24, color: "var(--danger)" }}>{error}</main>;
  if (!info) return <main style={{ padding: 24 }}>Loading…</main>;
  if (!info.authenticated) return null; // redirect effect above will navigate

  return (
    <main style={{ maxWidth: 480, margin: "80px auto", padding: 32, background: "var(--panel)", border: "1px solid var(--border)", borderRadius: 16 }}>
      <h1 style={{ marginTop: 0 }}>Authorize CLI</h1>
      <p style={{ color: "var(--muted)" }}>
        A command-line tool is requesting access as <strong>{info.subject?.name ?? "(unknown)"}</strong>
        {info.subject?.email ? ` (${info.subject.email})` : ""}.
      </p>
      <p style={{ color: "var(--muted)", fontSize: 13 }}>
        Code: <code>{info.user_code}</code> · expires {info.expires_at ?? "n/a"}
      </p>
      <div style={{ display: "flex", gap: 12, marginTop: 24 }}>
        <button onClick={() => handle("approve")} disabled={busy} style={{ flex: 1, padding: 10, background: "var(--accent)", color: "white", border: "none", borderRadius: 8 }}>
          Approve
        </button>
        <button onClick={() => handle("deny")} disabled={busy} style={{ flex: 1, padding: 10, background: "transparent", color: "var(--danger)", border: "1px solid var(--danger)", borderRadius: 8 }}>
          Deny
        </button>
      </div>
    </main>
  );
}
