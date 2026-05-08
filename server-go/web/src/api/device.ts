import { api } from "./client";
import type { DeviceFlowInfo } from "../types/api";

export const deviceApi = {
  info: (userCode: string) =>
    api.get<DeviceFlowInfo>(`/api/oauth/device/info?user_code=${encodeURIComponent(userCode)}`),
  approve: (userCode: string) =>
    api.post<{ status: string }>("/api/oauth/device/approve", { user_code: userCode }),
  deny: (userCode: string) =>
    api.post<{ status: string }>("/api/oauth/device/deny", { user_code: userCode }),
};
