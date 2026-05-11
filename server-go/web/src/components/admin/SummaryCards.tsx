import type { AdminDashboardSummary } from "../../types/api";

interface Props {
  summary: AdminDashboardSummary;
  rangeLabel: string;
}

interface Tile {
  label: string;
  value: string;
  hint?: string;
}

export default function SummaryCards({ summary, rangeLabel }: Props) {
  const tiles: Tile[] = [
    {
      label: "今日活跃用户",
      value: String(summary.activeUsersToday),
    },
    {
      label: `${rangeLabel}活跃用户`,
      value: String(summary.activeUsersInRange),
    },
    {
      label: `${rangeLabel}总会话数`,
      value: summary.totalPrompts.toLocaleString(),
    },
    {
      label: `${rangeLabel}总 Checkpoint`,
      value: summary.totalCheckpoints.toLocaleString(),
    },
    {
      label: "AI 代码采纳率",
      value: `${summary.aiCodePercentage.toFixed(1)}%`,
    },
  ];

  return (
    <div className="metrics-grid admin-summary">
      {tiles.map(t => (
        <div className="card" key={t.label}>
          <p className="metric-label">{t.label}</p>
          <p className="kpi" style={{ fontSize: "1.75rem" }}>{t.value}</p>
        </div>
      ))}
    </div>
  );
}
