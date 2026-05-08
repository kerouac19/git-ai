import { useEffect, useState } from "react";
import { api, ApiError } from "../api/client";
import type { DashboardStats, MeApiResponse, User } from "../types/api";

type State =
  | { status: "loading" }
  | { status: "authenticated"; user: User; dashboard: DashboardStats }
  | { status: "anonymous" }
  | { status: "error"; error: Error };

export function useMe(): State {
  const [state, setState] = useState<State>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    api.get<MeApiResponse>("/api/me")
      .then(res => {
        if (cancelled) return;
        setState({ status: "authenticated", user: res.user, dashboard: res.dashboard });
      })
      .catch(err => {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 401) {
          setState({ status: "anonymous" });
        } else {
          setState({ status: "error", error: err });
        }
      });
    return () => { cancelled = true; };
  }, []);

  return state;
}
