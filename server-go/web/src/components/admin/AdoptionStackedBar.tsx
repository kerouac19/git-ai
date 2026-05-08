import {
  Bar, BarChart, CartesianGrid, Legend, ResponsiveContainer,
  Tooltip, XAxis, YAxis,
} from "recharts";
import type { AdminTrendPoint } from "../../types/api";

interface Props {
  data: AdminTrendPoint[];
}

export default function AdoptionStackedBar({ data }: Props) {
  return (
    <div className="card admin-chart">
      <h2>AI 代码采纳趋势</h2>
      {data.length === 0 ? (
        <p className="muted" style={{ margin: 0 }}>暂无数据</p>
      ) : (
        <div style={{ width: "100%", height: 280 }}>
          <ResponsiveContainer>
            <BarChart data={data} margin={{ top: 8, right: 24, left: 0, bottom: 8 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
              <XAxis dataKey="date" tick={{ fontSize: 11 }} />
              <YAxis tick={{ fontSize: 11 }} />
              <Tooltip />
              <Legend />
              <Bar dataKey="generatedAiLines" name="生成"   stackId="a" fill="#a5b4fc" />
              <Bar dataKey="committedAiLines" name="已提交" stackId="a" fill="#6366f1" />
              <Bar dataKey="editedAiLines"    name="人工编辑" stackId="a" fill="#f97316" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}
