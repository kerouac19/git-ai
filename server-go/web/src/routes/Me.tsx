import { useNavigate } from "react-router-dom";
import ProtectedRoute from "../components/ProtectedRoute";
import { authApi } from "../api/auth";
import type { DashboardStats, User } from "../types/api";

function MeContent({ user, dashboard }: { user: User; dashboard: DashboardStats }) {
  const navigate = useNavigate();

  async function onLogout() {
    try { await authApi.logout(); } catch { /* ignore */ }
    navigate("/login", { replace: true });
  }

  const ai    = dashboard.aiCode;
  const today = dashboard.today;
  const ms    = dashboard.metricsSummary;
  const out   = dashboard.aiOutput;
  const act   = dashboard.activity;

  const org     = user.orgs?.[0];
  const initial = user.name?.[0]?.toUpperCase() ?? "?";

  const hasSynced = !!ms?.lastSyncAt;

  const aiPct = ai ? ai.percentage.toFixed(1) : "—";
  const aiPctNum = ai ? ai.percentage : 0;

  return (
    <main className="page-main">
      {/* ── Profile header ───────────────────────────────────── */}
      <div className="panel">
        <div className="me-page__profile-header">
          <div className="me-page__avatar">{initial}</div>

          <div className="me-page__header-identity">
            <div className="me-page__header-name-row">
              <h1>{user.name || user.email}</h1>
              <span className="badge badge-accent">{user.role}</span>
            </div>
            <p className="muted" style={{ marginBottom: 0 }}>{user.email}</p>
          </div>

          <div className="me-page__header-status">
            <span className={`me-page__sync-status-badge ${hasSynced ? "synced" : "offline"}`}>
              <span className={`status-dot ${hasSynced ? "online" : "offline"}`}></span>
              {hasSynced ? "Syncing" : "Not connected"}
            </span>
            <p className="me-page__sync-time">Last sync: {ms?.lastSyncAt ?? "—"}</p>
          </div>
        </div>

        {/* ── Org + User ID grid ───────────────────────────────── */}
        <div className="grid">
          {org ? (
            <div className="card me-page__org-card">
              <h2>Organization</h2>
              <div className="me-page__org-info">
                <div className="me-page__org-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                    strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
                    style={{ color: "var(--accent)" }}>
                    <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
                    <polyline points="9 22 9 12 15 12 15 22" />
                  </svg>
                </div>
                <div>
                  <p className="me-page__org-name">{org.org_name}</p>
                  <p className="me-page__org-slug">Slug: {org.org_slug}</p>
                </div>
              </div>
            </div>
          ) : (
            <div className="card me-page__org-card">
              <h2>Organization</h2>
              <p className="muted" style={{ margin: 0 }}>No organization</p>
            </div>
          )}

          <div className="card">
            <h2>User ID</h2>
            <p className="me-page__uid-value">{user.id}</p>
          </div>
        </div>
      </div>

      {/* ── Metrics grid ─────────────────────────────────────── */}
      <div className="metrics-grid" style={{ marginTop: 24 }}>
        {/* AI code contribution % */}
        <div className="card">
          <p className="metric-label">AI Code Contribution</p>
          <p className="kpi">
            {aiPct}
            {ai && <span className="kpi-unit">%</span>}
          </p>
          <div className="me-page__progress-track">
            <div className="me-page__progress-fill" style={{ width: `${Math.min(aiPctNum, 100)}%` }} />
          </div>
          <p className="muted" style={{ fontSize: "0.75rem" }}>
            AI committed {ai?.committedAiLines ?? 0} lines / total {ai?.totalAddedLines ?? 0} lines
          </p>
        </div>

        {/* Active prompts */}
        <div className="card">
          <p className="metric-label">Active Prompts</p>
          <p className="kpi">{act?.activePromptCount ?? "—"}</p>
          <p className="muted" style={{ fontSize: "0.75rem" }}>Independent prompts in the past 7 days</p>
          <div style={{ marginTop: 16, display: "flex", gap: 8 }}>
            <span className="me-page__prompt-badge">Prompts</span>
          </div>
        </div>

        {/* Top agent */}
        <div className="card">
          <p className="metric-label">Top Agent</p>
          <p className="kpi" style={{ fontSize: "1.5rem", margin: "12px 0" }}>
            {dashboard.leaders?.topAgent?.label ?? "—"}
          </p>
          <p className="muted" style={{ fontSize: "0.75rem" }}>
            Active count: {dashboard.leaders?.topAgent?.promptCount ?? 0}
          </p>
        </div>

        {/* Top model */}
        <div className="card">
          <p className="metric-label">Top AI Model</p>
          <p className="kpi" style={{ fontSize: "1.5rem", margin: "12px 0" }}>
            {dashboard.leaders?.topModel?.label ?? "—"}
          </p>
          <p className="muted" style={{ fontSize: "0.75rem" }}>
            Active count: {dashboard.leaders?.topModel?.promptCount ?? 0}
          </p>
        </div>
      </div>

      {/* ── 7-day output & activity ───────────────────────────── */}
      <div className="card me-page__stats-card" style={{ marginTop: 24 }}>
        <div>
          <h2>AI Output Efficiency (7d)</h2>
          <div className="me-page__stat-rows">
            <div className="me-page__stat-row">
              <span className="muted">Generated lines</span>
              <span style={{ fontWeight: 700 }}>{out?.generated ?? "—"}</span>
            </div>
            <div className="me-page__stat-row">
              <span className="muted">Committed lines</span>
              <span style={{ fontWeight: 700 }}>{ai?.committedAiLines ?? "—"}</span>
            </div>
            <div className="me-page__stat-row">
              <span className="muted">Human-edited lines</span>
              <span style={{ fontWeight: 700 }}>{out?.edited ?? "—"}</span>
            </div>
          </div>
        </div>
        <div>
          <h2>Development Activity (7d)</h2>
          <div className="me-page__stat-rows">
            <div className="me-page__stat-row">
              <span className="muted">Files touched</span>
              <span style={{ fontWeight: 700 }}>{act?.checkpointFileCount ?? "—"}</span>
            </div>
            <div className="me-page__stat-row">
              <span className="muted">Repos involved</span>
              <span style={{ fontWeight: 700 }}>{ms?.repoCount7d ?? "—"}</span>
            </div>
            <div className="me-page__stat-row">
              <span className="muted">Sync events</span>
              <span style={{ fontWeight: 700 }}>{ms?.eventCount7d ?? "—"}</span>
            </div>
          </div>
        </div>
      </div>

      {/* ── Today's overview ─────────────────────────────────── */}
      <div className="panel me-page__today-panel">
        <div className="me-page__today-header">
          <h2>Today's Overview</h2>
          <span className="muted" style={{ fontSize: "0.75rem" }}>
            Updated: {today?.lastUpdatedAt ?? "—"}
          </span>
        </div>
        {(today?.activityCount ?? 0) > 0 ? (
          <p style={{ margin: 0 }}>
            Today there are <strong>{today!.activityCount}</strong> activity records,
            covering <strong>{today!.promptCount}</strong> prompts
            and <strong>{today!.fileCount}</strong> files.
          </p>
        ) : (
          <p style={{ margin: 0 }}>No active data synced today.</p>
        )}
      </div>

      {/* ── Sign out ─────────────────────────────────────────── */}
      <div className="actions">
        <button className="secondary" type="button" onClick={onLogout}>
          Sign out
        </button>
      </div>
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
