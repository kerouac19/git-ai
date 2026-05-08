import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { useMe } from "../hooks/useMe";
import type { DashboardStats, User } from "../types/api";

interface Props {
  children: (data: { user: User; dashboard: DashboardStats }) => ReactNode;
}

export default function ProtectedRoute({ children }: Props) {
  const state = useMe();
  const location = useLocation();

  if (state.status === "loading") {
    return <div style={{ padding: 24 }}>Loading…</div>;
  }
  if (state.status === "anonymous") {
    const redirect = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?redirect=${redirect}`} replace />;
  }
  if (state.status === "error") {
    return <div style={{ padding: 24, color: "var(--danger)" }}>Error: {state.error.message}</div>;
  }
  return <>{children({ user: state.user, dashboard: state.dashboard })}</>;
}
