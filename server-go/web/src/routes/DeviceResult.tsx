import { useSearchParams } from "react-router-dom";

type Tone = "ok" | "error";

const COPY: Record<string, { title: string; message: string; tone: Tone; linkURL?: string; linkText?: string }> = {
  ok: {
    title: "Device Approved",
    message: "CLI authorization completed. You can return to your terminal.",
    tone: "ok",
    linkURL: "/me",
    linkText: "Open Dashboard",
  },
  denied: {
    title: "Device Denied",
    message: "CLI authorization was denied. You can close this tab.",
    tone: "error",
  },
  error: {
    title: "Authorization Error",
    message: "The device code could not be processed.",
    tone: "error",
  },
};

const ICONS: Record<Tone, string> = {
  ok:    "✓",
  error: "✕",
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

        <div className={`notice ${c.tone}`} style={{ justifyContent: "center" }}>
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
