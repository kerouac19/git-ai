# Releases Upgrade Plan 评估报告

> 评审对象：`server-go/docs/releases-upgrade-plan.md`
> 评审依据：
> - `server-go/internal/handler/releases.go`
> - `server-go/cmd/server/main.go`
> - `server-go/internal/config/config.go`
> - `install.sh` / `install.ps1`（项目根）
> - `src/commands/upgrade.rs`（Rust 客户端）

---

## 总体评价

**可行、结构完整、对现有契约理解基本到位**。任务拆分粒度合理，依赖关系清晰，安全与原子性约束都提到了。可以作为实现依据，但存在若干**可执行性细节**和**边界 case** 没有明确，下面按严重程度分类。

**计划质量：B+**

---

## 一、与现状完全对齐的部分（无问题）

1. **客户端契约**与 `upgrade.rs` 源码逐条对上：
   - `ReleasesResponse { channels: { version, checksum } }`（`upgrade.rs:100-109`）
   - `/worker/releases/{channel}/download/SHA256SUMS`（`upgrade.rs:288`）
   - `SHA256(SHA256SUMS) == checksum`（`fetch_and_verify_checksums`）
   - Unix → `install.sh`，Windows → `install.ps1`（`upgrade.rs:317-319`）
   - 解析格式 `"<hash>  <filename>"`（两个空格）
2. **channel 列表** 与 `releases.go:13` 的 `releaseChannels` 一致。
3. **双 prefix `/worker/*` 和 `/workers/*`** 确实已在 `main.go:138-150` 注册。
4. **无需修改 Rust 客户端** —— `upgrade.rs` 的 `api_base_url` 来自配置，对换服务端不感知。
5. **原子 rename + 临时目录** 的存储设计是正确路径。
6. **明确禁止用 `CORS_ORIGIN` 推断下载地址** —— 这是正确判断；`CORS_ORIGIN` 默认是 `http://localhost:3000`，用它构造下载链接会把生产搞挂。

---

## 二、阻塞性或需要在实现前解决的问题（P0）

### P0-1：渲染后的 `install.sh` / `install.ps1` **不是** "基于项目根模板改几个占位符" 那么简单

项目根的 `install.sh` / `install.ps1` 把下载 URL 硬编码为 GitHub：

```bash
# install.sh:224-239
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${RELEASE_TAG}/${BINARY_NAME}"
```

```powershell
# install.ps1:396-407
$downloadUrlExe = "https://github.com/$Repo/releases/download/$releaseTag/$binaryName.exe"
```

计划里只说"占位符替换 + 下载 URL 规则"，但**没有说明这些 URL 分支需要彻底重写**。模板必须：

- 删除 `GIT_AI_LOCAL_BINARY`、GitHub fallback、`GIT_AI_RELEASE_TAG` 环境变量路径分支（或明确保留哪些）；
- 插入 `PRIVATE_API_BASE` / `PRIVATE_CHANNEL` **新变量**（现有脚本没有这些变量，不只是值替换）；
- 私有部署模板要决定是否保留安装 token 交换（`INSTALL_NONCE` / `API_BASE` / `exchange-nonce`）。

**建议**：计划里要么明确"复制并改造"，要么在 Task 2 列出模板与原脚本的 diff 范围，避免实现时扯皮。

### P0-2：Gin 路由可能冲突

计划同时注册：

```
GET /worker/releases/:channel/download/:name
GET /worker/releases/:channel/:tag/download/:name   ← 新增
```

Gin 的 radix 路由在同一位置**同时接受 `download` 字面量和 `:tag` 参数**会 panic（历史上多次出现类似 issue）。需要验证或改路径结构，例如：

```
GET /worker/releases/:channel/download/:name        （保留）
GET /worker/releases/:channel/tag/:tag/download/:name   （新路径，避免冲突）
```

或在注册顺序上做合并 handler、通过 `:name` 里判断。**计划里必须选择一个方案并固化**，否则实现时会返工。

### P0-3：Admin 上传的鉴权方式没讲清楚

