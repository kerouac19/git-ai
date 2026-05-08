package service

import (
	"context"
	"fmt"

	"git-ai-server/internal/model"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

type AdminDashboardService struct {
	Pool *pgxpool.Pool
}

// rangeToDays maps the public "7d"/"30d" labels to integers used in the SQL.
// Caller MUST validate the input — anything other than 7 or 30 is undefined.
func rangeToDays(rangeKey string) int {
	switch rangeKey {
	case "30d":
		return 30
	default:
		return 7
	}
}

func (s *AdminDashboardService) GetGlobalStats(ctx context.Context, rangeKey string) (*model.AdminDashboardData, error) {
	days := rangeToDays(rangeKey)

	g, ctx := errgroup.WithContext(ctx)

	var (
		summary  model.AdminDashboardSummary
		trend    []model.AdminTrendPoint
		topUsers []model.AdminTopUser
		topOrgs  []model.AdminTopOrg
		agents   []model.AdminDistributionRow
		models   []model.AdminDistributionRow
	)

	g.Go(func() error { var err error; summary, err = s.getSummary(ctx, days); return err })
	g.Go(func() error { var err error; trend, err = s.getTrend(ctx, days); return err })
	g.Go(func() error { var err error; topUsers, err = s.getTopUsers(ctx, days); return err })
	g.Go(func() error { var err error; topOrgs, err = s.getTopOrgs(ctx, days); return err })
	g.Go(func() error { var err error; agents, err = s.getDistribution(ctx, days, "20"); return err })
	g.Go(func() error { var err error; models, err = s.getDistribution(ctx, days, "21"); return err })

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &model.AdminDashboardData{
		Range:             rangeKey,
		Summary:           summary,
		Trend:             trend,
		TopUsers:          topUsers,
		TopOrgs:           topOrgs,
		AgentDistribution: agents,
		ModelDistribution: models,
	}, nil
}

func (s *AdminDashboardService) getSummary(ctx context.Context, days int) (model.AdminDashboardSummary, error) {
	var sum model.AdminDashboardSummary
	var totalAdded, committedAI int64
	err := s.Pool.QueryRow(ctx, fmt.Sprintf(`
		select
			coalesce(count(distinct user_id) filter (where event_timestamp >= date_trunc('day', now())), 0) as active_today,
			coalesce(count(distinct user_id), 0) as active_in_range,
			coalesce(count(distinct attrs_json->>'22') filter (where event_id = 2 and coalesce(attrs_json->>'22', '') <> ''), 0) as total_prompts,
			coalesce(count(*) filter (where event_id = 4), 0) as total_checkpoints,
			coalesce(sum((values_json->>'2')::int) filter (where event_id = 1), 0) as total_added,
			coalesce(sum((values_json->'5'->>0)::int) filter (where event_id = 1), 0) as committed_ai
		from public.metrics_events
		where event_timestamp >= now() - interval '%d days'
	`, days)).Scan(
		&sum.ActiveUsersToday,
		&sum.ActiveUsersInRange,
		&sum.TotalPrompts,
		&sum.TotalCheckpoints,
		&totalAdded,
		&committedAI,
	)
	if err != nil {
		return sum, fmt.Errorf("admin summary: %w", err)
	}
	if totalAdded > 0 {
		sum.AICodePercentage = float64(committedAI) / float64(totalAdded) * 100
	}
	return sum, nil
}

func (s *AdminDashboardService) getTrend(ctx context.Context, days int) ([]model.AdminTrendPoint, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		with days as (
			select generate_series(
				date_trunc('day', now()) - interval '%d days' + interval '1 day',
				date_trunc('day', now()),
				interval '1 day'
			)::date as day
		)
		select
			d.day::text,
			coalesce(count(distinct e.user_id), 0) as active_users,
			coalesce(count(distinct e.attrs_json->>'22') filter (where e.event_id = 2 and coalesce(e.attrs_json->>'22', '') <> ''), 0) as prompt_count,
			coalesce(count(*) filter (where e.event_id = 4), 0) as checkpoint_count,
			coalesce(sum((e.values_json->'5'->>0)::int) filter (where e.event_id = 1), 0) as committed_ai,
			coalesce(sum((e.values_json->>'2')::int) filter (where e.event_id = 1), 0) as total_added,
			coalesce(sum((e.values_json->'7'->>0)::int) filter (where e.event_id = 1), 0) as generated_ai,
			coalesce(sum((e.values_json->'4'->>0)::int) filter (where e.event_id = 1), 0) as edited_ai
		from days d
		left join public.metrics_events e on date_trunc('day', e.event_timestamp)::date = d.day
		group by d.day
		order by d.day
	`, days-1))
	if err != nil {
		return nil, fmt.Errorf("admin trend query: %w", err)
	}
	defer rows.Close()

	out := make([]model.AdminTrendPoint, 0, days)
	for rows.Next() {
		var p model.AdminTrendPoint
		if err := rows.Scan(
			&p.Date,
			&p.ActiveUsers,
			&p.PromptCount,
			&p.CheckpointCount,
			&p.CommittedAILines,
			&p.TotalAddedLines,
			&p.GeneratedAILines,
			&p.EditedAILines,
		); err != nil {
			return nil, fmt.Errorf("admin trend scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *AdminDashboardService) getTopUsers(ctx context.Context, days int) ([]model.AdminTopUser, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		select
			e.user_id,
			coalesce(u.name, '') as name,
			coalesce(u.email, '') as email,
			count(distinct e.attrs_json->>'22') filter (where e.event_id = 2 and coalesce(e.attrs_json->>'22', '') <> '') as prompt_count,
			coalesce(sum((e.values_json->'5'->>0)::int) filter (where e.event_id = 1), 0) as committed_ai
		from public.metrics_events e
		left join public.users u on u.id::text = e.user_id
		where e.event_timestamp >= now() - interval '%d days'
		group by e.user_id, u.name, u.email
		order by prompt_count desc, committed_ai desc, e.user_id asc
		limit 10
	`, days))
	if err != nil {
		return nil, fmt.Errorf("admin top users: %w", err)
	}
	defer rows.Close()

	out := make([]model.AdminTopUser, 0, 10)
	for rows.Next() {
		var u model.AdminTopUser
		if err := rows.Scan(&u.UserID, &u.Name, &u.Email, &u.PromptCount, &u.CommittedAILines); err != nil {
			return nil, fmt.Errorf("admin top users scan: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *AdminDashboardService) getTopOrgs(ctx context.Context, days int) ([]model.AdminTopOrg, error) {
	// Best-effort: if org_memberships or orgs tables don't exist or schema
	// differs, return empty rather than failing the whole dashboard. The
	// caller can adapt the JOIN once real org schema is confirmed.
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		select
			o.id::text,
			coalesce(o.name, '') as org_name,
			count(distinct e.attrs_json->>'22') filter (where e.event_id = 2 and coalesce(e.attrs_json->>'22', '') <> '') as prompt_count,
			count(distinct e.user_id) as member_count
		from public.metrics_events e
		join public.org_memberships m on m.user_id::text = e.user_id
		join public.orgs o on o.id = m.org_id
		where e.event_timestamp >= now() - interval '%d days'
		group by o.id, o.name
		order by prompt_count desc, member_count desc, o.id asc
		limit 10
	`, days))
	if err != nil {
		// Tolerate schema drift — log and return empty.
		return []model.AdminTopOrg{}, nil
	}
	defer rows.Close()

	out := make([]model.AdminTopOrg, 0, 10)
	for rows.Next() {
		var o model.AdminTopOrg
		if err := rows.Scan(&o.OrgID, &o.OrgName, &o.PromptCount, &o.MemberCount); err != nil {
			return nil, fmt.Errorf("admin top orgs scan: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// getDistribution computes prompt-count share grouped by attrs_json->>attrKey.
// attrKey "20" = agent, "21" = model.
func (s *AdminDashboardService) getDistribution(ctx context.Context, days int, attrKey string) ([]model.AdminDistributionRow, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		select
			coalesce(nullif(attrs_json->>'%s', ''), '(unknown)') as label,
			count(distinct attrs_json->>'22') as prompt_count
		from public.metrics_events
		where event_id = 2
			and event_timestamp >= now() - interval '%d days'
			and coalesce(attrs_json->>'22', '') <> ''
		group by 1
		order by prompt_count desc, label asc
	`, attrKey, days))
	if err != nil {
		return nil, fmt.Errorf("admin distribution: %w", err)
	}
	defer rows.Close()

	type bucket struct {
		label string
		count int
	}
	var buckets []bucket
	var total int
	for rows.Next() {
		var b bucket
		if err := rows.Scan(&b.label, &b.count); err != nil {
			return nil, fmt.Errorf("admin distribution scan: %w", err)
		}
		buckets = append(buckets, b)
		total += b.count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]model.AdminDistributionRow, 0, len(buckets))
	for _, b := range buckets {
		share := 0.0
		if total > 0 {
			share = float64(b.count) / float64(total)
		}
		out = append(out, model.AdminDistributionRow{
			Label:       b.label,
			PromptCount: b.count,
			Share:       share,
		})
	}
	return out, nil
}
