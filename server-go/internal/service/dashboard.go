package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

type DashboardService struct {
	Pool       *pgxpool.Pool
	MetricsSvc *MetricsService
}

func (s *DashboardService) GetDashboardStats(ctx context.Context, userID string) (map[string]interface{}, error) {
	g, ctx := errgroup.WithContext(ctx)

	var overviewRow map[string]interface{}
	var topAgent map[string]interface{}
	var topModel map[string]interface{}
	var today map[string]interface{}
	var weeklyStats []int
	var metricsSummary interface{}
	var casSummary map[string]interface{}

	g.Go(func() error {
		var err error
		overviewRow, err = s.getOverview(ctx, userID)
		return err
	})

	g.Go(func() error {
		var err error
		topAgent, err = s.getTopAgent(ctx, userID)
		return err
	})

	g.Go(func() error {
		var err error
		topModel, err = s.getTopModel(ctx, userID)
		return err
	})

	g.Go(func() error {
		var err error
		today, err = s.getTodaySummary(ctx, userID)
		return err
	})

	g.Go(func() error {
		var err error
		weeklyStats, err = s.getWeeklyStats(ctx, userID)
		return err
	})

	g.Go(func() error {
		summary, err := s.MetricsSvc.GetUserMetricsSummary(ctx, userID)
		if err != nil {
			return err
		}
		metricsSummary = summary
		return nil
	})

	g.Go(func() error {
		var err error
		casSummary, err = s.getCasRelatedMetrics(ctx)
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	totalAddedLines := asInt(overviewRow["total_added_lines"])
	committedAiLines := asInt(overviewRow["committed_ai_lines"])
	generatedAiLines := asInt(overviewRow["generated_ai_lines"])
	editedAiLines := asInt(overviewRow["edited_ai_lines"])
	activePromptCount := asInt(overviewRow["active_prompts"])
	checkpointFileCount := asInt(overviewRow["checkpoint_files"])
	aiCodePercentage := percentage(committedAiLines, totalAddedLines)

	var topAgentLabel interface{}
	topAgentPromptCount := 0
	if topAgent != nil {
		topAgentLabel = topAgent["label"]
		topAgentPromptCount = asInt(topAgent["prompt_count"])
	}

	var topModelLabel interface{}
	topModelPromptCount := 0
	if topModel != nil {
		topModelLabel = topModel["label"]
		topModelPromptCount = asInt(topModel["prompt_count"])
	}

	return map[string]interface{}{
		"userInfo": map[string]interface{}{
			"id": userID,
		},
		"aiCode": map[string]interface{}{
			"percentage":       aiCodePercentage,
			"totalAddedLines":  totalAddedLines,
			"committedAiLines": committedAiLines,
		},
		"leaders": map[string]interface{}{
			"topAgent": map[string]interface{}{
				"label":       topAgentLabel,
				"promptCount": topAgentPromptCount,
			},
			"topModel": map[string]interface{}{
				"label":       topModelLabel,
				"promptCount": topModelPromptCount,
			},
		},
		"activity": map[string]interface{}{
			"activePromptCount":   activePromptCount,
			"checkpointFileCount": checkpointFileCount,
		},
		"aiOutput": map[string]interface{}{
			"generated": generatedAiLines,
			"committed": committedAiLines,
			"edited":    editedAiLines,
			"ratio":     aiCodePercentage,
		},
		"today": today,
		"trends": []map[string]interface{}{
			{
				"period": "week",
				"values": weeklyStats,
			},
		},
		"metricsSummary": metricsSummary,
		"casSummary":     casSummary,
	}, nil
}

func (s *DashboardService) GetPublicStats(ctx context.Context) (map[string]interface{}, error) {
	casMetrics, err := s.getCasRelatedMetrics(ctx)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"totalUsers":   0,
		"systemHealth": "operational",
	}
	for k, v := range casMetrics {
		result[k] = v
	}

	return result, nil
}

func (s *DashboardService) getOverview(ctx context.Context, userID string) (map[string]interface{}, error) {
	var totalAddedLines, committedAiLines, generatedAiLines, editedAiLines, activePrompts, checkpointFiles int64
	err := s.Pool.QueryRow(ctx, `
		select
			coalesce(sum((values_json->>'2')::int) filter (where event_id = 1), 0) as total_added_lines,
			coalesce(sum((values_json->'5'->>0)::int) filter (where event_id = 1), 0) as committed_ai_lines,
			coalesce(sum((values_json->'7'->>0)::int) filter (where event_id = 1), 0) as generated_ai_lines,
			coalesce(sum((values_json->'4'->>0)::int) filter (where event_id = 1), 0) as edited_ai_lines,
			coalesce(count(distinct coalesce(session_id, attrs_json->>'24')) filter (where event_id = 2 and coalesce(session_id, attrs_json->>'24') <> ''), 0) as active_prompts,
			coalesce(count(*) filter (where event_id = 4), 0) as checkpoint_files
		from public.metrics_events
		where user_id = $1
			and event_timestamp >= now() - interval '7 days'
	`, userID).Scan(&totalAddedLines, &committedAiLines, &generatedAiLines, &editedAiLines, &activePrompts, &checkpointFiles)
	if err != nil {
		return nil, fmt.Errorf("querying overview: %w", err)
	}

	return map[string]interface{}{
		"total_added_lines":  totalAddedLines,
		"committed_ai_lines": committedAiLines,
		"generated_ai_lines": generatedAiLines,
		"edited_ai_lines":    editedAiLines,
		"active_prompts":     activePrompts,
		"checkpoint_files":   checkpointFiles,
	}, nil
}

func (s *DashboardService) getTopAgent(ctx context.Context, userID string) (map[string]interface{}, error) {
	var label *string
	var promptCount int64
	err := s.Pool.QueryRow(ctx, `
		select
			nullif(attrs_json->>'20', '') as label,
			count(distinct coalesce(session_id, attrs_json->>'24')) as prompt_count
		from public.metrics_events
		where user_id = $1
			and event_id = 2
			and event_timestamp >= now() - interval '7 days'
			and coalesce(session_id, attrs_json->>'24') <> ''
		group by 1
		order by 2 desc, 1 asc
		limit 1
	`, userID).Scan(&label, &promptCount)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying top agent: %w", err)
	}

	return map[string]interface{}{
		"label":        label,
		"prompt_count": promptCount,
	}, nil
}

