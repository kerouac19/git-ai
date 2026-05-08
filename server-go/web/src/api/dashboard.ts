import { api } from "./client";
import type { DashboardStats } from "../types/api";

export const dashboardApi = {
  stats: () => api.get<DashboardStats>("/api/dashboard/stats"),
};
