import { useEffect, useState } from "react";
import { Link, Navigate } from "react-router-dom";
import ProtectedRoute from "../components/ProtectedRoute";
import { adminApi } from "../api/admin";
import { ApiError } from "../api/client";
import type { AdminDashboardData, AdminRangeKey, User } from "../types/api";

import RangeToggle from "../components/admin/RangeToggle";
import SummaryCards from "../components/admin/SummaryCards";
import TrendChart from "../components/admin/TrendChart";
import AdoptionStackedBar from "../components/admin/AdoptionStackedBar";
import Leaderboard from "../components/admin/Leaderboard";
import DistributionDonut from "../components/admin/DistributionDonut";

type FetchState =
  | { status: "loading" }
  | { status: "error"; message: string; forbidden?: boolean }
  | { status: "ready"; data: AdminDashboardData };

function AdminActivityContent({ user }: { user: User }) {
  const [range, setRange] = useState<AdminRangeKey>("7d");
  const [tick, setTick] = useState(0);
  const [state, setState] = useState<FetchState>({ status: "loading" });

  useEffect(() => {
    if (user.role !== "admin") return;
    let cancelled = false;
    setState({ status: "loading" });
    adminApi.fetchDashboard(range)
      .then(res => { if (!cancelled) setState({ status: "ready", data: res.data }); })
      .catch(err => {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 403) {
          setState({ status: "error", message: "您没有权限访问此页面。", forbidden: true });
          return;
        }
        const message = err instanceof Error ? err.message : "未知错误";
        setState({ status: "error", message });
      });
    return () => { cancelled = true; };
  }, [range, user.role, tick]);

  if (user.role !== "admin") {
    return <Navigate to="/me" replace />;
  }

  if (state.status === "error" && state.forbidden) {
    return <Navigate to="/me" replace />;
  }

  const rangeLabel = range === "7d" ? "7 天" : "30 天";

  return (
    <main className="page-main admin-page">
      <div className="panel">
        <div className="admin-page__header">
          <div>
            <h1 style={{ margin: 0 }}>平台活跃度看板</h1>
            <p className="muted" style={{ margin: "4px 0 0 0" }}>
              全平台聚合数据 · <Link to="/me">返回个人页</Link>
            </p>
          </div>
          <RangeToggle
            value={range}
            onChange={setRange}
            disabled={state.status === "loading"}
          />
        </div>

        {state.status === "loading" && (
          <p className="muted" style={{ marginTop: 24 }}>加载中…</p>
        )}

        {state.status === "error" && !state.forbidden && (
          <div className="card" style={{ marginTop: 24 }}>
            <p style={{ color: "var(--danger)" }}>加载失败: {state.message}</p>
            <button type="button" onClick={() => setTick(t => t + 1)}>重试</button>
          </div>
        )}

        {state.status === "ready" && (
          <>
            <SummaryCards summary={state.data.summary} rangeLabel={rangeLabel} />

            <div className="admin-page__chart-stack">
              <TrendChart data={state.data.trend} />
              <AdoptionStackedBar data={state.data.trend} />
            </div>

            <div className="grid">
              <Leaderboard
                title="Top 用户"
                rows={state.data.topUsers}
                columns={[
                  { header: "用户", render: (r) => r.name || r.email || r.userId },
                  { header: "Prompt", render: (r) => r.promptCount.toLocaleString(), align: "right" },
                  { header: "AI 行数", render: (r) => r.committedAiLines.toLocaleString(), align: "right" },
                ]}
              />
              <Leaderboard
                title="Top 组织"
                rows={state.data.topOrgs}
                columns={[
                  { header: "组织", render: (r) => r.orgName || r.orgId },
                  { header: "Prompt", render: (r) => r.promptCount.toLocaleString(), align: "right" },
                  { header: "成员", render: (r) => r.memberCount.toLocaleString(), align: "right" },
                ]}
              />
            </div>

            <div className="grid">
              <DistributionDonut title="Agent 分布" rows={state.data.agentDistribution} />
              <DistributionDonut title="模型分布" rows={state.data.modelDistribution} />
            </div>
          </>
        )}
      </div>
    </main>
  );
}

export default function AdminActivity() {
  return (
    <ProtectedRoute>
      {({ user }) => <AdminActivityContent user={user} />}
    </ProtectedRoute>
  );
}