func (s *DashboardService) getTopModel(ctx context.Context, userID string) (map[string]interface{}, error) {
	var label *string
	var promptCount int64
	err := s.Pool.QueryRow(ctx, `
		select
			nullif(attrs_json->>'21', '') as label,
			count(distinct coalesce(session_id, attrs_json->>'24')) as prompt_count
		from public.metrics_events
		where user_id = $1
			and event_id = 2
			and event_timestamp >= now() - interval '7 days'
			and coalesce(session_id, attrs_json->>'24') <> ''
		group by 1
		order by 2 desc, 1 asc
		limit 1
	`, userID).Scan(&label, &promptCount)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying top model: %w", err)
	}

	return map[string]interface{}{
		"label":        label,
		"prompt_count": promptCount,
	}, nil
}

func (s *DashboardService) getTodaySummary(ctx context.Context, userID string) (map[string]interface{}, error) {
	var activityCount, promptCount, fileCount int64
	var lastUpdatedAt *time.Time
	err := s.Pool.QueryRow(ctx, `
		select
			coalesce(count(*) filter (where event_id in (2, 4) and event_timestamp >= date_trunc('day', now())), 0) as activity_count,
			coalesce(count(distinct coalesce(session_id, attrs_json->>'24')) filter (where event_id = 2 and event_timestamp >= date_trunc('day', now()) and coalesce(session_id, attrs_json->>'24') <> ''), 0) as prompt_count,
			coalesce(count(distinct values_json->>'2') filter (where event_id = 4 and event_timestamp >= date_trunc('day', now()) and coalesce(values_json->>'2', '') <> ''), 0) as file_count,
			max(received_at) as last_updated_at
		from public.metrics_events
		where user_id = $1
	`, userID).Scan(&activityCount, &promptCount, &fileCount, &lastUpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("querying today summary: %w", err)
	}

	var lastUpdatedAtStr interface{}
	if lastUpdatedAt != nil {
		lastUpdatedAtStr = lastUpdatedAt.UTC().Format(time.RFC3339)
	}

	return map[string]interface{}{
		"activityCount": activityCount,
		"promptCount":   promptCount,
		"fileCount":     fileCount,
		"lastUpdatedAt": lastUpdatedAtStr,
	}, nil
}

func (s *DashboardService) getWeeklyStats(ctx context.Context, userID string) ([]int, error) {
	rows, err := s.Pool.Query(ctx, `
		select
			greatest(
				0,
				least(
					6,
					6 - floor(extract(epoch from (now() - event_timestamp)) / 86400)::int
				)
			) as day_index,
			coalesce(sum((values_json->'5'->>0)::int), 0) as committed_ai_lines
		from public.metrics_events
		where user_id = $1
			and event_id = 1
			and event_timestamp >= now() - interval '7 days'
		group by 1
		order by 1
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying weekly stats: %w", err)
	}
	defer rows.Close()

	values := make([]int, 7)
	for rows.Next() {
		var dayIndex, committedAiLines int
		if err := rows.Scan(&dayIndex, &committedAiLines); err != nil {
			return nil, fmt.Errorf("scanning weekly stats row: %w", err)
		}
		if dayIndex >= 0 && dayIndex < len(values) {
			values[dayIndex] = committedAiLines
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating weekly stats rows: %w", err)
	}

	return values, nil
}

func (s *DashboardService) getCasRelatedMetrics(ctx context.Context) (map[string]interface{}, error) {
	var totalEntries, recentEntries int64
	err := s.Pool.QueryRow(ctx, `
		SELECT
			count(*) as total_entries,
			count(*) FILTER (WHERE created_at >= now() - interval '7 days') as recent_entries
		FROM cas_entries
	`).Scan(&totalEntries, &recentEntries)
	if err != nil {
		return nil, fmt.Errorf("querying cas metrics: %w", err)
	}

	return map[string]interface{}{
		"totalEntries":  totalEntries,
		"recentEntries": recentEntries,
		"growthRate":    0,
	}, nil
}

func asInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func percentage(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return (float64(numerator) / float64(denominator)) * 100
}
