import { useSearchParams } from "react-router-dom";

type Tone = "ok" | "error";

const COPY: Record<string, { title: string; message: string; tone: Tone; linkURL?: string; linkText?: string }> = {
  ok: {
    title: "授权成功",
    message: "CLI 授权已完成，您可以返回终端继续操作。",
    tone: "ok",
    linkURL: "/me",
    linkText: "打开仪表盘",
  },
  denied: {
    title: "授权已拒绝",
    message: "CLI 授权请求已被拒绝，您可以关闭此标签页。",
    tone: "error",
  },
  error: {
    title: "授权错误",
    message: "设备代码处理失败，请重试。",
    tone: "error",
  },
};

const ICONS: Record<Tone, React.ReactNode> = {
  ok: (
    <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="20 6 9 17 4 12" />
    </svg>
  ),
  error: (
    <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round">
      <line x1="18" y1="6" x2="6" y2="18" />
      <line x1="6" y1="6" x2="18" y2="18" />
    </svg>
  ),
};

export default function DeviceResult() {
  const [params] = useSearchParams();
  const status = params.get("status") ?? "error";
  const c = COPY[status] ?? COPY.error;

  return (
    <main className="device-result__page-main">
      <section className="panel device-result__panel">
        <p className="muted">Git AI device authorization</p>

        <div className={`device-result__icon ${c.tone}`}>
          {ICONS[c.tone]}
        </div>

        <h1 className={`device-result__title ${c.tone}`}>{c.title}</h1>

        <div className={`notice ${c.tone} device-result__notice`} style={{ justifyContent: "center" }}>
          {c.message}
        </div>

        {c.linkURL && c.linkText && (
          <div className="actions device-result__actions">
            <a className="button primary" href={c.linkURL}>{c.linkText}</a>
          </div>
        )}
      </section>
    </main>
  );
}
