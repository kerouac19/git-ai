# server-go 前后端分离设计

日期：2026-05-08
范围：server-go（不影响 git-ai CLI 与 daemon）

## 背景与目标

`server-go` 当前用 `html/template` 渲染 5 个页面：`/login`、`/me`、`/oauth/device`、设备授权结果页、`login_required`。模板与 Go 二进制耦合，前端无法独立迭代，也难以承载更丰富的交互。

本次改造的目标：
- 前端独立成 React + Vite + TypeScript 项目，单页应用（SPA）
- 后端只保留 JSON API，不再渲染 HTML
- 一次性重写，不维护双套实现

非目标：
- 不引入 SSR、SEO 优化
- 不改变 worker / CLI 走的接口契约
- 不改变身份系统、数据库 schema

## 关键决策

| 维度 | 选择 | 备注 |
|---|---|---|
| 框架 | React + Vite + TypeScript | 生态成熟、AI 工具支持好 |
| 部署形态 | Nginx 反代 + 静态目录 | 同源，避免 CORS |
| 代码位置 | `server-go/web/` | 与现有 deploy 脚本同根 |
| 节奏 | 一次性重写 | 项目体量小（5 页面） |
| 会话 | 保留 HttpOnly Cookie + JWT | 与现有 `auth/cookie.go` 一致 |
| CSRF | 双提交 Cookie token | 比单 header 校验更严，比 server-side token 表轻 |
| URL | `/login`、`/me`、`/oauth/device` 均保留 | CLI 已硬编码 `/oauth/device` |

## 架构

```
                     ┌──────────────┐
   browser ──HTTPS──►│   Nginx 443  │
                     └──────┬───────┘
                            │
        ┌───────────────────┼──────────────────┐
        ▼                   ▼                  ▼
  /, /assets/*         /api/*, /health,    /workers?/*,
  (SPA 静态)           /oauth/device/*     /releases (CLI 兼容)
        │              (JSON)              (JSON)
        ▼                   ▼                  ▼
 /var/www/git-ai/      ┌─────────────────────────┐
    dist/              │   Go server :3000       │
                       │   (无 html/template)     │
                       └─────────────────────────┘
```

同源带来的好处：Cookie 自然生效，无 CORS。

## 后端改动

### 删除

- `server-go/internal/templates/`（整个目录）
- `cmd/server/main.go` 中：
  - `templates`、`templateFS`、`init()`
  - `handleLoginPage`、`handleDeviceFlowPage`、`handleDeviceApprove`、`handleDeviceDeny`、`handleMePage`
  - `renderResult`、`renderLoginRequired`
  - `buildDashboardPageData`、`deviceFlowPageData`、`deviceResultPageData`、`dashboardPageData`
  - `toFloat`、`toInt`、`toString` 辅助函数
  - 路由：`GET /login`、`GET /me`、`GET /oauth/device`、`POST /oauth/device/approve`、`POST /oauth/device/deny`

### 新增 JSON 接口

放在 `/api/oauth/device/*` 命名空间下，与现有 `/api/*` 风格一致。新增 handler 在 `internal/handler/device_flow.go`。

| 方法 | 路径 | 用途 | 鉴权 |
|---|---|---|---|
| GET | `/api/oauth/device/info?user_code=` | 拉取设备授权元数据：subject、过期时间、status | Cookie（可选）；未登录也返回基础信息便于跳登录 |
| POST | `/api/oauth/device/approve` | body `{user_code}` | Cookie 必需 + CSRF |
| POST | `/api/oauth/device/deny` | body `{user_code}` | Cookie 必需 + CSRF |

返回结构遵循 `respond.go` 现有 JSON envelope。

### CSRF 中间件（双提交 Cookie）

新增 `internal/middleware/csrf.go`：

- 登录成功时，除现有 HttpOnly 会话 cookie 外额外 `Set-Cookie: csrf_token=<random32>; Path=/; SameSite=Lax`（**非 HttpOnly**，前端 JS 需要读取）。`Secure` 属性沿用现有 `auth/cookie.go` 中 `isProduction` 的判定，保持与会话 cookie 一致
- 注销时清除 `csrf_token`
- 中间件对 `/api/*` 下所有不安全方法（POST/PUT/PATCH/DELETE）校验：
  - 请求头 `X-CSRF-Token` 必须存在且与 `csrf_token` cookie 值相等（常量时间比较）
  - 不通过返回 403 + JSON 错误
