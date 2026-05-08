import { FormEvent, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { ApiError } from "../api/client";
import { authApi } from "../api/auth";

function safeRedirect(value: string | null): string {
  if (!value) return "/me";
  // Only allow same-origin absolute paths: starts with single / and is not protocol-relative
  if (!value.startsWith("/") || value.startsWith("//")) return "/me";
  return value;
}

export default function Login() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const redirect = safeRedirect(params.get("redirect"));

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await authApi.login(username, password);
      navigate(redirect, { replace: true });
    } catch (err) {
      if (err instanceof ApiError) {
        setError(
          typeof err.body === "object" && err.body && "message" in err.body
            ? String((err.body as { message: unknown }).message)
            : err.message,
        );
      } else {
        setError(String(err));
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="login__page-main">
      <section className="panel login__panel">
        {/* Icon */}
        <div className="login__icon">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor"
            strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="8" r="4" />
            <path d="M4 20c0-4 3.6-7 8-7s8 3 8 7" />
          </svg>
        </div>

        <h1>登录 Git AI</h1>
        <p className="muted login__subtitle">
          登录后即可访问个人仪表盘和审核 CLI 授权。
        </p>

        <form className="login__form" onSubmit={onSubmit}>
          <label className="login__field">
            <span className="login__field-label">用户名</span>
            <input
              className="login__input"
              value={username}
              onChange={e => setUsername(e.target.value)}
              autoFocus
              required
            />
          </label>
          <label className="login__field">
            <span className="login__field-label">密码</span>
            <input
              className="login__input"
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              required
            />
          </label>
          {error && <div className="login__error">{error}</div>}
          <button
            className="primary login__submit"
            type="submit"
            disabled={submitting}
          >
            {submitting ? "登录中…" : "登录"}
          </button>
        </form>
      </section>
    </main>
  );
}