计划写 `Authorization: Bearer {admin JWT}` 并"复用 `JWTAuthMiddleware` + `adminOnly()`"。但现状：

- `adminOnly()` 定义在 `cmd/server/main.go:525-541`，不在包外可复用——要么提取到 `middleware` 包，要么在 `main.go` 中直接注册 `release_admin.go` 的路由。
- `JWTAuthMiddleware` 期望的是 session access token（短期、从 login / device flow 取得）。脚本里用 `GIT_AI_ADMIN_TOKEN` 存"静态 token"这个概念，当前系统**没有**长期有效 admin token 的签发流程（既不是 API key，也不是 PAT）。

**两个方向必须选一个**：

1. 引入长期 admin API token（新机制）。
2. 脚本先用 username/password 登录拿短期 JWT、再上传（多一步但复用现有）。

计划里含糊带过，实现阶段会踩坑。

### P0-4：`EXTERNAL_URL` 必要性条件不严谨

计划说"生产环境必须显式设置 `EXTERNAL_URL`"。问题：

- 现有 `config.go` 没有这个字段，要加 viper 默认值和 `AppEnv == "production"` 校验。
- **何时必须**？如果这台服务端从来不调用 `POST /api/releases/:channel`（即不作为 release publisher），那 `EXTERNAL_URL` 是否仍然必填？计划模糊。建议：只在首次 `SaveRelease` 时校验，或在启动时基于环境做 warning。

---

## 三、需要补充的边界 case（P1）

### P1-1：同 tag 重复上传语义

`rename(tmp, {tag}/)` 遇到已存在的 `{tag}/` 会失败。计划没写：

- 拒绝？(`409 Conflict`) — 推荐，符合"release 不可变"的语义。
- 覆盖？— 破坏客户端已缓存的 checksum。

**必须在 Task 3 响应表里写死**。

### P1-2：客户端 checksum race

客户端流程：
1. GET `/worker/releases` → `checksum_A`
2. GET `SHA256SUMS` → 服务端在中间时刻被 publish 新 release → `SHA256_B ≠ checksum_A` → 升级失败

概率低但真实存在。可以在 Task 4 的 `SHA256SUMS` handler 里：
- 记住本次读取的 `current.json`；
- 或支持客户端带 `?tag=vX` 参数锁定版本（上面 P0-2 里 tag 绑定路径已经覆盖部分）。

计划可以明确写"首版接受该 race，发生时客户端重试即可"。

### P1-3：模板 drift / 单点失配风险

`server-go/internal/templates/install.sh` 和项目根 `install.sh` 是**两份几乎重复的脚本**。一旦某天项目根脚本修复 bug（比如处理 shell 检测的 edge case），很容易忘了同步。**计划要说清策略**：

- a) 手工同步（写 CODEOWNERS/CHECKLIST）；
- b) CI 用 sed/脚本从根脚本生成 templates（差异只在占位符与下载逻辑片段）；
- c) 干脆让 server 动态 `sed` 项目根脚本。

推荐 (b)。

### P1-4：`install.sh` 里的 `INSTALL_NONCE` / `exchange-nonce`

项目根脚本有登录 nonce 交换逻辑（在私有部署场景里通常不需要）。模板是否保留？计划没提。需要决定。

### P1-5：Tag 合法性校验

`^[A-Za-z0-9._+-]+$` 允许 `..` 绕过吗？检查用的是字符集，`..` 由两个 `.` 组成，**能通过**。必须在 `tag` 上额外拒绝 `..`、`.` 开头、空字符串。

---

## 四、建议补充的内容（非阻塞）

1. **回滚端点**：`POST /api/releases/:channel/rollback { tag }` 改 `current.json`。非常便宜，值得一起做。
2. **保留策略**：`releases/{channel}/` 下旧 tag 无限累积；可以在 Task 1 里至少记 TODO。
3. **release `list` 端点**：`GET /api/releases/:channel` 返回历史 tag 列表，运维视觉化时有用。
4. **Metrics**：release 下载量可以简单埋点，放在后续任务。
5. **Task 0 文字**：说"客户端无需修改"但又列 `src/commands/upgrade.rs` 为涉及文件，读起来矛盾。建议改"涉及文件（仅用于契约校对，不修改）"。

