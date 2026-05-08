import { api } from "./client";

interface LoginResponseData {
  id: string;
  username: string;
  display_name: string;
  role: string;
  status: string;
}

export interface LoginResponse {
  success: boolean;
  message: string;
  data: LoginResponseData;
  access_token: string;
  csrf_token: string;
}

export const authApi = {
  login: (username: string, password: string) =>
    api.post<LoginResponse>("/api/user/login", { username, password }),
  logout: () => api.post<{ success: boolean }>("/api/user/logout"),
};
