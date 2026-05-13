-- git-ai server-go: full schema for fresh installs.
--
-- 这是 "从 0 建库" 的目标状态脚本，不与 internal/database/migrations/ 下
-- 的增量脚本绑定。它体现的是基于线上上报数据盘点后的清理结果：
--   - 移除 0 行的 authorship_records / commit_attributions（客户端从未上报）
--   - 移除 0 填充的 metrics_events.parent_session_id /
--     external_parent_session_id / custom_attributes
--   - bundles 补齐 user_id / updated_at
--   - metrics_events 补常用查询的复合索引
--
-- 用法：
--   psql -h <host> -U <user> -d <db> -f schema.sql
--
-- 脚本最后会 seed schema_migrations(version=9, dirty=false)，
-- 这样 server 启动时调用的 golang-migrate Up() 看到所有已知版本
-- 都"已应用"，会直接 no-op，而不会重新跑 001/005 等迁移把
-- 已经被我们刻意删除的表/列再加回来。

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- orgs：组织（用户的归属单位）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS orgs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(128) NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 默认 org，配合 users.org_id 的 DEFAULT 使用；尾部 a1 与
-- bootstrap admin user UUID …001 (DEFAULT_USER_ID) 故意不同。
INSERT INTO orgs (id, name)
VALUES ('00000000-0000-0000-0000-0000000000a1', '研发')
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- users：本地账号（密码登录 / 管理后台）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      VARCHAR(64)  NOT NULL UNIQUE,
    email         VARCHAR(255) UNIQUE,
    display_name  VARCHAR(128),
    password_hash TEXT         NOT NULL,
    role          VARCHAR(32)  NOT NULL DEFAULT 'user',
    status        INTEGER      NOT NULL DEFAULT 1,
    org_id        UUID         NOT NULL
                  DEFAULT '00000000-0000-0000-0000-0000000000a1'
                  REFERENCES orgs(id) ON DELETE RESTRICT,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users (username);
CREATE INDEX IF NOT EXISTS idx_users_email    ON users (email);
CREATE INDEX IF NOT EXISTS idx_users_org_id   ON users (org_id);

