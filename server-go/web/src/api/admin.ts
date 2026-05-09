import { api } from "./client";
import type { AdminDashboardResponse, AdminRangeKey } from "../types/api";

export const adminApi = {
  fetchDashboard: (range: AdminRangeKey) =>
    api.get<AdminDashboardResponse>(`/api/dashboard/global?range=${range}`),
};
