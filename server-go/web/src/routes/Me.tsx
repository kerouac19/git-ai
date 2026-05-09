import { Link, useNavigate } from "react-router-dom";
import ProtectedRoute from "../components/ProtectedRoute";
import { authApi } from "../api/auth";
import type { DashboardStats, User } from "../types/api";

function MeContent({ user, dashboard, org }: { user: User; dashboard: DashboardStats; org?: { id: string; name: string } }) {
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
              {hasSynced ? "正在同步" : "未连接"}
            </span>
            <p className="me-page__sync-time">最后同步: {ms?.lastSyncAt ?? "—"}</p>
          </div>
        </div>

        {/* ── Org + User ID grid ───────────────────────────────── */}
        <div className="grid">
          {org ? (
            <div className="card me-page__org-card">
              <h2>组织架构</h2>
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
                  <p className="me-page__org-name">{org.name}</p>
                </div>
              </div>
            </div>
          ) : (
            <div className="card me-page__org-card">
              <h2>组织架构</h2>
              <p className="muted" style={{ margin: 0 }}>暂无组织</p>
            </div>
          )}

          <div className="card">
            <h2>用户识别码</h2>
            <p className="me-page__uid-value">{user.id}</p>
          </div>
        </div>
      </div>

      <Link to="/dashboard" className="card admin-entry-card" style={{ marginTop: 16, display: "block" }}>
        <h2 style={{ margin: 0 }}>团队看板</h2>
        <p className="muted" style={{ margin: "4px 0 0 0" }}>
          查看全平台活跃度统计 →
        </p>
      </Link>

      {/* ── Metrics grid ─────────────────────────────────────── */}
      <div className="metrics-grid" style={{ marginTop: 24 }}>
        {/* AI code contribution % */}
        <div className="card">
          <p className="metric-label">AI 代码贡献占比</p>
          <p className="kpi">
            {aiPct}
            {ai && <span className="kpi-unit">%</span>}
          </p>
          <div className="me-page__progress-track">
            <div className="me-page__progress-fill" style={{ width: `${Math.min(aiPctNum, 100)}%` }} />
          </div>
          <p className="muted" style={{ fontSize: "0.75rem" }}>
            AI 提交 {ai?.committedAiLines ?? 0} 行 / 总计 {ai?.totalAddedLines ?? 0} 行
          </p>
        </div>

        {/* Active prompts */}
        <div className="card">
          <p className="metric-label">活跃 Prompt 数</p>
          <p className="kpi">{act?.activePromptCount ?? "—"}</p>
          <p className="muted" style={{ fontSize: "0.75rem" }}>过去 7 天内独立 Prompt 统计</p>
          <div style={{ marginTop: 16, display: "flex", gap: 8 }}>
            <span className="me-page__prompt-badge">Prompts</span>
          </div>
        </div>

        {/* Top agent */}
        <div className="card">
          <p className="metric-label">最常使用 Agent</p>
          <p className="kpi" style={{ fontSize: "1.5rem", margin: "12px 0" }}>
            {dashboard.leaders?.topAgent?.label ?? "—"}
          </p>
          <p className="muted" style={{ fontSize: "0.75rem" }}>
            活跃次数: {dashboard.leaders?.topAgent?.promptCount ?? 0}
          </p>
        </div>

        {/* Top model */}
        <div className="card">
          <p className="metric-label">常用 AI 模型</p>
          <p className="kpi" style={{ fontSize: "1.5rem", margin: "12px 0" }}>
            {dashboard.leaders?.topModel?.label ?? "—"}
          </p>
          <p className="muted" style={{ fontSize: "0.75rem" }}>
            活跃次数: {dashboard.leaders?.topModel?.promptCount ?? 0}
          </p>
        </div>
      </div>

      {/* ── 7-day output & activity ───────────────────────────── */}
      <div className="card me-page__stats-card" style={{ marginTop: 24 }}>
        <div>
          <h2>AI 输出效能 (7d)</h2>
          <div className="me-page__stat-rows">
            <div className="me-page__stat-row">
              <span className="muted">生成代码行数</span>
              <span style={{ fontWeight: 700 }}>{out?.generated ?? "—"}</span>
            </div>
            <div className="me-page__stat-row">
              <span className="muted">已提交代码行数</span>
              <span style={{ fontWeight: 700 }}>{ai?.committedAiLines ?? "—"}</span>
            </div>
            <div className="me-page__stat-row">
              <span className="muted">人工编辑代码行数</span>
              <span style={{ fontWeight: 700 }}>{out?.edited ?? "—"}</span>
            </div>
          </div>
        </div>
        <div>
          <h2>开发活跃度 (7d)</h2>
          <div className="me-page__stat-rows">
            <div className="me-page__stat-row">
              <span className="muted">触达文件总数</span>
              <span style={{ fontWeight: 700 }}>{act?.checkpointFileCount ?? "—"}</span>
            </div>
            <div className="me-page__stat-row">
              <span className="muted">涉及代码仓库</span>
              <span style={{ fontWeight: 700 }}>{ms?.repoCount7d ?? "—"}</span>
            </div>
            <div className="me-page__stat-row">
              <span className="muted">同步事件总数</span>
              <span style={{ fontWeight: 700 }}>{ms?.eventCount7d ?? "—"}</span>
            </div>
          </div>
        </div>
      </div>

      {/* ── Today's overview ─────────────────────────────────── */}
      <div className="panel me-page__today-panel">
        <div className="me-page__today-header">
          <h2>今日动态概览</h2>
          <span className="muted" style={{ fontSize: "0.75rem" }}>
            更新时间: {today?.lastUpdatedAt ?? "—"}
          </span>
        </div>
        {(today?.activityCount ?? 0) > 0 ? (
          <p style={{ margin: 0 }}>
            今日已有 <strong>{today!.activityCount}</strong> 条活动记录，涵盖 <strong>{today!.promptCount}</strong> 个 Prompt 及 <strong>{today!.fileCount}</strong> 个文件。
          </p>
        ) : (
          <p style={{ margin: 0 }}>今日暂无活跃数据同步。</p>
        )}
      </div>

      {/* ── Sign out ─────────────────────────────────────────── */}
      <div className="actions">
        <button className="secondary" type="button" onClick={onLogout}>
          退出登录
        </button>
      </div>
    </main>
  );
}

export default function Me() {
  return (
    <ProtectedRoute>
      {({ user, dashboard, org }) => <MeContent user={user} dashboard={dashboard} org={org} />}
    </ProtectedRoute>
  );
}
