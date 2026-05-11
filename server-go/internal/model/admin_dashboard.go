package model

// AdminDashboardData is the payload returned by GET /api/dashboard/global.
// All numeric fields default to zero and all slices default to empty (never
// nil) so the SPA can render unconditionally.
type AdminDashboardData struct {
	Range                string                 `json:"range"`
	Summary              AdminDashboardSummary  `json:"summary"`
	Trend                []AdminTrendPoint      `json:"trend"`
	TopUsers             []AdminTopUser         `json:"topUsers"`
	TopOrgs              []AdminTopOrg          `json:"topOrgs"`
	AgentDistribution    []AdminDistributionRow `json:"agentDistribution"`
	ModelDistribution    []AdminDistributionRow `json:"modelDistribution"`
	CheckpointByEditKind []AdminDistributionRow `json:"checkpointByEditKind"`
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

// AdminDistributionRow is a generic "label + count + share" row used by
// multiple admin dashboard distributions:
//
//   - AgentDistribution / ModelDistribution: PromptCount is the number of
//     distinct prompt_id values bucketed under the label (agent / model).
//   - CheckpointByEditKind: PromptCount is the count of checkpoint events
//     bucketed under the label (edit_kind = "file_edit" / "bash" /
//     "(unknown)"). Despite the field name, no prompt aggregation is
//     involved in that context — the type is reused so the frontend can
//     pipe each distribution through the same donut/bar component
//     (DistributionDonut keys off "promptCount" regardless of source).
//
// Renaming PromptCount to Count would be cleaner but breaks the frontend
// JSON contract; this comment is the lighter-weight fix.
type AdminDistributionRow struct {
	Label       string  `json:"label"`
	PromptCount int     `json:"promptCount"`
	Share       float64 `json:"share"`
}