---

## 五、优先级排序（实施顺序修正建议）

原计划 0→1→{2,3}→{4,5}→6 基本合理，但建议：

- **先做 Task 2 的模板渲染原型**（单元测试即可），因为 P0-1 的复杂度是整个项目最不确定的点；
- **早期就决定 P0-3 的鉴权方案**，否则 Task 3 和 Task 5 都卡住；
- Task 6 里加入**专门的 E2E 冒烟**：`publish-release.sh → git-ai upgrade --force`，跑真实 daemon 重启链路。

---

## 附：待决策清单（Open Questions）

| # | 问题 | 需决策 |
|---|---|---|
| 1 | 安装脚本模板与项目根脚本的关系 | 复制/生成/动态 sed 三选一 |
| 2 | `GET :channel/:tag/download/:name` 路由形式 | 是否采用 `/tag/:tag/` 前缀避免 Gin 冲突 |
| 3 | Admin 鉴权 | 长期 API token vs. 登录拿短 JWT |
| 4 | `EXTERNAL_URL` 启动时是否强校验 | 强制 vs. 仅上传时校验 |
| 5 | 同 tag 重复上传 | 409 拒绝 vs. 覆盖 |
| 6 | 模板是否保留 `INSTALL_NONCE` 交换 | 保留 / 移除 |
| 7 | `tag` 额外拒绝规则 | `..`、`.` 开头、空值等 |

---

## 附录 A：替代方案评估 —— GitHub 定时同步模式

### 背景

原计划采用"管理员手动上传 + publish 脚本"模式。后续评审提出问题：是否可以让服务端自己定时从 GitHub 拉取最新构建产物，客户端直接 `git-ai upgrade`？结论是**显著更省工作量，推荐切换到此方案**。

### 与原方案的工作量对比

| 原方案模块 | 新方案是否仍需 | 说明 |
|---|---|---|
| Task 1 存储层 | 需要 | 结构基本不变，调用方从 admin API 改为 sync job |
| Task 2 脚本模板渲染 | 需要 | 安装脚本仍要指向私有服务器下载二进制，否则私有部署无意义 |
| Task 3 Admin 上传 API | **完全删除** | 彻底消除 P0-3 鉴权难题 |
| Task 4 公开端点 | 需要 | 基本不变 |
| Task 5 发布脚本 | **完全删除** | 由 sync job 替代 |
| Task 6 测试 | 简化 | 只需测 sync → serve 链路 |
| 新增：sync job | 新增 | 单文件 ~200 行 |

净工作量估计减少 30-40%，并消除最不确定的 P0-3（admin token 签发）。

### 可行性验证（已通过代码确认）

1. **GitHub Release 资产命名已对齐**：`.github/workflows/release.yml` 里 artifact 名直接就是 `git-ai-linux-x64`、`git-ai-macos-arm64`、`git-ai-windows-x64.exe` 等，零映射成本
2. **Channel 映射天然存在**：workflow 有 `prerelease` 标志（`release.yml:277, 420`），GitHub "latest stable" → `latest` channel、`prerelease == true` → `next` channel
3. **API 限流可接受**：GitHub 匿名 60 次/小时；配置只读 token 后 5000 次/小时。每 5-10 分钟轮询一次远远够用

### 架构示意

```text
                  ┌────────────────────────────────────────┐
                  │          私有服务器 (server-go)         │
                  │                                        │
  cron (10min)    │   ┌──────────────┐                     │
  ───────────────►│   │  sync job     │                    │
                  │   │  (goroutine) │                     │
                  │   └──────┬───────┘                     │
                  │          │ 1. GET api.github.com/releases
                  │          │ 2. 比较 tag vs current.json
                  │          │ 3. 下载缺失的 artifact
                  │          │ 4. SaveRelease(channel, tag, ...)
                  │          ▼                             │
                  │   ┌──────────────┐                     │
                  │   │  存储层       │                     │
                  │   └──────┬───────┘                     │
                  │          │                             │
                  │   ┌──────▼───────┐                     │
  客户端          │   │ releases     │                     │
  ───────────────►│   │  handler     │                     │
                  │   └──────────────┘                     │
                  └────────────────────────────────────────┘
```

