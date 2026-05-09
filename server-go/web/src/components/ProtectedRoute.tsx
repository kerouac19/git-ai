import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { useMe } from "../hooks/useMe";
import type { DashboardStats, User } from "../types/api";

interface Props {
  children: (data: { user: User; dashboard: DashboardStats; org?: { id: string; name: string } }) => ReactNode;
}

import Skeleton from "./Skeleton";

export default function ProtectedRoute({ children }: Props) {
  const state = useMe();
  const location = useLocation();

  if (state.status === "loading") {
    return (
      <div className="page-main">
        <div className="panel" style={{ marginBottom: 24 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 24, marginBottom: 32 }}>
            <Skeleton width={80} height={80} style={{ borderRadius: 20 }} />
            <div style={{ flex: 1 }}>
              <Skeleton width={200} height="2rem" style={{ marginBottom: 8 }} />
              <Skeleton width={150} height="1rem" />
            </div>
          </div>
          <div className="grid">
            <Skeleton height={100} />
            <Skeleton height={100} />
          </div>
        </div>
        <div className="metrics-grid">
          <Skeleton height={140} />
          <Skeleton height={140} />
          <Skeleton height={140} />
          <Skeleton height={140} />
        </div>
      </div>
    );
  }
  if (state.status === "anonymous") {
    const redirect = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?redirect=${redirect}`} replace />;
  }
  if (state.status === "error") {
    return <div style={{ padding: 24, color: "var(--danger)" }}>Error: {state.error.message}</div>;
  }
  return <>{children({ user: state.user, dashboard: state.dashboard, org: state.org })}</>;
}
