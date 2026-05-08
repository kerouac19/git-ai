export interface User {
  id: string;
  email: string;
  name: string;
  role: string;
  personal_org_id?: string;
  orgs?: Array<{
    org_id: string;
    org_name: string;
    org_slug: string;
    role: string;
  }>;
}

export interface MeApiResponse {
  success: boolean;
  user: User;
  dashboard: DashboardStats;
  recentAuthorship: unknown[];
  totalAuthorshipRecords: number;
}

export interface DashboardStats {
  aiCode?: { percentage: number; totalAddedLines: number; committedAiLines: number };
  aiOutput?: { generated: number; committed: number; edited: number; ratio: number };
  leaders?: {
    topAgent?: { label: string; promptCount: number };
    topModel?: { label: string; promptCount: number };
  };
  activity?: { activePromptCount: number; checkpointFileCount: number };
  metricsSummary?: { eventCount7d: number; repoCount7d: number; lastSyncAt?: string };
  today?: { activityCount: number; promptCount: number; fileCount: number; lastUpdatedAt?: string };
}

export interface DeviceFlowInfo {
  user_code: string;
  status: "pending" | "approved" | "denied";
  expires_at?: string;
  authenticated: boolean;
  subject?: { name: string; email: string };
}
