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

import Skeleton from "../components/Skeleton";

function DashboardSkeleton() {
  return (
    <div className="page-main admin-page">
      <div className="admin-page__header" style={{ marginBottom: 24 }}>
        <div>
          <Skeleton width={120} height="2rem" style={{ marginBottom: 8 }} />
          <Skeleton width={300} height="1rem" />
        </div>
        <Skeleton width={140} height="2.5rem" />
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: 24 }}>
        <div className="metrics-grid">
          {[1, 2, 3, 4, 5].map(i => (
            <div className="card" key={i}>
              <Skeleton width="60%" style={{ marginBottom: 12 }} />
              <Skeleton width="40%" height="2rem" />
            </div>
          ))}
        </div>

        <div className="admin-page__chart-stack" style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(400px, 1fr))", gap: 24 }}>
          <div className="card"><Skeleton height={280} /></div>
          <div className="card"><Skeleton height={280} /></div>
        </div>

        <div className="grid">
          <div className="card"><Skeleton height={200} /></div>
          <div className="card"><Skeleton height={200} /></div>
        </div>
      </div>
    </div>
  );
}

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

  if (state.status === "loading") {
    return <DashboardSkeleton />;
  }

  return (
    <div className="page-main admin-page fade-in">
      <div className="admin-page__header" style={{ marginBottom: 24 }}>
        <div>
          <h1 style={{ margin: 0 }}>团队看板</h1>
          <p className="muted" style={{ margin: "4px 0 0 0" }}>
            聚合全平台开发者活跃度与 AI 采纳数据
          </p>
        </div>
        <RangeToggle
          value={range}
          onChange={setRange}
        />
      </div>

      {state.status === "error" && (
        <div className="card" style={{ textAlign: "center", padding: "48px 0" }}>
          <p style={{ color: "var(--danger)", marginBottom: 16 }}>加载失败: {state.message}</p>
          <button type="button" className="primary" onClick={() => setTick(t => t + 1)}>重试</button>
        </div>
      )}

      {state.status === "ready" && (
        <div style={{ display: "flex", flexDirection: "column", gap: 24 }}>
          <SummaryCards summary={state.data.summary} rangeLabel={rangeLabel} />

          <div className="admin-page__chart-stack" style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(400px, 1fr))", gap: 24 }}>
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
        </div>
      )}
    </div>
  );
}



export default function Dashboard() {
  return (
    <ProtectedRoute>
      {() => <DashboardContent />}
    </ProtectedRoute>
  );
}
