import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import ProtectedRoute from "../components/ProtectedRoute";
import { adminApi } from "../api/admin";
import type { AdminDashboardData, AdminRangeKey } from "../types/api";

import RangeToggle from "../components/admin/RangeToggle";
import SummaryCards from "../components/admin/SummaryCards";
import TrendChart from "../components/admin/TrendChart";
import AdoptionStackedBar from "../components/admin/AdoptionStackedBar";
import Leaderboard from "../components/admin/Leaderboard";
import DistributionDonut from "../components/admin/DistributionDonut";

type FetchState =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "ready"; data: AdminDashboardData };

function DashboardContent() {
  const [range, setRange] = useState<AdminRangeKey>("7d");
  const [tick, setTick] = useState(0);
  const [state, setState] = useState<FetchState>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    setState({ status: "loading" });
    adminApi.fetchDashboard(range)
      .then(res => { if (!cancelled) setState({ status: "ready", data: res.data }); })
      .catch(err => {
        if (cancelled) return;
        const message = err instanceof Error ? err.message : "未知错误";
        setState({ status: "error", message });
      });
    return () => { cancelled = true; };
  }, [range, tick]);

  const rangeLabel = range === "7d" ? "7 天" : "30 天";

  return (
    <main className="page-main admin-page">
      <div className="panel">
        <div className="admin-page__header">
          <div>
            <h1 style={{ margin: 0 }}>团队看板</h1>
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

        {state.status === "error" && (
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

export default function Dashboard() {
  return (
    <ProtectedRoute>
      {() => <DashboardContent />}
    </ProtectedRoute>
  );
}
