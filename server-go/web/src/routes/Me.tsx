import { useNavigate } from "react-router-dom";
import ProtectedRoute from "../components/ProtectedRoute";
import Stat from "../components/Stat";
import { authApi } from "../api/auth";
import type { DashboardStats, User } from "../types/api";

function MeContent({ user, dashboard }: { user: User; dashboard: DashboardStats }) {
  const navigate = useNavigate();

  async function onLogout() {
    try { await authApi.logout(); } catch { /* ignore */ }
    navigate("/login", { replace: true });
  }

  const ai = dashboard.aiCode;
  const today = dashboard.today;
  const ms = dashboard.metricsSummary;

  return (
    <main style={{ maxWidth: 1000, margin: "0 auto", padding: "40px 20px 80px" }}>
      <header style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 32 }}>
        <div>
          <h1 style={{ fontSize: 28, marginBottom: 4 }}>Hi, {user.name || user.email}</h1>
          <div style={{ color: "var(--muted)" }}>{user.email} · {user.role}</div>
        </div>
        <button onClick={onLogout} style={{ padding: "8px 14px", background: "transparent", border: "1px solid var(--border)", borderRadius: 8 }}>
          Sign out
        </button>
      </header>

      <section style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 16, marginBottom: 32 }}>
        <Stat label="AI code %" value={ai ? `${ai.percentage.toFixed(1)}%` : "—"} hint={ai ? `${ai.committedAiLines}/${ai.totalAddedLines} lines` : undefined} />
        <Stat label="Today activity" value={today?.activityCount ?? "—"} hint={today?.lastUpdatedAt ? `last: ${today.lastUpdatedAt}` : undefined} />
        <Stat label="Today prompts" value={today?.promptCount ?? "—"} />
        <Stat label="7d events" value={ms?.eventCount7d ?? "—"} hint={ms?.repoCount7d != null ? `${ms.repoCount7d} repos` : undefined} />
      </section>

      <section style={{ background: "var(--panel)", border: "1px solid var(--border)", borderRadius: 12, padding: 20 }}>
        <h2 style={{ fontSize: 14, color: "var(--muted)", textTransform: "uppercase", letterSpacing: 0.5, marginTop: 0 }}>Top agent / model</h2>
        <div style={{ display: "flex", gap: 24, flexWrap: "wrap" }}>
          <div>
            <div style={{ color: "var(--muted)", fontSize: 13 }}>Agent</div>
            <div style={{ fontWeight: 600 }}>{dashboard.leaders?.topAgent?.label ?? "—"}</div>
            <div style={{ color: "var(--muted)", fontSize: 13 }}>{dashboard.leaders?.topAgent?.promptCount ?? 0} prompts</div>
          </div>
          <div>
            <div style={{ color: "var(--muted)", fontSize: 13 }}>Model</div>
            <div style={{ fontWeight: 600 }}>{dashboard.leaders?.topModel?.label ?? "—"}</div>
            <div style={{ color: "var(--muted)", fontSize: 13 }}>{dashboard.leaders?.topModel?.promptCount ?? 0} prompts</div>
          </div>
        </div>
      </section>
    </main>
  );
}

export default function Me() {
  return (
    <ProtectedRoute>
      {({ user, dashboard }) => <MeContent user={user} dashboard={dashboard} />}
    </ProtectedRoute>
  );
}
