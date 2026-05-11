CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS authorship_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    git_commit_hash TEXT NOT NULL,
    file_attributions TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ai_attribution_percentage DOUBLE PRECISION NOT NULL
);

CREATE TABLE IF NOT EXISTS commit_attributions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commit_hash TEXT NOT NULL,
    author TEXT NOT NULL,
    file_changes TEXT NOT NULL,
    ai_contribution_metrics TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS cas_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hash TEXT NOT NULL UNIQUE,
    encrypted_content TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'text/plain',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key VARCHAR(255) NOT NULL UNIQUE,
    value TEXT,
    description TEXT,
    category VARCHAR(100) NOT NULL DEFAULT 'general',
    is_sensitive BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_config_key ON config (key);
CREATE INDEX IF NOT EXISTS idx_config_category ON config (category);

CREATE TABLE IF NOT EXISTS oauth_device_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_code TEXT NOT NULL UNIQUE,
    user_code TEXT NOT NULL UNIQUE,
    client_id TEXT NOT NULL DEFAULT 'git-ai-cli',
    verification_uri TEXT NOT NULL,
    status TEXT NOT NULL,
    user_id TEXT NOT NULL,
    subject_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    approved_at TIMESTAMPTZ,
    denied_at TIMESTAMPTZ,
    last_polled_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_oauth_device_codes_user_id ON oauth_device_codes (user_id);
CREATE INDEX IF NOT EXISTS idx_oauth_device_codes_status ON oauth_device_codes (status);
CREATE INDEX IF NOT EXISTS idx_oauth_device_codes_expires_at ON oauth_device_codes (expires_at);

CREATE TABLE IF NOT EXISTS metrics_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    distinct_id TEXT,
    schema_version INTEGER NOT NULL,
    event_timestamp TIMESTAMPTZ NOT NULL,
    event_id INTEGER NOT NULL,
    values_json JSONB NOT NULL,
    attrs_json JSONB NOT NULL,
    git_ai_version TEXT,
    repo_url TEXT,
    author TEXT,
    commit_sha TEXT,
    base_commit_sha TEXT,
    branch TEXT,
    tool TEXT,
    model TEXT,
    external_session_id TEXT,
    session_id TEXT,
    trace_id TEXT,
    parent_session_id TEXT,
    external_parent_session_id TEXT,
    custom_attributes TEXT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_metrics_events_user_id ON metrics_events (user_id);
CREATE INDEX IF NOT EXISTS idx_metrics_events_event_timestamp ON metrics_events (event_timestamp);
CREATE INDEX IF NOT EXISTS idx_metrics_events_repo_url ON metrics_events (repo_url);
CREATE INDEX IF NOT EXISTS idx_metrics_events_distinct_id ON metrics_events (distinct_id);
CREATE INDEX IF NOT EXISTS idx_metrics_events_session_id ON metrics_events (session_id);
CREATE INDEX IF NOT EXISTS idx_metrics_events_branch ON metrics_events (branch);

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    action TEXT NOT NULL,
    resource TEXT NOT NULL,
    params_json JSONB NOT NULL,
    ip TEXT NOT NULL,
    user_agent TEXT,
    occurred_at TIMESTAMPTZ NOT NULL,
    success BOOLEAN NOT NULL,
    details TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs (user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_occurred_at ON audit_logs (occurred_at);
