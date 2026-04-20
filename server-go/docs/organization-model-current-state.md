# Server-Go 组织模型现状

> 目的：记录当前 `server-go` 中组织 ID / 组织信息的实际来源、存储位置和限制，作为后续组织模型改造的依据。

## 结论

`server-go` 当前没有真正的组织表，也没有持久化的组织成员关系模型。

当前的组织信息主要是登录后构造进 token 的 identity claim：

- `personal_org_id`
- `orgs[].org_id`
- `orgs[].org_name`
- `orgs[].org_slug`
- `orgs[].role`

其中 `org_id` 目前不是独立组织实体的主键，而是根据不同登录路径临时构造出来的值。

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
    user_id TEXT NOT NULL,
    subject_json JSONB NOT NULL,
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

普通用户登录时，组织信息由 `userToSubject()` 构造：

```go
PersonalOrgID: user.ID
Orgs: []auth.Org{
	{
		OrgID:   user.ID,
		OrgName: name,
		OrgSlug: user.Username,
		Role:    user.Role,
	},
}
```

也就是说：

- `personal_org_id = users.id`
- `orgs[0].org_id = users.id`
- `orgs[0].org_name = display_name 或 username`
- `orgs[0].org_slug = username`
- `orgs[0].role = users.role`

当前没有 API 或数据库字段可以给普通用户单独设置组织。

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
org_id = user.ID
org_slug = user.Username
```

## 当前限制

- 没有 `organizations` 表。
- 没有 `organization_memberships` 表。
- 没有组织创建、选择、切换、邀请或成员管理接口。
- `users` 表没有 `org_id` 字段。
- 普通用户的组织 ID 当前等同于用户 ID。
- `org_slug` 没有统一 slugify/格式化逻辑，登录路径直接使用 `username`。
- dashboard 只展示 token 中的第一个组织，不参与组织权限判断。

## 后续改造方向

如果需要真正支持组织，应新增持久化组织模型，并停止把 `user.ID` 当作组织 ID。

建议方向：

1. 新增 `organizations` 表：
   - `id`
   - `name`
   - `slug`
   - `created_at`
   - `updated_at`

2. 新增 `organization_memberships` 表：
   - `organization_id`
   - `user_id`
   - `role`
   - `created_at`
   - `updated_at`

3. 修改登录逻辑：
   - `userToSubject()` 从数据库读取用户所属组织。
   - `PersonalOrgID` 使用真实个人组织或默认组织 ID。
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
