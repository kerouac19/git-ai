package model

// AdminDashboardData is the payload returned by GET /api/admin/dashboard/stats.
// All numeric fields default to zero and all slices default to empty (never
// nil) so the SPA can render unconditionally.
type AdminDashboardData struct {
	Range             string                 `json:"range"`
	Summary           AdminDashboardSummary  `json:"summary"`
	Trend             []AdminTrendPoint      `json:"trend"`
	TopUsers          []AdminTopUser         `json:"topUsers"`
	TopOrgs           []AdminTopOrg          `json:"topOrgs"`
	AgentDistribution []AdminDistributionRow `json:"agentDistribution"`
	ModelDistribution []AdminDistributionRow `json:"modelDistribution"`
}

type AdminDashboardSummary struct {
	ActiveUsersToday   int     `json:"activeUsersToday"`
	ActiveUsersInRange int     `json:"activeUsersInRange"`
	TotalPrompts       int     `json:"totalPrompts"`
	TotalCheckpoints   int     `json:"totalCheckpoints"`
	AICodePercentage   float64 `json:"aiCodePercentage"`
}

type AdminTrendPoint struct {
	Date             string `json:"date"`
	ActiveUsers      int    `json:"activeUsers"`
	PromptCount      int    `json:"promptCount"`
	CheckpointCount  int    `json:"checkpointCount"`
	CommittedAILines int    `json:"committedAiLines"`
	TotalAddedLines  int    `json:"totalAddedLines"`
	GeneratedAILines int    `json:"generatedAiLines"`
	EditedAILines    int    `json:"editedAiLines"`
}

type AdminTopUser struct {
	UserID           string `json:"userId"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	PromptCount      int    `json:"promptCount"`
	CommittedAILines int    `json:"committedAiLines"`
}

type AdminTopOrg struct {
	OrgID       string `json:"orgId"`
	OrgName     string `json:"orgName"`
	PromptCount int    `json:"promptCount"`
	MemberCount int    `json:"memberCount"`
}

type AdminDistributionRow struct {
	Label       string  `json:"label"`
	PromptCount int     `json:"promptCount"`
	Share       float64 `json:"share"`
}
