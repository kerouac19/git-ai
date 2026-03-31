import { existsSync, readFileSync } from 'node:fs';
import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';
import { RequestMethod, ValidationPipe } from '@nestjs/common';
import { describeDatabaseTarget } from './database/db.config';
import { ensurePostgresSchemaCompatibility } from './database/postgres-schema-compat';
import type { Request, Response } from 'express';
import express from 'express';
import { CompatibilityAuthService } from './auth/compatibility-auth.service';
import { DashboardService } from './dashboard/dashboard.service';
import {
  clearSessionCookie,
  extractAccessTokenFromCookieHeader,
  serializeSessionCookie,
} from './auth/http-auth.util';

function escapeHtml(value: unknown) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function formatTimestamp(value?: number | string | null) {
  if (!value) {
    return 'N/A';
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return 'N/A';
  }

  return date.toISOString();
}

function renderPage(title: string, body: string) {
  return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>${escapeHtml(title)}</title>
    <style>
      :root {
        color-scheme: light;
        --bg: #f8fafc;
        --panel: #ffffff;
        --border: #e2e8f0;
        --text: #0f172a;
        --muted: #64748b;
        --accent: #0f766e;
        --accent-soft: #f0fdfa;
        --accent-border: #99f6e4;
        --danger: #be123c;
        --danger-soft: #fff1f2;
        --success: #15803d;
        --success-soft: #f0fdf4;
        --card-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
        --panel-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
      }
      * { box-sizing: border-box; }
      body {
        margin: 0;
        min-height: 100vh;
        font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif, "Apple Color Emoji", "Segoe UI Emoji";
        background-color: var(--bg);
        background-image: 
          radial-gradient(at 0% 0%, rgba(15, 118, 110, 0.05) 0px, transparent 50%),
          radial-gradient(at 100% 0%, rgba(15, 118, 110, 0.05) 0px, transparent 50%);
        color: var(--text);
        line-height: 1.5;
      }
      main {
        max-width: 1000px;
        margin: 0 auto;
        padding: 40px 20px 80px;
      }
      .panel {
        background: var(--panel);
        border: 1px solid var(--border);
        border-radius: 24px;
        padding: 32px;
        box-shadow: var(--panel-shadow);
      }
      h1, h2, h3, p { margin-top: 0; }
      h1 { font-size: 2.25rem; font-weight: 800; tracking: -0.025em; margin-bottom: 8px; color: var(--text); }
      h2 { font-size: 0.875rem; font-weight: 700; letter-spacing: 0.05em; text-transform: uppercase; color: var(--muted); margin-bottom: 16px; }
      p, li { line-height: 1.6; }
      .muted { color: var(--muted); font-size: 0.9375rem; }
      .grid {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
        gap: 20px;
      }
      .metrics-grid {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
        gap: 16px;
      }
      .card {
        background: var(--panel);
        border: 1px solid var(--border);
        border-radius: 16px;
        padding: 24px;
        transition: transform 0.2s ease, box-shadow 0.2s ease;
      }
      .card:hover {
        transform: translateY(-2px);
        box-shadow: var(--card-shadow);
      }
      .kpi {
        font-size: 2.5rem;
        font-weight: 800;
        line-height: 1;
        margin: 4px 0;
        color: var(--accent);
      }
      .kpi-unit {
        font-size: 1rem;
        font-weight: 500;
        color: var(--muted);
        margin-left: 4px;
      }
      .actions {
        display: flex;
        gap: 12px;
        flex-wrap: wrap;
        margin-top: 24px;
      }
      button, a.button {
        appearance: none;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        border: 0;
        cursor: pointer;
        text-decoration: none;
        padding: 10px 24px;
        border-radius: 12px;
        font-weight: 600;
        font-size: 0.9375rem;
        transition: all 0.2s ease;
      }
      button.primary, a.button.primary {
        background: var(--accent);
        color: white;
      }
      button.primary:hover, a.button.primary:hover {
        background: #0d5a52;
        box-shadow: 0 4px 12px rgba(15, 118, 110, 0.2);
      }
      button.secondary, a.button.secondary {
        background: var(--danger-soft);
        color: var(--danger);
      }
      button.secondary:hover, a.button.secondary:hover {
        background: #ffe4e6;
      }
      code {
        font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
        background: #f1f5f9;
        padding: 2px 6px;
        border-radius: 6px;
        font-size: 0.875rem;
        color: var(--accent);
      }
      .notice {
        padding: 16px;
        border-radius: 12px;
        margin: 20px 0 0;
        display: flex;
        align-items: center;
        gap: 12px;
        font-weight: 500;
      }
      .notice.ok { background: var(--success-soft); color: var(--success); border: 1px solid #dcfce7; }
      .notice.error { background: var(--danger-soft); color: var(--danger); border: 1px solid #fee2e2; }
      .badge {
        display: inline-flex;
        align-items: center;
        padding: 2px 10px;
        border-radius: 9999px;
        font-size: 0.75rem;
        font-weight: 700;
        text-transform: uppercase;
      }
      .badge-accent { background: var(--accent-soft); color: var(--accent); border: 1px solid var(--accent-border); }
      .status-dot {
        width: 8px;
        height: 8px;
        border-radius: 50%;
        display: inline-block;
        margin-right: 6px;
      }
      .status-dot.online { background: #22c55e; box-shadow: 0 0 0 4px rgba(34, 197, 94, 0.2); }
      .status-dot.offline { background: #94a3b8; }
      
      .profile-header {
        display: flex;
        align-items: center;
        gap: 24px;
        margin-bottom: 32px;
      }
      .avatar {
        width: 80px;
        height: 80px;
        background: linear-gradient(135deg, var(--accent), #2dd4bf);
        border-radius: 24px;
        display: flex;
        align-items: center;
        justify-content: center;
        color: white;
        font-size: 2rem;
        font-weight: 800;
        box-shadow: 0 8px 16px rgba(15, 118, 110, 0.2);
      }
      .metric-label {
        color: var(--muted);
        font-size: 0.875rem;
        font-weight: 500;
      }
      form { margin: 0; }
      @media (max-width: 640px) {
        .profile-header { flex-direction: column; text-align: center; }
        h1 { font-size: 1.75rem; }
      }
    </style>
  </head>
  <body>
    <main>${body}</main>
  </body>
</html>`;
}

function renderDashboardPage(payload: Record<string, unknown>, dashboard: any) {
  const orgs = Array.isArray(payload.orgs) ? payload.orgs : [];
  const primaryOrg = orgs[0] as Record<string, unknown> | undefined;
  const metricsSummary = dashboard?.metricsSummary;
  const lastSyncAt =
    typeof metricsSummary?.lastSyncAt === 'string' ? metricsSummary.lastSyncAt : null;
  const aiCodePercentage = Number(dashboard?.aiCode?.percentage ?? 0);
  const totalAddedLines = Number(dashboard?.aiCode?.totalAddedLines ?? 0);
  const committedAiLines = Number(dashboard?.aiCode?.committedAiLines ?? 0);
  const topAgentLabel = dashboard?.leaders?.topAgent?.label || '暂无';
  const topAgentCount = Number(dashboard?.leaders?.topAgent?.promptCount ?? 0);
  const topModelLabel = dashboard?.leaders?.topModel?.label || '暂无';
  const topModelCount = Number(dashboard?.leaders?.topModel?.promptCount ?? 0);
  const activePromptCount = Number(dashboard?.activity?.activePromptCount ?? 0);
  const checkpointFileCount = Number(dashboard?.activity?.checkpointFileCount ?? 0);
  const generatedAiLines = Number(dashboard?.aiOutput?.generated ?? 0);
  const editedAiLines = Number(dashboard?.aiOutput?.edited ?? 0);
  const todayActivityCount = Number(dashboard?.today?.activityCount ?? 0);
  const todayPromptCount = Number(dashboard?.today?.promptCount ?? 0);
  const todayFileCount = Number(dashboard?.today?.fileCount ?? 0);
  const todayLastUpdatedAt =
    typeof dashboard?.today?.lastUpdatedAt === 'string' ? dashboard.today.lastUpdatedAt : null;
  
  const syncStatus = lastSyncAt ? 'online' : 'offline';
  const syncStatusLabel = lastSyncAt ? '正在同步' : '未连接';
  
  const displayName = payload.name || payload.email || payload.sub || 'Git AI 用户';
  const displayEmail = payload.email || '未提供邮箱';
  const displayUserId = payload.sub || '未识别';
  const displayRole = payload.role || 'user';
  const displayOrgName = primaryOrg?.org_name || payload.personal_org_id || '个人空间';
  const displayOrgSlug = primaryOrg?.org_slug || 'personal';
  
  const avatarLetter = (displayName as string).charAt(0).toUpperCase();

  const todaySummary = todayActivityCount > 0
    ? `今日已有 <strong>${todayActivityCount}</strong> 条活动记录，涵盖 <strong>${todayPromptCount}</strong> 个 Prompt 及 <strong>${todayFileCount}</strong> 个文件。`
    : '今日暂无活跃数据同步。';

  return renderPage(
    'Git AI Dashboard',
    `
      <div class="panel">
        <div class="profile-header">
          <div class="avatar">${escapeHtml(avatarLetter)}</div>
          <div style="flex: 1;">
            <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 4px;">
              <h1>${escapeHtml(displayName)}</h1>
              <span class="badge badge-accent">${escapeHtml(displayRole)}</span>
            </div>
            <p class="muted" style="margin-bottom: 0;">${escapeHtml(displayEmail)}</p>
          </div>
          <div style="text-align: right;">
             <div class="badge" style="background: ${lastSyncAt ? '#f0fdf4' : '#f1f5f9'}; color: ${lastSyncAt ? '#15803d' : '#64748b'}; border: 1px solid ${lastSyncAt ? '#bbf7d0' : '#e2e8f0'}; padding: 4px 12px;">
               <span class="status-dot ${syncStatus}"></span>
               ${escapeHtml(syncStatusLabel)}
             </div>
             <p class="muted" style="margin-top: 8px; font-size: 0.75rem;">最后同步: ${escapeHtml(formatTimestamp(lastSyncAt))}</p>
          </div>
        </div>

        <div class="grid">
          <div class="card" style="background: var(--accent-soft); border-color: var(--accent-border);">
            <h2>组织架构</h2>
            <div style="display: flex; align-items: center; gap: 12px;">
              <div style="width: 40px; height: 40px; background: white; border-radius: 10px; display: flex; align-items: center; justify-content: center; border: 1px solid var(--accent-border);">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="color: var(--accent);"><path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"></path><polyline points="9 22 9 12 15 12 15 22"></polyline></svg>
              </div>
              <div>
                <p style="font-weight: 700; margin: 0;">${escapeHtml(displayOrgName)}</p>
                <p class="muted" style="font-size: 0.75rem; margin: 0;">Slug: ${escapeHtml(displayOrgSlug)}</p>
              </div>
            </div>
          </div>
          <div class="card">
            <h2>用户识别码</h2>
            <p style="font-family: monospace; font-size: 0.875rem; background: #f8fafc; padding: 8px 12px; border-radius: 8px; border: 1px solid var(--border); word-break: break-all;">${escapeHtml(displayUserId)}</p>
          </div>
        </div>
      </div>

      <div class="metrics-grid" style="margin-top: 24px;">
        <div class="card">
          <p class="metric-label">AI 代码贡献占比</p>
          <p class="kpi">${escapeHtml(aiCodePercentage.toFixed(1))}<span class="kpi-unit">%</span></p>
          <div style="width: 100%; height: 8px; background: #f1f5f9; border-radius: 4px; margin: 12px 0; overflow: hidden;">
            <div style="width: ${aiCodePercentage}%; height: 100%; background: var(--accent); border-radius: 4px;"></div>
          </div>
          <p class="muted" style="font-size: 0.75rem;">AI 提交 ${escapeHtml(committedAiLines)} 行 / 总计 ${escapeHtml(totalAddedLines)} 行</p>
        </div>
        
        <div class="card">
          <p class="metric-label">活跃 Prompt 数</p>
          <p class="kpi">${escapeHtml(activePromptCount)}</p>
          <p class="muted" style="font-size: 0.75rem;">过去 7 天内独立 Prompt 统计</p>
          <div style="margin-top: 16px; display: flex; gap: 8px;">
            <span class="badge" style="background: #eff6ff; color: #1d4ed8;">Prompts</span>
          </div>
        </div>

        <div class="card">
          <p class="metric-label">最常使用 Agent</p>
          <p class="kpi" style="font-size: 1.5rem; margin: 12px 0;">${escapeHtml(topAgentLabel)}</p>
          <p class="muted" style="font-size: 0.75rem;">活跃次数: ${escapeHtml(topAgentCount)}</p>
        </div>

        <div class="card">
          <p class="metric-label">常用 AI 模型</p>
          <p class="kpi" style="font-size: 1.5rem; margin: 12px 0;">${escapeHtml(topModelLabel)}</p>
          <p class="muted" style="font-size: 0.75rem;">活跃次数: ${escapeHtml(topModelCount)}</p>
        </div>
      </div>

      <div class="card" style="margin-top: 24px; display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 32px;">
        <div>
          <h2>AI 输出效能 (7d)</h2>
          <div style="display: flex; flex-direction: column; gap: 12px;">
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span class="muted">生成代码行数</span>
              <span style="font-weight: 700;">${escapeHtml(generatedAiLines)}</span>
            </div>
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span class="muted">已提交代码行数</span>
              <span style="font-weight: 700;">${escapeHtml(committedAiLines)}</span>
            </div>
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span class="muted">人工编辑代码行数</span>
              <span style="font-weight: 700;">${escapeHtml(editedAiLines)}</span>
            </div>
          </div>
        </div>
        <div>
          <h2>开发活跃度 (7d)</h2>
          <div style="display: flex; flex-direction: column; gap: 12px;">
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span class="muted">触达文件总数</span>
              <span style="font-weight: 700;">${escapeHtml(checkpointFileCount)}</span>
            </div>
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span class="muted">涉及代码仓库</span>
              <span style="font-weight: 700;">${escapeHtml(metricsSummary?.repoCount7d ?? 0)}</span>
            </div>
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span class="muted">同步事件总数</span>
              <span style="font-weight: 700;">${escapeHtml(metricsSummary?.eventCount7d ?? 0)}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="panel" style="margin-top: 24px; padding: 24px; border-style: dashed; background: #fafafa;">
        <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px;">
          <h2 style="margin: 0;">今日动态概览</h2>
          <span class="muted" style="font-size: 0.75rem;">更新时间: ${escapeHtml(formatTimestamp(todayLastUpdatedAt))}</span>
        </div>
        <p style="margin: 0; color: var(--text);">${todaySummary}</p>
      </div>
    `,
  );
}

function renderLoginRequiredPage() {
  return renderPage(
    'Git AI Authentication Required',
    `
      <section class="panel" style="text-align: center; padding: 60px 40px;">
        <div style="width: 64px; height: 64px; background: var(--danger-soft); border-radius: 20px; display: flex; align-items: center; justify-content: center; margin: 0 auto 24px; color: var(--danger);">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18.36 6.64a9 9 0 1 1-12.73 0"></path><line x1="12" y1="2" x2="12" y2="12"></line></svg>
        </div>
        <h1>需要身份验证</h1>
        <p class="muted" style="max-width: 400px; margin: 0 auto 32px;">当前浏览器会话无效或已过期。请通过 CLI 重新授权以访问您的个人仪表板。</p>
        
        <div class="notice error" style="justify-content: center; text-align: left; max-width: 460px; margin: 0 auto;">
          <div>
            <p style="margin-bottom: 8px;"><strong>快速重新登录：</strong></p>
            <code style="display: block; padding: 12px; font-size: 1rem;">git-ai login</code>
          </div>
        </div>
        
        <div style="margin-top: 40px;">
          <p class="muted" style="font-size: 0.875rem;">授权完成后，此页面将自动可用。</p>
        </div>
      </section>
    `,
  );
}

function renderDeviceFlowPage(entry: {
  userCode: string;
  expiresAt: number;
  status: string;
  subject: { email: string; name: string };
}) {
  const escapedUserCode = escapeHtml(entry.userCode);
  const statusNotice =
    entry.status === 'approved'
      ? '<div class="notice ok">This device has already been approved.</div>'
      : entry.status === 'denied'
        ? '<div class="notice error">This device request has already been denied.</div>'
        : '';
  const actions =
    entry.status === 'pending'
      ? `
        <div class="actions">
          <form method="post" action="/oauth/device/approve">
            <input type="hidden" name="user_code" value="${escapedUserCode}" />
            <button class="primary" type="submit">Approve Device</button>
          </form>
          <form method="post" action="/oauth/device/deny">
            <input type="hidden" name="user_code" value="${escapedUserCode}" />
            <button class="secondary" type="submit">Deny</button>
          </form>
        </div>
      `
      : `
        <div class="actions">
          <a class="button primary" href="/me">Open Dashboard</a>
        </div>
      `;

  return renderPage(
    'Git AI Device Authorization',
    `
      <section class="panel">
        <p class="muted">Git AI device authorization</p>
        <h1>Approve CLI access</h1>
        <p>Authorize the pending CLI login for <strong>${escapeHtml(entry.subject.name)}</strong> (${escapeHtml(entry.subject.email)}).</p>
        <div class="grid" style="margin-top: 24px;">
          <div class="card">
            <h2>User Code</h2>
            <p><code>${escapedUserCode}</code></p>
          </div>
          <div class="card">
            <h2>Expires At</h2>
            <p>${escapeHtml(formatTimestamp(entry.expiresAt))}</p>
          </div>
          <div class="card">
            <h2>Status</h2>
            <p>${escapeHtml(entry.status)}</p>
          </div>
        </div>
        ${statusNotice}
        ${actions}
      </section>
    `,
  );
}

function renderDeviceFlowResultPage(
  title: string,
  message: string,
  variant: 'ok' | 'error',
  actionHref?: string,
  actionLabel?: string,
) {
  return renderPage(
    title,
    `
      <section class="panel">
        <p class="muted">Git AI device authorization</p>
        <h1>${escapeHtml(title)}</h1>
        <div class="notice ${variant}">${escapeHtml(message)}</div>
        ${
          actionHref && actionLabel
            ? `<div class="actions"><a class="button primary" href="${escapeHtml(actionHref)}">${escapeHtml(actionLabel)}</a></div>`
            : ''
        }
      </section>
    `,
  );
}

function extractRequestAccessToken(req: Request) {
  const authorizationHeader = req.headers.authorization;
  if (typeof authorizationHeader === 'string' && authorizationHeader.startsWith('Bearer ')) {
    return authorizationHeader.slice('Bearer '.length).trim();
  }

  return extractAccessTokenFromCookieHeader(req.headers.cookie);
}

function extractUserCode(req: Request) {
  const bodyUserCode = typeof req.body?.user_code === 'string' ? req.body.user_code : undefined;
  const queryUserCode =
    typeof req.query.user_code === 'string' ? req.query.user_code : undefined;
  return bodyUserCode || queryUserCode || '';
}

function resolveTrustProxySetting() {
  const value = process.env.TRUST_PROXY?.trim();
  if (!value) {
    return false;
  }

  const lowered = value.toLowerCase();
  if (['true', 'yes', 'on'].includes(lowered)) {
    return true;
  }

  if (['false', 'no', 'off'].includes(lowered)) {
    return false;
  }

  if (/^\d+$/.test(value)) {
    return Number(value);
  }

  return value;
}

function resolveJsonBodyLimit() {
  const value = process.env.JSON_BODY_LIMIT?.trim();
  return value || '2mb';
}

function isTruthyEnv(value?: string) {
  if (!value) {
    return false;
  }

  return ['true', '1', 'yes', 'on'].includes(value.trim().toLowerCase());
}

function resolveDevHttpsOptions() {
  if (!isTruthyEnv(process.env.DEV_HTTPS)) {
    return null;
  }

  const keyPath = process.env.DEV_HTTPS_KEY_PATH?.trim() || 'certs/localhost-key.pem';
  const certPath = process.env.DEV_HTTPS_CERT_PATH?.trim() || 'certs/localhost.pem';

  if (!existsSync(keyPath)) {
    throw new Error(`DEV_HTTPS is enabled but key file was not found: ${keyPath}`);
  }

  if (!existsSync(certPath)) {
    throw new Error(`DEV_HTTPS is enabled but cert file was not found: ${certPath}`);
  }

  return {
    keyPath,
    certPath,
    httpsOptions: {
      key: readFileSync(keyPath),
      cert: readFileSync(certPath),
    },
  };
}

async function bootstrap() {
  await ensurePostgresSchemaCompatibility();
  const devHttps = resolveDevHttpsOptions();
  const app = await NestFactory.create(AppModule, devHttps?.httpsOptions ? {
    httpsOptions: devHttps.httpsOptions,
  } : undefined);

  // 设置全局前缀
  app.setGlobalPrefix('api', {
    exclude: [
      { path: 'worker/cas/upload', method: RequestMethod.POST },
      { path: 'worker/cas', method: RequestMethod.GET },
      { path: 'worker/cas/checkout', method: RequestMethod.GET },
      { path: 'workers/cas/upload', method: RequestMethod.POST },
      { path: 'workers/cas', method: RequestMethod.GET },
      { path: 'workers/cas/checkout', method: RequestMethod.GET },
      { path: 'worker/oauth/device/code', method: RequestMethod.POST },
      { path: 'worker/oauth/token', method: RequestMethod.POST },
      { path: 'worker/metrics/upload', method: RequestMethod.POST },
      { path: 'workers/oauth/device/code', method: RequestMethod.POST },
      { path: 'workers/oauth/token', method: RequestMethod.POST },
      { path: 'workers/metrics/upload', method: RequestMethod.POST },
    ],
  });

  // 启用CORS（可根据企业需求配置）
  app.enableCors({
    origin: process.env.CORS_ORIGIN || '*', // 在生产环境中应指定确切的域
    methods: 'GET,HEAD,PUT,PATCH,POST,DELETE',
    credentials: true,
  });

  // 全局管道验证
  app.useGlobalPipes(
    new ValidationPipe({
      whitelist: true, // 自动删除非白名单属性
      forbidNonWhitelisted: true, // 非白名单属性抛出错误
      transform: true, // 自动转换类型
    }),
  );
  app.use(express.json({ limit: resolveJsonBodyLimit() }));
  app.use(express.urlencoded({ extended: false }));

  // 配置信任代理（在负载均衡器后面运行时很重要）
  const httpAdapter = app.getHttpAdapter().getInstance();
  const dashboardService = app.get(DashboardService);
  const compatibilityAuthService = app.get(CompatibilityAuthService);
  const trustProxy = resolveTrustProxySetting();
  httpAdapter.set('trust proxy', trustProxy);
  httpAdapter.get('/health', (_req: unknown, res: { json: (body: unknown) => void }) => {
    res.json({
      status: 'ok',
      service: 'git-ai-private-deploy-server',
    });
  });
  httpAdapter.get('/api/health', (_req: unknown, res: { json: (body: unknown) => void }) => {
    res.json({
      status: 'ok',
      service: 'git-ai-private-deploy-server',
      timestamp: new Date().toISOString(),
    });
  });
  httpAdapter.get('/oauth/device', async (req: Request, res: Response) => {
    const userCode =
      typeof req.query.user_code === 'string' ? req.query.user_code : undefined;
    if (!userCode) {
      res.status(400).type('html').send(
        renderDeviceFlowResultPage(
          'Missing User Code',
          'No user_code query parameter was provided.',
          'error',
        ),
      );
      return;
    }

    const entry = await compatibilityAuthService.getDeviceCodeByUserCode(userCode);
    if (!entry) {
      res.status(404).type('html').send(
        renderDeviceFlowResultPage(
          'Device Request Not Found',
          'The device code is missing, expired, or has already been completed.',
          'error',
        ),
      );
      return;
    }

    res.type('html').send(renderDeviceFlowPage(entry));
  });

  httpAdapter.post('/oauth/device/approve', async (req: Request, res: Response) => {
    const userCode = extractUserCode(req);
    const entry = await compatibilityAuthService.approveDeviceCode(userCode);

    if (!entry) {
      res.status(404).type('html').send(
        renderDeviceFlowResultPage(
          'Device Request Not Found',
          'The device code is missing, expired, or has already been completed.',
          'error',
        ),
      );
      return;
    }

    if (entry.status === 'denied') {
      res.status(409).type('html').send(
        renderDeviceFlowResultPage(
          'Authorization Denied',
          'This device request was already denied and cannot be approved anymore.',
          'error',
        ),
      );
      return;
    }

    const accessToken = compatibilityAuthService.issueBrowserSessionToken(entry.subject);
    res.setHeader(
      'Set-Cookie',
      serializeSessionCookie(
        accessToken,
        compatibilityAuthService.getAccessTokenTtlSeconds(),
      ),
    );
    res.type('html').send(
      renderDeviceFlowResultPage(
        'Device Approved',
        'CLI authorization has been approved. This browser session is now signed in.',
        'ok',
        '/me',
        'Open Dashboard',
      ),
    );
  });

  httpAdapter.post('/oauth/device/deny', async (req: Request, res: Response) => {
    const userCode = extractUserCode(req);
    const entry = await compatibilityAuthService.denyDeviceCode(userCode);

    if (!entry) {
      res.status(404).type('html').send(
        renderDeviceFlowResultPage(
          'Device Request Not Found',
          'The device code is missing, expired, or has already been completed.',
          'error',
        ),
      );
      return;
    }

    res.setHeader('Set-Cookie', clearSessionCookie());
    res.type('html').send(
      renderDeviceFlowResultPage(
        'Device Denied',
        'CLI authorization was denied. You can close this tab and retry git-ai login later.',
        'error',
      ),
    );
  });

  httpAdapter.get('/me', async (req: Request, res: Response) => {
    const accessToken = extractRequestAccessToken(req);
    const tokenPayload =
      accessToken && compatibilityAuthService.decodeAccessToken(accessToken);

    if (!tokenPayload || typeof tokenPayload.sub !== 'string') {
      res.status(401).type('html').send(renderLoginRequiredPage());
      return;
    }

    const dashboard = await dashboardService.getDashboardStats(tokenPayload.sub);
    res.type('html').send(renderDashboardPage(tokenPayload, dashboard));
  });

  // 启动服务器
  const port = process.env.PORT || 3000;
  await app.listen(port);

  const scheme = devHttps ? 'https' : 'http';
  console.log(`Application is running on: ${scheme}://localhost:${port}`);
  console.log(`Environment: ${process.env.NODE_ENV || 'development'}`);
  console.log(`Database target: ${describeDatabaseTarget()}`);
  console.log(`Trust proxy: ${String(trustProxy)}`);
  if (devHttps) {
    console.log(`DEV_HTTPS certificate: ${devHttps.certPath}`);
    console.log(`DEV_HTTPS key: ${devHttps.keyPath}`);
  }
  console.log('Security features enabled: HTTPS redirect, Audit logging, Input validation');
}
bootstrap();
