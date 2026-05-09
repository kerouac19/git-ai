import { Link, useNavigate } from "react-router-dom";
import ProtectedRoute from "../components/ProtectedRoute";
import { authApi } from "../api/auth";
import type { DashboardStats, User } from "../types/api";

function MeContent({ user, dashboard, org }: { user: User; dashboard: DashboardStats; org?: { id: string; name: string } }) {
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
    <div className="page-main fade-in">
      {/* ── Profile header ───────────────────────────────────── */}
      <div className="panel" style={{ marginBottom: 24 }}>
        <div className="me-page__profile-header" style={{ display: "flex", alignItems: "center", gap: 24, flexWrap: "wrap" }}>
          <div className="me-page__avatar" style={{ 
            width: 80, height: 80, borderRadius: 20, flexShrink: 0,
            display: "flex", alignItems: "center", justifyContent: "center",
            fontSize: "2rem", fontWeight: 800, color: "white"
          }}>
            {initial}
          </div>

          <div className="me-page__header-identity" style={{ flex: 1, minWidth: 200 }}>
            <div className="me-page__header-name-row" style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 4 }}>
              <h1 style={{ margin: 0, fontSize: "1.75rem" }}>{user.name || user.email}</h1>
              <span className="badge badge-accent">{user.role}</span>
            </div>
            <p className="muted" style={{ margin: 0 }}>{user.email}</p>
          </div>

          <div className="me-page__header-status">
            <div className={`me-page__sync-status-badge ${hasSynced ? "synced" : "offline"}`}>
              <span className={`status-dot ${hasSynced ? "online" : "offline"}`} />
              {hasSynced ? "正在同步" : "未连接"}
            </div>
            <p className="me-page__sync-time" style={{ marginTop: 8, fontSize: "0.75rem", textAlign: "right" }}>
              最后同步: {ms?.lastSyncAt ?? "—"}
            </p>
          </div>
        </div>

        {/* ── Org + User ID grid ───────────────────────────────── */}
        <div className="grid" style={{ marginTop: 32 }}>
          <div className="card me-page__org-card" style={{ display: "flex", alignItems: "center", gap: 16 }}>
            <div className="me-page__org-icon">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
                style={{ color: "var(--accent)" }}>
                <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
                <polyline points="9 22 9 12 15 12 15 22" />
              </svg>
            </div>
            <div style={{ overflow: "hidden" }}>
              <h2 style={{ marginBottom: 4 }}>组织架构</h2>
              <p className="me-page__org-name" style={{ 
                margin: 0, fontWeight: 700, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" 
              }}>
                {org?.name ?? "暂无组织"}
              </p>
            </div>
          </div>

          <div className="card" style={{ display: "flex", flexDirection: "column", justifyContent: "center" }}>
            <h2 style={{ marginBottom: 8 }}>用户识别码</h2>
            <p className="me-page__uid-value" style={{ margin: 0 }}>{user.id}</p>
          </div>
        </div>
      </div>

      {/* ── Metrics grid ─────────────────────────────────────── */}
      <div className="metrics-grid" style={{ marginBottom: 24 }}>
        <div className="card">
          <p className="metric-label">AI 代码贡献占比</p>
          <div style={{ display: "flex", alignItems: "baseline", gap: 4 }}>
            <p className="kpi">{aiPct}</p>
            <span className="kpi-unit">%</span>
          </div>
          <div className="me-page__progress-track">
            <div className="me-page__progress-fill" style={{ width: `${Math.min(aiPctNum, 100)}%` }} />
          </div>
          <p className="muted" style={{ fontSize: "0.75rem", margin: 0 }}>
            AI 提交 {ai?.committedAiLines ?? 0} 行 / 总计 {ai?.totalAddedLines ?? 0} 行
          </p>
        </div>

        <div className="card">
          <p className="metric-label">活跃 Prompt 数</p>
          <p className="kpi">{act?.activePromptCount ?? "—"}</p>
          <p className="muted" style={{ fontSize: "0.75rem", margin: "8px 0 0" }}>
            过去 7 天内独立 Prompt 统计
          </p>
        </div>

        <div className="card">
          <p className="metric-label">最常使用 Agent</p>
          <p className="kpi" style={{ fontSize: "1.25rem", margin: "12px 0", height: "2.25rem", display: "flex", alignItems: "center" }}>
            {dashboard.leaders?.topAgent?.label ?? "—"}
          </p>
          <p className="muted" style={{ fontSize: "0.75rem", margin: 0 }}>
            活跃次数: {dashboard.leaders?.topAgent?.promptCount ?? 0}
          </p>
        </div>

        <div className="card">
          <p className="metric-label">常用 AI 模型</p>
          <p className="kpi" style={{ fontSize: "1.25rem", margin: "12px 0", height: "2.25rem", display: "flex", alignItems: "center" }}>
            {dashboard.leaders?.topModel?.label ?? "—"}
          </p>
          <p className="muted" style={{ fontSize: "0.75rem", margin: 0 }}>
            活跃次数: {dashboard.leaders?.topModel?.promptCount ?? 0}
          </p>
        </div>
      </div>

      {/* ── 7-day output & activity ───────────────────────────── */}
      <div className="grid" style={{ marginBottom: 24 }}>
        <div className="card">
          <h2 style={{ marginBottom: 16 }}>AI 输出效能 (7d)</h2>
          <div className="me-page__stat-rows" style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <div className="me-page__stat-row" style={{ display: "flex", justifyContent: "space-between" }}>
              <span className="muted">生成代码行数</span>
              <span style={{ fontWeight: 700 }}>{out?.generated ?? "—"}</span>
            </div>
            <div className="me-page__stat-row" style={{ display: "flex", justifyContent: "space-between" }}>
              <span className="muted">已提交代码行数</span>
              <span style={{ fontWeight: 700 }}>{ai?.committedAiLines ?? "—"}</span>
            </div>
            <div className="me-page__stat-row" style={{ display: "flex", justifyContent: "space-between" }}>
              <span className="muted">人工编辑代码行数</span>
              <span style={{ fontWeight: 700 }}>{out?.edited ?? "—"}</span>
            </div>
          </div>
        </div>

        <div className="card">
          <h2 style={{ marginBottom: 16 }}>开发活跃度 (7d)</h2>
          <div className="me-page__stat-rows" style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <div className="me-page__stat-row" style={{ display: "flex", justifyContent: "space-between" }}>
              <span className="muted">触达文件总数</span>
              <span style={{ fontWeight: 700 }}>{act?.checkpointFileCount ?? "—"}</span>
            </div>
            <div className="me-page__stat-row" style={{ display: "flex", justifyContent: "space-between" }}>
              <span className="muted">涉及代码仓库</span>
              <span style={{ fontWeight: 700 }}>{ms?.repoCount7d ?? "—"}</span>
            </div>
            <div className="me-page__stat-row" style={{ display: "flex", justifyContent: "space-between" }}>
              <span className="muted">同步事件总数</span>
              <span style={{ fontWeight: 700 }}>{ms?.eventCount7d ?? "—"}</span>
            </div>
          </div>
        </div>
      </div>

      {/* ── Today's overview ─────────────────────────────────── */}
      <div className="panel me-page__today-panel" style={{ padding: 24, background: "var(--bg-muted)", borderStyle: "dashed" }}>
        <div className="me-page__today-header" style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
          <h2 style={{ margin: 0 }}>今日动态概览</h2>
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
    </div>
  );
}



export default function Me() {
  return (
    <ProtectedRoute>
      {({ user, dashboard, org }) => <MeContent user={user} dashboard={dashboard} org={org} />}
    </ProtectedRoute>
  );
}
