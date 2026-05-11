import {
  CartesianGrid, Legend, Line, LineChart, ResponsiveContainer,
  Tooltip, XAxis, YAxis,
} from "recharts";
import type { AdminTrendPoint } from "../../types/api";

interface Props {
  data: AdminTrendPoint[];
}

export default function TrendChart({ data }: Props) {
  return (
    <div className="card admin-chart">
      <h2>使用趋势</h2>
      {data.length === 0 ? (
        <p className="muted" style={{ margin: 0 }}>暂无数据</p>
      ) : (
        <div style={{ width: "100%", height: 280 }}>
          <ResponsiveContainer>
            <LineChart data={data} margin={{ top: 8, right: 24, left: 0, bottom: 8 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
              <XAxis dataKey="date" tick={{ fontSize: 11, fill: "var(--text-muted)" }} axisLine={{ stroke: "var(--border)" }} />
              <YAxis yAxisId="left" tick={{ fontSize: 11, fill: "var(--text-muted)" }} axisLine={{ stroke: "var(--border)" }} />
              <YAxis yAxisId="right" orientation="right" tick={{ fontSize: 11, fill: "var(--text-muted)" }} axisLine={{ stroke: "var(--border)" }} />
              <Tooltip 
                contentStyle={{ background: "var(--bg-card)", borderColor: "var(--border)", borderRadius: "8px", color: "var(--text-main)" }}
                itemStyle={{ fontSize: "12px" }}
              />
              <Legend iconType="circle" />
              <Line
                yAxisId="left"
                type="monotone"
                dataKey="activeUsers"
                name="活跃用户"
                stroke="var(--accent)"
                strokeWidth={2.5}
                dot={false}
                activeDot={{ r: 4, strokeWidth: 0 }}
              />
              <Line
                yAxisId="right"
                type="monotone"
                dataKey="promptCount"
                name="会话数"
                stroke="#22c55e"
                strokeWidth={2.5}
                dot={false}
                activeDot={{ r: 4, strokeWidth: 0 }}
              />
            </LineChart>

          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}
