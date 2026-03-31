import { Client, ClientConfig } from 'pg';
import { resolvePostgresDatabaseUrl } from './db.config';

function getPostgresClientConfig(): ClientConfig {
  return {
    connectionString: resolvePostgresDatabaseUrl(),
  };
}

export async function ensurePostgresSchemaCompatibility() {
  const client = new Client(getPostgresClientConfig());

  try {
    await client.connect();

    await client.query(`create extension if not exists pgcrypto`);

    await client.query(`
      create table if not exists public.oauth_device_codes (
        id uuid primary key default gen_random_uuid(),
        device_code text not null unique,
        user_code text not null unique,
        client_id text not null default 'git-ai-cli',
        verification_uri text not null,
        status text not null,
        user_id text not null,
        subject_json jsonb not null,
        created_at timestamptz not null default now(),
        expires_at timestamptz not null,
        approved_at timestamptz null,
        denied_at timestamptz null,
        last_polled_at timestamptz null
      )
    `);
    await client.query(`
      create index if not exists idx_oauth_device_codes_user_id
      on public.oauth_device_codes (user_id)
    `);
    await client.query(`
      create index if not exists idx_oauth_device_codes_status
      on public.oauth_device_codes (status)
    `);
    await client.query(`
      create index if not exists idx_oauth_device_codes_expires_at
      on public.oauth_device_codes (expires_at)
    `);
    await client.query(`
      create table if not exists public.metrics_events (
        id uuid primary key default gen_random_uuid(),
        user_id text not null,
        distinct_id text null,
        schema_version integer not null,
        event_timestamp timestamptz not null,
        event_id integer not null,
        values_json jsonb not null,
        attrs_json jsonb not null,
        git_ai_version text null,
        repo_url text null,
        tool text null,
        model text null,
        prompt_id text null,
        external_prompt_id text null,
        received_at timestamptz not null default now()
      )
    `);
    await client.query(`
      create index if not exists idx_metrics_events_user_id
      on public.metrics_events (user_id)
    `);
    await client.query(`
      create index if not exists idx_metrics_events_event_timestamp
      on public.metrics_events (event_timestamp)
    `);
    await client.query(`
      create index if not exists idx_metrics_events_repo_url
      on public.metrics_events (repo_url)
    `);
    await client.query(`
      create index if not exists idx_metrics_events_distinct_id
      on public.metrics_events (distinct_id)
    `);
    await client.query(`
      create table if not exists public.audit_logs (
        id uuid primary key default gen_random_uuid(),
        user_id text not null,
        action text not null,
        resource text not null,
        params_json jsonb not null,
        ip text not null,
        user_agent text null,
        occurred_at timestamptz not null,
        success boolean not null,
        details text null,
        created_at timestamptz not null default now()
      )
    `);
    await client.query(`
      create index if not exists idx_audit_logs_user_id
      on public.audit_logs (user_id)
    `);
    await client.query(`
      create index if not exists idx_audit_logs_occurred_at
      on public.audit_logs (occurred_at)
    `);

    const columnCheck = await client.query<{
      data_type: string;
    }>(
      `select data_type
       from information_schema.columns
       where table_schema = 'public'
         and table_name = 'authorship_records'
         and column_name = 'aiAttributionPercentage'`,
    );

    const currentType = columnCheck.rows[0]?.data_type;
    if (!currentType || currentType === 'double precision' || currentType === 'real') {
      return;
    }

    await client.query(`
      alter table public.authorship_records
      alter column "aiAttributionPercentage" type double precision
      using "aiAttributionPercentage"::double precision
    `);
  } finally {
    await client.end().catch(() => undefined);
  }
}
