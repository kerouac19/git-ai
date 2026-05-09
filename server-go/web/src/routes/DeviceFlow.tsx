import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { ApiError } from "../api/client";
import { deviceApi } from "../api/device";
import type { DeviceFlowInfo } from "../types/api";
import { formatLocalDateTime } from "../utils/datetime";

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

  if (error) {
    return (
      <main className="page-main">
        <section className="panel">
          <div className="notice error">{error}</div>
        </section>
      </main>
    );
  }

  if (!info) {
    return (
      <main className="page-main">
        <section className="panel">
          <p className="muted">加载中…</p>
        </section>
      </main>
    );
  }

  if (!info.authenticated) return null; // redirect effect above will navigate

  const isPending  = info.status === "pending";
  const isApproved = info.status === "approved";
  const isDenied   = info.status === "denied";

  return (
    <main className="page-main">
      <section className="panel">
        <p className="muted device-flow__subtitle">Git AI device authorization</p>
        <h1>审核命令行授权</h1>
        <p>
          为 <strong>{info.subject?.name ?? "(unknown)"}</strong>
          {info.subject?.email ? `（${info.subject.email}）` : ""} 授权待处理的 CLI 登录请求。
        </p>

        <div className="grid" style={{ marginTop: 24 }}>
          <div className="card">
            <h2>用户代码</h2>
            <p><code className="device-flow__code-value">{info.user_code}</code></p>
          </div>
          <div className="card">
            <h2>过期时间</h2>
            <p>{formatLocalDateTime(info.expires_at)}</p>
          </div>
          <div className="card">
            <h2>状态</h2>
            <p>{info.status}</p>
          </div>
        </div>

        {isApproved && (
          <div className="notice ok">该设备已通过授权。</div>
        )}
        {isDenied && (
          <div className="notice error">该设备授权请求已被拒绝。</div>
        )}

        {isPending ? (
          <div className="actions">
            <button className="primary" type="button" disabled={busy} onClick={() => handle("approve")}>
              同意
            </button>
            <button className="secondary" type="button" disabled={busy} onClick={() => handle("deny")}>
              拒绝
            </button>
          </div>
        ) : (
          <div className="actions">
            <a className="button primary" href="/me">打开仪表盘</a>
          </div>
        )}
      </section>
    </main>
  );
}
