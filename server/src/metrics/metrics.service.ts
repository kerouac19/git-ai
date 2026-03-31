import { BadRequestException, Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma/prisma.service';

type MetricsUploadError = {
  index: number;
  error: string;
};

type SparseArray = Record<string, unknown>;

type MetricsEventPayload = {
  t: number;
  e: number;
  v: SparseArray;
  a: SparseArray;
};

type MetricsBatchPayload = {
  v: number;
  events: MetricsEventPayload[];
};

type MetricsSummaryRow = {
  event_count_7d: bigint | number | string | null;
  repo_count_7d: bigint | number | string | null;
  last_sync_at: Date | null;
};

function asNullableString(value: unknown) {
  return typeof value === 'string' && value.trim() ? value : null;
}

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

@Injectable()
export class MetricsService {
  private readonly maxBatchSize = 250;
  private readonly supportedSchemaVersion = 1;
  private readonly supportedEventIds = new Set([1, 2, 3, 4]);

  constructor(private readonly prisma: PrismaService) {}

  validateBatchShape(body: Record<string, unknown>): MetricsBatchPayload {
    const version = body.v;
    const events = body.events;

    if (version !== this.supportedSchemaVersion) {
      throw new BadRequestException(
        `Unsupported metrics schema version: ${String(version)}`,
      );
    }

    if (!Array.isArray(events)) {
      throw new BadRequestException('events must be an array');
    }

    if (events.length > this.maxBatchSize) {
      throw new BadRequestException(
        `events must contain at most ${this.maxBatchSize} items`,
      );
    }

    return {
      v: version,
      events: events as MetricsEventPayload[],
    };
  }

  async uploadBatch(
    userId: string,
    distinctId: string | undefined,
    body: MetricsBatchPayload,
  ) {
    const errors: MetricsUploadError[] = [];

    for (const [index, event] of body.events.entries()) {
      const validationError = this.validateEvent(event);
      if (validationError) {
        errors.push({
          index,
          error: validationError,
        });
        continue;
      }

      const attrs = event.a;
      await this.prisma.$executeRaw`
        insert into public.metrics_events (
          user_id,
          distinct_id,
          schema_version,
          event_timestamp,
          event_id,
          values_json,
          attrs_json,
          git_ai_version,
          repo_url,
          tool,
          model,
          prompt_id,
          external_prompt_id
        )
        values (
          ${userId},
          ${distinctId || null},
          ${body.v},
          ${new Date(event.t * 1000)},
          ${event.e},
          ${JSON.stringify(event.v)}::jsonb,
          ${JSON.stringify(attrs)}::jsonb,
          ${asNullableString(attrs['0'])},
          ${asNullableString(attrs['1'])},
          ${asNullableString(attrs['20'])},
          ${asNullableString(attrs['21'])},
          ${asNullableString(attrs['22'])},
          ${asNullableString(attrs['23'])}
        )
      `;
    }

    return { errors };
  }

  async getUserMetricsSummary(userId: string) {
    const rows = await this.prisma.$queryRaw<MetricsSummaryRow[]>`
      select
        count(*) filter (
          where event_timestamp >= now() - interval '7 days'
        ) as event_count_7d,
        count(distinct repo_url) filter (
          where event_timestamp >= now() - interval '7 days'
            and repo_url is not null
            and repo_url <> ''
        ) as repo_count_7d,
        max(received_at) as last_sync_at
      from public.metrics_events
      where user_id = ${userId}
    `;

    const row = rows[0];

    return {
      eventCount7d: asNumber(row?.event_count_7d),
      repoCount7d: asNumber(row?.repo_count_7d),
      lastSyncAt: row?.last_sync_at?.toISOString() || null,
    };
  }

  private validateEvent(event: unknown) {
    if (!event || typeof event !== 'object' || Array.isArray(event)) {
      return 'event must be an object';
    }

    const candidate = event as Partial<MetricsEventPayload>;
    if (!Number.isInteger(candidate.t) || Number(candidate.t) <= 0) {
      return 't must be a positive unix timestamp';
    }

    if (!Number.isInteger(candidate.e) || !this.supportedEventIds.has(Number(candidate.e))) {
      return 'e must be a supported event id';
    }

    if (!this.isSparseObject(candidate.v)) {
      return 'v must be an object';
    }

    if (!this.isSparseObject(candidate.a)) {
      return 'a must be an object';
    }

    if (typeof candidate.a?.['0'] !== 'string' || !candidate.a['0'].trim()) {
      return 'a.0 (git_ai_version) is required';
    }

    return null;
  }

  private isSparseObject(value: unknown): value is SparseArray {
    return !!value && typeof value === 'object' && !Array.isArray(value);
  }
}
