# Server-Go 组织模型现状

> 目的：记录当前 `server-go` 中组织 ID / 组织信息的实际来源、存储位置和限制，作为后续组织模型改造的依据。

## 结论

`server-go` 当前有一个扁平的 `orgs` 表，并通过 `users.org_id` 给每个本地用户绑定一个组织；还没有多组织 membership 模型。

当前的组织信息主要是登录后构造进 token 的 identity claim：

- `personal_org_id`
- `orgs[].org_id`
- `orgs[].org_name`
- `orgs[].org_slug`
- `orgs[].role`

用户名密码登录签发 token 时，`personal_org_id` 和 `orgs[0].org_id` 来自 `users.org_id`，`orgs[0].org_name` 来自 `orgs.name`。默认/API key/本地兼容 subject 仍由环境变量构造。

### 数据库

组织和用户归属定义在 `server-go/internal/database/schema.sql` 与 migration `005_create_orgs`：

```sql
CREATE TABLE IF NOT EXISTS orgs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(128) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    ...
    org_id UUID NOT NULL
           DEFAULT '00000000-0000-0000-0000-0000000000a1'
           REFERENCES orgs(id) ON DELETE RESTRICT,
    ...
);
```

默认组织为 `00000000-0000-0000-0000-0000000000a1` / `研发`。

## 组织 ID 存储位置

### JWT claims

组织信息定义在 `server-go/internal/auth/jwt.go`：

```go
type Org struct {
	OrgID   string `json:"org_id"`
	OrgName string `json:"org_name"`
	OrgSlug string `json:"org_slug"`
	Role    string `json:"role"`
}

type TokenSubject struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	PersonalOrgID string `json:"personal_org_id"`
	Orgs          []Org  `json:"orgs"`
	Role          string `json:"role"`
}
```

签发 access token / refresh token 时，`TokenSubject` 会被复制到 JWT claims 中。服务端中间件再从 claims 中取出 `personal_org_id` 和 `orgs` 放到 request context。

### Device flow 临时记录

OAuth device flow 会把整份 `TokenSubject` 序列化到 `oauth_device_codes.subject_json`。

相关表字段：

```sql
CREATE TABLE IF NOT EXISTS oauth_device_codes (
    ...
    user_id TEXT,
    subject_json JSONB,
    ...
);
```

注意：这里的 `user_id` 存的是 `subject.Sub`，不是组织 ID。组织 ID 只存在于 `subject_json` 内部。

### 默认配置

默认 subject 的组织信息来自环境变量 / config：

```env
DEFAULT_PERSONAL_ORG_ID=git-ai-local-org
DEFAULT_ORG_NAME=Git AI Local
DEFAULT_ORG_SLUG=git-ai-local
```

这些配置用于构造默认用户/API key/兼容路径下的 token subject。

## 当前如何设置组织

### 用户名密码登录

普通用户登录时，`UserService.FindByUsernameOrEmail()` 会 JOIN `orgs`：

```go
SELECT u.id, ..., u.org_id::text, o.name, ...
FROM users u
JOIN orgs o ON o.id = u.org_id
WHERE u.username = $1 OR u.email = $1
```

`userToSubject()` 再把数据库组织写入 JWT：

- `personal_org_id = users.org_id`
- `orgs[0].org_id = users.org_id`
- `orgs[0].org_name = orgs.name`
- `orgs[0].org_slug = username`
- `orgs[0].role = users.role`

当前还没有用户组织修改接口，新用户默认进入默认组织。

### 默认/API key/本地兼容 subject

默认组织可以通过环境变量设置：

```env
DEFAULT_PERSONAL_ORG_ID=your-org
DEFAULT_ORG_NAME=Your Company
DEFAULT_ORG_SLUG=your-company
```

这会影响使用默认 `TokenSubject` 的路径，例如 API key subject、默认用户 subject、部分兼容/device flow fallback 逻辑。

### CLI device flow

CLI device flow 审批时会读取浏览器登录用户的 JWT claims，并复制成 `TokenSubject` 写入 `oauth_device_codes.subject_json`。

因此，如果浏览器登录的是普通用户，CLI 最终拿到的组织信息仍然是：

```text
org_id = users.org_id
org_slug = user.Username
```

## 当前限制

- 只有扁平 `orgs` 表，还没有多组织 membership 模型。
- 没有 `organization_memberships` 表。
- 没有组织创建、选择、切换、邀请或成员管理接口。
- `org_slug` 没有统一 slugify/格式化逻辑，登录路径直接使用 `username`。
- `/api/dashboard/global` 是跨用户、跨组织统计，当前仅 admin 可访问。
- dashboard 普通用户统计仍主要按 `user_id` 聚合，不是组织级视图。

## 后续改造方向

如果需要真正支持多组织，应在现有 `orgs` / `users.org_id` 基础上补 membership 和组织管理能力。

建议方向：

1. 新增 `organization_memberships` 表：
   - `organization_id`
   - `user_id`
   - `role`
   - `created_at`
   - `updated_at`

2. 扩展 `orgs`：
   - 增加 `slug`
   - 明确默认组织和个人组织语义

3. 修改登录逻辑：
   - `PersonalOrgID` 使用当前/默认组织 ID。
   - `Orgs` 使用真实 membership 列表。

4. 修改 API key/default subject 逻辑：
   - 明确 API key 代表的 user/org。
   - 避免仅依赖 `DEFAULT_PERSONAL_ORG_ID` 构造身份。

5. 增加组织管理接口：
   - 创建组织
   - 更新组织名称/slug
   - 添加/移除成员
   - 修改成员角色
   - 设置当前/默认组织

6. 审计下游查询：
   - 当前 metrics/authorship/dashboard 主要按 `user_id` 聚合。
   - 如需组织级视图，应新增 `org_id` 写入路径和查询过滤条件。
