import type { ReactNode } from "react";

interface Column<T> {
  header: string;
  render: (row: T) => ReactNode;
  align?: "left" | "right";
}

interface Props<T> {
  title: string;
  rows: T[];
  columns: Column<T>[];
  emptyMessage?: string;
}

export default function Leaderboard<T>({ title, rows, columns, emptyMessage }: Props<T>) {
  return (
    <div className="card admin-leaderboard">
      <h2>{title}</h2>
      {rows.length === 0 ? (
        <p className="muted" style={{ margin: 0 }}>{emptyMessage ?? "暂无数据"}</p>
      ) : (
        <table className="admin-leaderboard__table">
          <thead>
            <tr>
              {columns.map((c, i) => (
                <th key={i} style={{ textAlign: c.align ?? "left" }}>{c.header}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => (
              <tr key={i}>
                {columns.map((c, j) => (
                  <td key={j} style={{ textAlign: c.align ?? "left" }}>{c.render(row)}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