-- ---------------------------------------------------------------------------
-- oauth_device_codes：CLI device-flow 授权
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS oauth_device_codes (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_code       TEXT NOT NULL UNIQUE,
    user_code         TEXT NOT NULL UNIQUE,
    client_id         TEXT NOT NULL DEFAULT 'git-ai-cli',
    verification_uri  TEXT NOT NULL,
    status            TEXT NOT NULL,
    -- user_id / subject_json 仅在用户授权后填入，创建时为 NULL。
    user_id           TEXT,
    subject_json      JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at        TIMESTAMPTZ NOT NULL,
    approved_at       TIMESTAMPTZ,
    denied_at         TIMESTAMPTZ,
    last_polled_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_oauth_device_codes_user_id    ON oauth_device_codes (user_id);
CREATE INDEX IF NOT EXISTS idx_oauth_device_codes_status     ON oauth_device_codes (status);
CREATE INDEX IF NOT EXISTS idx_oauth_device_codes_expires_at ON oauth_device_codes (expires_at);

-- ---------------------------------------------------------------------------
-- metrics_events：客户端遥测主表
--
-- 列布局：
--   * 顶层固定字段（user_id / event_id / 时间戳等）
--   * values_json / attrs_json 保留完整原始 sparse 数据
--   * 从 attrs_json 中 promote 出来的常用查询列（pos 0-5, 20, 21, 23-25）
--
-- 已从历史迁移中剔除的列：
--   - prompt_id (旧的 attr 22，客户端已 tombstone)
--   - parent_session_id (attr 26)
--   - external_parent_session_id (attr 27)
--   - custom_attributes (attr 30)
--   线上盘点这 3 个列 0 填充；如需重新启用，可在 attrs_json 上做查询，
--   或后续单独写迁移加回来。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS metrics_events (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             TEXT        NOT NULL,
    distinct_id         TEXT,
    schema_version      INTEGER     NOT NULL,
    event_timestamp     TIMESTAMPTZ NOT NULL,
    event_id            INTEGER     NOT NULL,
    values_json         JSONB       NOT NULL,
    attrs_json          JSONB       NOT NULL,
    -- promoted from attrs_json (位置见 src/metrics/events.rs)
    git_ai_version      TEXT,  -- attr 0
    repo_url            TEXT,  -- attr 1
    author              TEXT,  -- attr 2
    commit_sha          TEXT,  -- attr 3
    base_commit_sha     TEXT,  -- attr 4
    branch              TEXT,  -- attr 5
    tool                TEXT,  -- attr 20
    model               TEXT,  -- attr 21
    external_session_id TEXT,  -- attr 23
    session_id          TEXT,  -- attr 24
    trace_id            TEXT,  -- attr 25
    received_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 单列索引：保留覆盖单维度过滤的查询
CREATE INDEX IF NOT EXISTS idx_metrics_events_event_timestamp ON metrics_events (event_timestamp);
CREATE INDEX IF NOT EXISTS idx_metrics_events_distinct_id     ON metrics_events (distinct_id);
CREATE INDEX IF NOT EXISTS idx_metrics_events_session_id      ON metrics_events (session_id);
CREATE INDEX IF NOT EXISTS idx_metrics_events_branch          ON metrics_events (branch);

-- 复合索引：匹配 dashboard / admin_dashboard 里常见的访问模式
--   * (user_id, event_timestamp DESC) — /me 摘要、用户级窗口聚合
--   * (event_id, event_timestamp)     — admin 大盘里大量 filter (where event_id = X)
--   * (repo_url, event_timestamp)     — 7 天 distinct repo 计数
CREATE INDEX IF NOT EXISTS idx_metrics_events_user_ts
    ON metrics_events (user_id, event_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_events_event_ts
    ON metrics_events (event_id, event_timestamp);
CREATE INDEX IF NOT EXISTS idx_metrics_events_repo_ts
    ON metrics_events (repo_url, event_timestamp);

-- ---------------------------------------------------------------------------
-- audit_logs：管理动作审计
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS audit_logs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      TEXT        NOT NULL,
    action       TEXT        NOT NULL,
    resource     TEXT        NOT NULL,
    params_json  JSONB       NOT NULL,
    ip           TEXT        NOT NULL,
    user_agent   TEXT,
    occurred_at  TIMESTAMPTZ NOT NULL,
    success      BOOLEAN     NOT NULL,
    details      TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id     ON audit_logs (user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_occurred_at ON audit_logs (occurred_at);
-- audit_logs ~2k/天增长，按 created_at 做清理/分页需要这个索引
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at  ON audit_logs (created_at);

-- ---------------------------------------------------------------------------
-- bundles：prompt bundle 上传（POST /api/bundles）
--
-- 相比旧表新增：
--   - user_id：能追踪到具体创建者；与 metrics_events / audit_logs 一致用
--     TEXT 而非 FK，兼容外部认证主体。
--   - updated_at：为后续支持 bundle 更新留接口。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS bundles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     TEXT        NOT NULL,
    title       TEXT        NOT NULL,
    data        JSONB       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bundles_user_id    ON bundles (user_id);
CREATE INDEX IF NOT EXISTS idx_bundles_created_at ON bundles (created_at);

-- ---------------------------------------------------------------------------
-- cas_entries：内容寻址存储（加密 blob + 可选元数据）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS cas_entries (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hash              TEXT NOT NULL UNIQUE,
    encrypted_content TEXT NOT NULL,
    content_type      TEXT NOT NULL DEFAULT 'text/plain',
    metadata          JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- config：服务端运行时配置 KV
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS config (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key          VARCHAR(255) NOT NULL UNIQUE,
    value        TEXT,
    description  TEXT,
    category     VARCHAR(100) NOT NULL DEFAULT 'general',
    is_sensitive BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_config_key      ON config (key);
CREATE INDEX IF NOT EXISTS idx_config_category ON config (category);

-- ---------------------------------------------------------------------------
-- schema_migrations：golang-migrate 元数据表
--
-- 用 schema.sql 起的库，没有跑过任何 *.up.sql 迁移，但 server 启动时
-- 会无条件调用 RunMigrations()。如果不 seed，migrate 会从 v0 开始执行
-- 001~008 把刚才用 schema.sql 创建的"目标状态"反向污染回旧形态
-- （比如重新创建 authorship_records / commit_attributions）。
--
-- 这里直接把版本号写到 migrations/ 目录的最新值，标 dirty=false，
-- migrate.Up() 走到这里就会返回 ErrNoChange。
-- 当 migrations/ 目录新增 009_* 等文件时，需要同步更新这里的版本号。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT  NOT NULL PRIMARY KEY,
    dirty   BOOLEAN NOT NULL
);
INSERT INTO schema_migrations (version, dirty)
VALUES (9, FALSE)
ON CONFLICT (version) DO NOTHING;

COMMIT;
