import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";
import type { AdminDistributionRow } from "../../types/api";

interface Props {
  title: string;
  rows: AdminDistributionRow[];
}

const PALETTE = ["#6366f1", "#22c55e", "#f97316", "#06b6d4", "#a855f7", "#eab308", "#ef4444", "#14b8a6"];

export default function DistributionDonut({ title, rows }: Props) {
  return (
    <div className="card admin-donut">
      <h2>{title}</h2>
      {rows.length === 0 ? (
        <p className="muted" style={{ margin: 0 }}>暂无数据</p>
      ) : (
        <div style={{ width: "100%", height: 240 }}>
          <ResponsiveContainer>
            <PieChart>
              <Pie
                data={rows}
                dataKey="promptCount"
                nameKey="label"
                innerRadius={50}
                outerRadius={90}
                paddingAngle={2}
              >
                {rows.map((_, i) => (
                  <Cell key={i} fill={PALETTE[i % PALETTE.length]} />
                ))}
              </Pie>
              <Tooltip
                formatter={(value: number, _name: string | number, item: { payload?: { share?: number; label?: string } }) => {
                  const share = (item?.payload?.share ?? 0) * 100;
                  return [`${value} (${share.toFixed(1)}%)`, item?.payload?.label];
                }}
              />
            </PieChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}