- 仅作用于 Cookie 鉴权路径：worker / API key / Bearer token 路径不挂此中间件
- 应用范围：`/api/user/login`（首次登录无 cookie，需在响应中下发 token，不校验入站）、`/api/user/logout`、`/api/oauth/device/approve|deny`、`/api/authorship/*`（写）、`/api/cas/upload`、`/api/dashboard/generate-report`、`/api/config/*`、`/api/bundles`、`/api/user/register`

**注意**：`workerMW` 路径下的写接口（`authorshipWrite`、`cas/upload`）默认走 worker 鉴权（CLI JWT 或 X-API-Key），不受 CSRF 约束；但如果同一接口也允许浏览器 cookie 通过 worker MW 进入，需要在 worker MW 内部判断"凭据来自 cookie"时落到 CSRF 校验。实现细节在 plan 阶段确定。

### 登录响应

`POST /api/user/login` 在原有 `Set-Cookie: session=...; HttpOnly` 基础上，额外 `Set-Cookie: csrf_token=...`。响应 body 增加 `{ csrf_token: "..." }`，前端首次登录时直接拿到值，不依赖 `document.cookie` 解析。

## 前端项目（`server-go/web/`）

### 目录结构

```
web/
├── package.json
├── pnpm-lock.yaml
├── tsconfig.json
├── vite.config.ts
├── index.html
├── .gitignore                  # 包含 dist/, node_modules/
├── src/
│   ├── main.tsx
│   ├── App.tsx                 # Router 入口
│   ├── routes/
│   │   ├── Login.tsx
│   │   ├── Me.tsx              # 仪表盘
│   │   ├── DeviceFlow.tsx
│   │   └── DeviceResult.tsx
│   ├── api/
│   │   ├── client.ts           # fetch 封装
│   │   ├── auth.ts
│   │   ├── dashboard.ts
│   │   └── device.ts
│   ├── components/
│   │   ├── Stat.tsx
│   │   ├── Avatar.tsx
│   │   └── ProtectedRoute.tsx
│   ├── hooks/
│   │   └── useMe.ts
│   ├── styles/
│   │   └── globals.css
│   └── types/
│       └── api.ts              # API 响应类型
└── dist/                       # 构建产物，部署时 rsync
```

### 路由表

| 路径 | 组件 | 鉴权 |
|---|---|---|
| `/login` | `Login` | 无 |
| `/me` | `Me` | 需 cookie，否则跳 `/login?redirect=/me` |
| `/oauth/device` | `DeviceFlow`（依赖 `?user_code`） | 软鉴权：未登录跳登录后回 |
| `/oauth/device/result` | `DeviceResult` | 无 |
| `*` | 重定向到 `/me`（未登录时 `/me` 自身会再跳 `/login`） | — |

`Me`、`DeviceFlow`（在已登录态判断上）通过 `ProtectedRoute` 包装。

### `client.ts` 关键行为

```ts
async function request(path, init) {
  const headers = new Headers(init?.headers);
  // 双提交：从 cookie 读 csrf_token，写到请求头
  const unsafe = !['GET','HEAD','OPTIONS'].includes(init?.method ?? 'GET');
  if (unsafe) {
    const token = readCookie('csrf_token');
    if (token) headers.set('X-CSRF-Token', token);
  }
  const res = await fetch(path, { ...init, credentials: 'include', headers });
  if (res.status === 401) { /* 触发统一登出 + 跳 /login */ }
  if (!res.ok) throw new ApiError(res.status, await res.json().catch(() => null));
  return res.json();
}
```

### Vite dev 代理（`vite.config.ts`）

开发期 Vite 在 `localhost:5173`，把 `/api`、`/health`、`/oauth/device`（仅 GET 重定向到前端路由 — dev 下不代理这一条）代理到 `localhost:3000`：

```ts
proxy: {
  '/api': 'http://localhost:3000',
  '/health': 'http://localhost:3000',
  '/workers': 'http://localhost:3000',
  '/worker': 'http://localhost:3000',
  '/releases': 'http://localhost:3000',
}
```

`/oauth/device`、`/login`、`/me` 等不代理，由 Vite 自身的 history fallback 接管。

## Nginx 配置（`deploy/nginx.conf`）

替换现有 `location /` 段，新增静态托管和精确分流：

```nginx
root /var/www/git-ai/dist;
index index.html;

# 后端 JSON / OAuth / health
location /api/        { proxy_pass http://git_ai_backend; <现有 proxy_set_header 段> }
location = /health    { proxy_pass http://git_ai_backend; access_log off; <同上> }
location /workers/    { proxy_pass http://git_ai_backend; <同上> }
location /worker/     { proxy_pass http://git_ai_backend; <同上> }
location /releases    { proxy_pass http://git_ai_backend; <同上> }

# 静态资源（Vite 打包带 hash，可永久缓存）
location /assets/ {
    expires 1y;
    add_header Cache-Control "public, immutable";
    try_files $uri =404;
}

# SPA fallback：未命中文件返回 index.html
location / {
    try_files $uri /index.html;
}
```

