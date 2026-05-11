package service

import (
	"testing"
	"time"

	"git-ai-server/internal/model"
)

func TestGetSummaryCountsBySessionId(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: requires running PostgreSQL")
	}

	pool := openTestPool(t)

	// Use recent timestamps so the 7-day window in the SQL includes these events.
	now := int(time.Now().Unix())

	// Seed: 3 AgentUsage events (event_id=2) under 2 distinct session_ids
	// (s1 with two events, s2 with one). New-style records use session_id
	// column; we leave attrs_json["22"] empty to simulate post-tombstone data.
	metricsSvc := &MetricsService{Pool: pool}
	batch := &model.MetricsBatch{
		V: 1,
		Events: []model.MetricsEvent{
			{IsObject: true, T: now, E: 2, V: map[string]any{}, A: map[string]any{"0": "1.4.7", "24": "s1", "20": "claude-code", "21": "gpt-5.4"}},
			{IsObject: true, T: now + 1, E: 2, V: map[string]any{}, A: map[string]any{"0": "1.4.7", "24": "s1", "20": "claude-code", "21": "gpt-5.4"}},
			{IsObject: true, T: now + 2, E: 2, V: map[string]any{}, A: map[string]any{"0": "1.4.7", "24": "s2", "20": "cursor", "21": "gpt-5.4"}},
		},
	}
	errs, err := metricsSvc.UploadBatch(t.Context(), "00000000-0000-0000-0000-000000000001", nil, batch)
	if err != nil {
		t.Fatalf("seed UploadBatch error = %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("seed UploadBatch errors = %+v", errs)
	}

	adminSvc := &AdminDashboardService{Pool: pool}
	data, err := adminSvc.GetGlobalStats(t.Context(), "7d")
	if err != nil {
		t.Fatalf("GetGlobalStats error = %v", err)
	}

	if data.Summary.TotalPrompts != 2 {
		t.Fatalf("Summary.TotalPrompts = %d, want 2 (distinct session_ids); attrs[\"22\"] is empty so old SQL would return 0", data.Summary.TotalPrompts)
	}

	// Agent distribution: 1 distinct session under claude-code (s1, 2 events),
	// 1 distinct session under cursor (s2, 1 event).
	agents := map[string]int{}
	for _, row := range data.AgentDistribution {
		agents[row.Label] = row.PromptCount
	}
	if agents["claude-code"] != 1 || agents["cursor"] != 1 {
		t.Fatalf("AgentDistribution = %+v; want claude-code=1, cursor=1 (one distinct session each)", data.AgentDistribution)
	}
}
