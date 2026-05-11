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
  org?: { id: string; name: string };
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

export type AdminRangeKey = "7d" | "30d";

export interface AdminDashboardSummary {
  activeUsersToday: number;
  activeUsersInRange: number;
  totalPrompts: number;
  totalCheckpoints: number;
  aiCodePercentage: number;
}

export interface AdminTrendPoint {
  date: string;
  activeUsers: number;
  promptCount: number;
  checkpointCount: number;
  committedAiLines: number;
  totalAddedLines: number;
  generatedAiLines: number;
  editedAiLines: number;
}

export interface AdminTopUser {
  userId: string;
  name: string;
  email: string;
  promptCount: number;
  committedAiLines: number;
}

export interface AdminTopOrg {
  orgId: string;
  orgName: string;
  promptCount: number;
  memberCount: number;
}

/**
 * Generic "label + count + share" row used by multiple admin dashboard
 * distributions:
 *
 * - `agentDistribution` / `modelDistribution`: `promptCount` is the number
 *   of distinct sessions bucketed under the label (agent / model).
 * - `checkpointByEditKind`: `promptCount` is the count of checkpoint events
 *   bucketed under the label (`file_edit` / `bash` / `(unknown)`). Despite
 *   the field name, no prompt aggregation is involved in that context —
 *   the type is reused so the frontend can pipe each distribution through
 *   the same DistributionDonut component (it keys off `promptCount`).
 *
 * Renaming `promptCount` to `count` would be cleaner but breaks the
 * component contract; this JSDoc is the lighter-weight fix. Mirrors the
 * same note on the backend model.AdminDistributionRow.PromptCount field.
 */
export interface AdminDistributionRow {
  label: string;
  promptCount: number;
  share: number;
}

export interface AdminDashboardData {
  range: AdminRangeKey;
  summary: AdminDashboardSummary;
  trend: AdminTrendPoint[];
  topUsers: AdminTopUser[];
  topOrgs: AdminTopOrg[];
  agentDistribution: AdminDistributionRow[];
  modelDistribution: AdminDistributionRow[];
  checkpointByEditKind: AdminDistributionRow[];
}

export interface AdminDashboardResponse {
  success: boolean;
  data: AdminDashboardData;
  timestamp: string;
}
