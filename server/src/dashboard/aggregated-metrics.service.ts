import { Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma/prisma.service';

type NumberRow = {
  value: bigint | number | string | null;
};

type DashboardOverviewRow = {
  total_added_lines: bigint | number | string | null;
  committed_ai_lines: bigint | number | string | null;
  generated_ai_lines: bigint | number | string | null;
  edited_ai_lines: bigint | number | string | null;
  active_prompts: bigint | number | string | null;
  checkpoint_files: bigint | number | string | null;
};

type LeaderboardRow = {
  label: string | null;
  prompt_count: bigint | number | string | null;
};

type TodaySummaryRow = {
  activity_count: bigint | number | string | null;
  prompt_count: bigint | number | string | null;
  file_count: bigint | number | string | null;
  last_updated_at: Date | null;
};

function asNumber(value: bigint | number | string | null | undefined) {
  if (typeof value === 'bigint') {
    return Number(value);
  }
  if (typeof value === 'number') {
    return value;
  }
  if (typeof value === 'string' && value.trim()) {
    return Number(value);
  }
  return 0;
}

function percentage(numerator: number, denominator: number) {
  if (!denominator) {
    return 0;
  }

  return (numerator / denominator) * 100;
}

@Injectable()
export class AggregatedMetricsService {
  constructor(private readonly prisma: PrismaService) {}

  async getUserDashboardStats(userId: string): Promise<any> {
    const [overviewRow, topAgent, topModel, today, weeklyStats] = await Promise.all([
      this.getOverview(userId),
      this.getTopAgent(userId),
      this.getTopModel(userId),
      this.getTodaySummary(userId),
      this.getWeeklyStats(userId),
    ]);

    const totalAddedLines = asNumber(overviewRow?.total_added_lines);
    const committedAiLines = asNumber(overviewRow?.committed_ai_lines);
    const generatedAiLines = asNumber(overviewRow?.generated_ai_lines);
    const editedAiLines = asNumber(overviewRow?.edited_ai_lines);
    const activePromptCount = asNumber(overviewRow?.active_prompts);
    const checkpointFileCount = asNumber(overviewRow?.checkpoint_files);
    const aiCodePercentage = percentage(committedAiLines, totalAddedLines);

    return {
      userInfo: {
        id: userId,
      },
      aiCode: {
        percentage: aiCodePercentage,
        totalAddedLines,
        committedAiLines,
      },
      leaders: {
        topAgent: {
          label: topAgent?.label || null,
          promptCount: asNumber(topAgent?.prompt_count),
        },
        topModel: {
          label: topModel?.label || null,
          promptCount: asNumber(topModel?.prompt_count),
        },
      },
      activity: {
        activePromptCount,
        checkpointFileCount,
      },
      aiOutput: {
        generated: generatedAiLines,
        committed: committedAiLines,
        edited: editedAiLines,
        ratio: aiCodePercentage,
      },
      today: {
        activityCount: asNumber(today?.activity_count),
        promptCount: asNumber(today?.prompt_count),
        fileCount: asNumber(today?.file_count),
        lastUpdatedAt: today?.last_updated_at?.toISOString() || null,
      },
      trends: [
        {
          period: 'week',
          values: weeklyStats,
        },
      ],
    };
  }

  private async getWeeklyStats(userId: string): Promise<number[]> {
    const rows = await this.prisma.$queryRaw<Array<{ day_index: number; committed_ai_lines: bigint | number | string | null }>>`
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
      where user_id = ${userId}
        and event_id = 1
        and event_timestamp >= now() - interval '7 days'
      group by 1
      order by 1
    `;

    const values = [0, 0, 0, 0, 0, 0, 0];
    for (const row of rows) {
      const index = Number(row.day_index);
      if (index >= 0 && index < values.length) {
        values[index] = asNumber(row.committed_ai_lines);
      }
    }

    return values as number[];
  }

  async getCasRelatedMetrics(): Promise<any> {
    const threshold = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000);
    const [totalEntries, recentEntries] = await this.prisma.$transaction([
      this.prisma.casEntry.count(),
      this.prisma.casEntry.count({
        where: {
          createdAt: {
            gte: threshold,
          },
        },
      }),
    ]);

    return {
      totalEntries,
      recentEntries,
      growthRate: 0,
    };
  }

  private async getOverview(userId: string) {
    const rows = await this.prisma.$queryRaw<DashboardOverviewRow[]>`
      select
        coalesce(sum((values_json->>'2')::int) filter (where event_id = 1), 0) as total_added_lines,
        coalesce(sum((values_json->'5'->>0)::int) filter (where event_id = 1), 0) as committed_ai_lines,
        coalesce(sum((values_json->'7'->>0)::int) filter (where event_id = 1), 0) as generated_ai_lines,
        coalesce(sum((values_json->'4'->>0)::int) filter (where event_id = 1), 0) as edited_ai_lines,
        coalesce(count(distinct attrs_json->>'22') filter (where event_id = 2 and coalesce(attrs_json->>'22', '') <> ''), 0) as active_prompts,
        coalesce(count(*) filter (where event_id = 4), 0) as checkpoint_files
      from public.metrics_events
      where user_id = ${userId}
        and event_timestamp >= now() - interval '7 days'
    `;

    return rows[0];
  }

  private async getTopAgent(userId: string) {
    const rows = await this.prisma.$queryRaw<LeaderboardRow[]>`
      select
        nullif(attrs_json->>'20', '') as label,
        count(distinct attrs_json->>'22') as prompt_count
      from public.metrics_events
      where user_id = ${userId}
        and event_id = 2
        and event_timestamp >= now() - interval '7 days'
        and coalesce(attrs_json->>'22', '') <> ''
      group by 1
      order by 2 desc, 1 asc
      limit 1
    `;

    return rows[0] || null;
  }

  private async getTopModel(userId: string) {
    const rows = await this.prisma.$queryRaw<LeaderboardRow[]>`
      select
        nullif(attrs_json->>'21', '') as label,
        count(distinct attrs_json->>'22') as prompt_count
      from public.metrics_events
      where user_id = ${userId}
        and event_id = 2
        and event_timestamp >= now() - interval '7 days'
        and coalesce(attrs_json->>'22', '') <> ''
      group by 1
      order by 2 desc, 1 asc
      limit 1
    `;

    return rows[0] || null;
  }

  private async getTodaySummary(userId: string) {
    const rows = await this.prisma.$queryRaw<TodaySummaryRow[]>`
      select
        coalesce(count(*) filter (where event_id in (2, 4) and event_timestamp >= date_trunc('day', now())), 0) as activity_count,
        coalesce(count(distinct attrs_json->>'22') filter (where event_id = 2 and event_timestamp >= date_trunc('day', now()) and coalesce(attrs_json->>'22', '') <> ''), 0) as prompt_count,
        coalesce(count(distinct values_json->>'2') filter (where event_id = 4 and event_timestamp >= date_trunc('day', now()) and coalesce(values_json->>'2', '') <> ''), 0) as file_count,
        max(received_at) as last_updated_at
      from public.metrics_events
      where user_id = ${userId}
    `;

    return rows[0] || null;
  }
}