### 新方案的任务依赖图

```text
任务 0：契约校准（同前）
  └── 任务 1：存储层（同前，调用方改为 sync job）
        ├── 任务 2：脚本模板（同前，P0-1 依然适用）
        │     └── 任务 4：公开 releases 端点（同前，P0-2 依然适用）
        └── 任务 3（新）：GitHub sync job
              ├── 任务 5（新，可选）：手工触发 / 状态查询端点
              └── 任务 6：测试与冒烟
```

### 任务 3（新）：GitHub sync job 细节

- `internal/service/github_sync.go`：
  - `SyncOnce(ctx)`：拉 `GET /repos/{owner}/{repo}/releases`（分页，首页足够）
  - 对每个 release：按 `prerelease` 标志选 channel；对比 `current.json.tag`
  - 新版本 → 并行下载 `git-ai-*` → 调 `SaveRelease`（`SHA256SUMS` 由服务端自己生成，保证格式与校验链一致）
- `cmd/server/main.go`：启动 goroutine + `time.NewTicker(interval)`，SIGTERM 优雅退出
- 可选：`POST /api/releases/sync` 管理员手动触发（仍需鉴权，但不再是阻塞项）

### 新方案的新增待决策项

| # | 问题 | 建议 |
|---|---|---|
| N1 | 纯 pull 模式 vs. 双模式共存（保留 admin 上传） | 纯 pull 模式足够覆盖 99% 场景；air-gapped 场景再加 admin API 作为 v2 |
| N2 | `enterprise-latest` / `enterprise-next` 怎么办 | A：只同步 `latest` / `next`，企业 channel 返回 404 或 fallback；B：企业版由独立 repo 提供（超出当前范围） |
| N3 | GitHub token 配置 | 新增 `GITHUB_TOKEN`（可选），匿名时走 60/hr |
| N4 | Sync 周期 | `SYNC_INTERVAL_MINUTES`，默认 10 |
| N5 | 首次启动未同步时 `/worker/releases` 响应 | 首版返回 503 或保留原 env 占位逻辑，日志明确 |
| N6 | sync 失败重试策略 | 指数退避 + 暴露 `last_sync_error` 到 `/health` |
| N7 | 同 tag 在 GitHub 被重打包 | 对比 checksum，变化则覆盖并记录 warning（实际不应发生） |
| N8 | 企业网络出口限制 | 文档说明需访问 `api.github.com` + `objects.githubusercontent.com` |

### 原计划中哪些问题被自动消除

| 原编号 | 原问题 | 新方案状态 |
|---|---|---|
| P0-3 | Admin 上传鉴权方案未定 | **消除**：没有 admin 上传 |
| P1-1 | 同 tag 重复上传语义 | 简化：sync job 内部行为由 N7 决定，外部不再有上传端点 |
| 建议补充.1 | 回滚端点 | 仍有价值（切回旧 channel tag）；但不阻塞 |

### 仍然保留的原评审结论

- P0-1（脚本模板重写范围）：完全适用
- P0-2（Gin 路由冲突）：完全适用
- P0-4（`EXTERNAL_URL` 校验时机）：完全适用
- P1-2（客户端 checksum race）：仍存在，sync job 更新 `current.json` 时同样有窗口
- P1-3（模板 drift）：完全适用
- P1-4（`INSTALL_NONCE`）：完全适用
- P1-5（`tag` 字符集）：仍需实施，sync job 接收到的 tag 同样要校验

### 需用户确认的事项（阻塞重写计划文档）

1. **是否完全放弃 admin 上传？** 还是 pull 为主 + admin API 作为备选？
2. **企业 channel 如何处理？**（N2 的 A/B 方案）
3. **air-gapped 场景是否必须支持？** 需要则保留 admin 上传兼容路径

确认后可将 `releases-upgrade-plan.md` 按新方案重写。