保留现有 `limit_req zone=gitai_auth` 配置，挂在 `= /api/user/login` 和 `~ ^/workers?/oauth/device/code$`。

## 部署脚本（`scripts/deploy.sh`）

构建阶段新增（在现有 Go build 之前或之后）：

```bash
(cd web && pnpm install --frozen-lockfile && pnpm build)
```

发布阶段新增：

```bash
ssh deploy@$HOST 'mkdir -p /var/www/git-ai/dist'
rsync -av --delete web/dist/ deploy@$HOST:/var/www/git-ai/dist/
```

Go 二进制构建与上传保持不变。要求部署机已装 `node` 与 `pnpm`（由 deploy.sh 检测；缺失则报错退出）。

## 数据流

### 登录

1. SPA `Login` 页 `POST /api/user/login` body `{username, password}`
2. 后端验证 → `Set-Cookie: session=<jwt>; HttpOnly` + `Set-Cookie: csrf_token=<rand>` + 200 `{ user, csrf_token }`
3. SPA 跳 `?redirect` 或 `/me`

### 仪表盘

1. `Me` mount → 并发 `GET /api/me`、`GET /api/dashboard/stats`
2. 任一 401 → `client.ts` 跳 `/login?redirect=/me`
3. 200 → 渲染统计卡片

### 设备授权

1. CLI 打印 `https://git-ai.example.com/oauth/device?user_code=XXX`
2. 用户在浏览器打开 → SPA `DeviceFlow` 加载
3. `GET /api/oauth/device/info?user_code=XXX`
   - 找不到 / 已过期 → 跳 `/oauth/device/result?status=error&reason=not_found`
   - 已登录 → 显示 subject 与按钮
   - 未登录 → 跳 `/login?redirect=/oauth/device?user_code=XXX`
4. 同意 → `POST /api/oauth/device/approve` `{user_code}` → `/oauth/device/result?status=ok`
5. 拒绝 → `POST /api/oauth/device/deny` `{user_code}` → `/oauth/device/result?status=denied`

### 注销

1. `POST /api/user/logout` → 后端清 `session` 与 `csrf_token` cookie → 200
2. SPA 跳 `/login`

## 错误处理

- **后端**：保持 `respond.go` 的 JSON envelope（`{error: {code, message}}`），新增 401 / 403 / 410（设备码过期）/ 409（已批准/拒绝）等明确语义
- **前端 client.ts**：非 2xx → 抛 `ApiError`；401 → 统一跳登录；其他错误由组件局部展示
- **Nginx 静态层**：除 SPA fallback 外不做 5xx 自定义页

## 测试

### 后端

- 现有 handler 单测继续通过（删除涉及模板渲染的 helper 后无遗漏）
- 新增 `device_flow.go` handler 单测：`info`、`approve`、`deny` 三个接口的成功 / 未找到 / 已过期 / 未登录路径
- CSRF middleware 单测：缺 header、token 不匹配、token 一致三类

### 前端

- vitest + React Testing Library：路由保护、`client.ts` 401 行为、登录表单
- 可选 Playwright E2E：`login → /me → logout` 一条主链路（落到 plan 阶段决定是否纳入首版）

### 联调

- 本地 `task dev`（后端）+ `pnpm dev`（前端）通过 Vite 代理验证
- 部署到 staging 后跑 `scripts/smoke-test.sh`（如需扩展则在 plan 阶段补）

## 工作量预估

| 模块 | 估时 |
|---|---|
| 后端：删模板 / 新加 3 接口 / CSRF MW / 登录响应改造 | 0.5 天 |
| 前端：脚手架 + 路由 + 4 个页面 + 通用组件 + 样式 | 1.5–2 天 |
| Nginx + deploy.sh | 0.5 天 |
| 联调 + 测试 | 0.5 天 |
| **合计** | **3–3.5 天** |

## 开放问题（plan 阶段定）

1. UI 视觉风格：是直接复刻当前 HTML 模板（保守），还是顺手做一次轻量重设计？
2. 是否引入组件库（Radix UI / shadcn）以加速开发？
3. CSRF token 轮转策略：每次登录新发，还是定期轮转？
4. Playwright E2E 是否纳入首版？
