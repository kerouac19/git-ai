# 私有化部署 Releases — 文件存储 + 最小一致性校验方案

> 目标：让客户端 `git-ai upgrade` 在配置 `api_base_url = 私服` 后能正常完成版本检测与安装。
>
> 方式：外部定时 shell 脚本从 GitHub 拉取 3 个元数据文件并自行生成 `current.json` 上传到私服；**私服只做文件存储 + 必要的写入一致性校验 + 读取 fallback**（不解析业务内容，但会校验校验链完整）；二进制仍由 GitHub 分发（客户端网络可达 GitHub）。

## 背景

### 客户端 `git-ai upgrade` 实际请求

来自 `src/commands/upgrade.rs`：

```text
1. GET /worker/releases                              → {version, checksum}
2. GET /worker/releases/:channel/download/SHA256SUMS  → 校验 SHA256 == checksum
3. GET /worker/releases/:channel/download/install.sh  → 校验 hash == SHA256SUMS 中的记录
4. bash install.sh                                    → install.sh 从 github.com 下载二进制
```

Windows 路径用 `install.ps1`。客户端从不向私服请求二进制。

### 首次安装与后续升级分工

- **首次安装**：用户照常运行 GitHub 上的 `install.sh`（curl one-liner）
- **后续升级**：用户 `git-ai config set api_base_url https://gitai.example.com` → `git-ai upgrade` 从私服走

install.sh 不修改，直接镜像。

---

## 架构

```text
GitHub Releases                  运维机（外部 cron）              私有服务器 (server-go)
┌──────────────────┐             ┌──────────────────┐            ┌─────────────────────┐
│ /releases/latest │  ① 查 tag   │ sync-releases.sh │  ③ PUT    │ /api/releases/      │
│  .tag_name       │◄────────────│ (每 15 分钟)     │───────────►│   :channel/         │
│  .published_at   │             │                  │            │   artifacts/:tag/   │
│                  │             │                  │            │   :name             │
│                  │  ② 下载     │ 1. 查 current    │            │                     │
│ /releases/       │◄────────────│ 2. 跳过或下载    │  ④ PUT    │ /api/releases/      │
│  download/v1.3.2/│   3 个文件   │ 3. 生成          │───────────►│   :channel/         │
│  ├─ SHA256SUMS   │             │    current.json  │            │   current.json      │
│  ├─ install.sh   │             │ 4. PUT 文件      │            │                     │
│  └─ install.ps1  │             │ 5. PUT current   │            │ {STORAGE}/          │
└──────────────────┘             └──────────────────┘            │   :channel/         │
                                                                   │   ├─ current.json   │
                                                                   │   └─ :tag/          │
                                                                   │      ├─ SHA256SUMS  │
                                                                   │      ├─ install.sh  │
                                                                   │      └─ install.ps1 │
                                                                   └──────────┬──────────┘
                                                                              │
         客户端 ◄────────────────────────────────────────────────────────────┘
         ⑤ GET /worker/releases                         ← 读 current.json
           GET /worker/releases/:ch/download/:name      ← 按 current.tag 读文件
           ⑥ install.sh 内部仍访问 github.com 拉二进制
```

---

## 目录结构

```text
{RELEASE_STORAGE_PATH}/
├── latest/
│   ├── current.json                  # {"tag":"v1.3.2","checksum":"...","updated_at":"..."}
│   ├── v1.3.2/
│   │   ├── SHA256SUMS
│   │   ├── install.sh                # GitHub 原件，零修改
│   │   └── install.ps1               # GitHub 原件，零修改
│   ├── v1.3.1/ ...
│   └── .tmp-v1.3.3-abc123/           # 上传中的临时目录
└── next/
    ├── current.json
    └── v1.3.4-rc1/ ...
```

- 二进制不存，由 `install.sh` 运行时从 github.com 下载
- 旧 tag 目录保留，运维手工清理

---

## 任务分解

### 任务 1：存储层 + 配置

**新增文件**：`server-go/internal/service/release_store.go`

**新增配置**（`internal/config/config.go`）：

| 变量 | 必须 | 默认值 | 说明 |
|---|---|---|---|
| `RELEASE_STORAGE_PATH` | 否 | `/opt/git-ai/releases` | 存储根 |
| `RELEASE_UPLOAD_TOKEN` | 生产必须 | 空 | 写入侧 Bearer token；空时写入端点返回 503 |

**服务接口**：

```go
type ReleaseStore struct { Root string }

// 写入侧：
//   - PutArtifact 只做字节读写，不解析内容
//   - PutCurrent  做最小一致性校验（解析 JSON 顶层字段 + 解析 SHA256SUMS 的 install.sh/install.ps1 条目），
//                 保证 current.json 指向的 release 对客户端的校验链完整
func (s *ReleaseStore) PutArtifact(channel, tag, name string, r io.Reader) error
func (s *ReleaseStore) PutCurrent(channel string, body []byte) error

// 读取侧
func (s *ReleaseStore) ResolveEffectiveChannel(requested string) (effective string, ok bool)
func (s *ReleaseStore) GetCurrentRaw(channel string) ([]byte, error)       // 返回 current.json 原始字节
func (s *ReleaseStore) OpenArtifact(channel, tag, name string) (io.ReadCloser, int64, error)
func (s *ReleaseStore) OpenCurrentArtifact(channel, name string) (io.ReadCloser, int64, error)
```

**PutArtifact 行为**（需保证并发下 release 不可变）：

1. 校验 `channel` / `tag` / `name` 合法
2. `mkdir -p {channel}/{tag}/`
3. 写入请求 body 到 `{channel}/{tag}/.tmp-{name}-{nonce}`
4. 若目标 `{channel}/{tag}/{name}` 已存在：
   - 读取并逐字节对比内容，一致 → 删除临时文件，返回 200（幂等重试）
   - 不一致 → 删除临时文件，返回 409 Conflict
5. 否则用 **`os.Link(tmp, target)`** 尝试以"不可覆盖"语义落盘：
   - 成功 → 删除临时文件，返回 200
   - 失败且错误为 `EEXIST`（并发竞争，另一个进程抢先落盘）→ 回到第 4 步的内容比较分支

> **不使用 `os.Rename`**：POSIX `rename` 会静默覆盖已存在的目标文件，两个并发上传同 `{tag}/{name}` 且内容不同时，第二个会直接覆盖第一个，绕过 409。`os.Link` 在目标已存在时返回 `EEXIST`，天然实现 "immutable create"。
>
> **平台支持**：Linux / macOS / 大多数 Windows 文件系统都支持 hard link；若部署环境有不支持 hard link 的奇特 FS，可退化为 `openat(O_CREAT|O_EXCL)` + 复制临时文件内容，实现代价相当。

**PutCurrent 行为**（强校验，确保客户端升级链路完整）：

1. 解析 body，取 `tag` / `checksum` 字段；`tag` 合法性校验
2. 验证 `{channel}/{tag}/` 下 3 个文件都存在（`SHA256SUMS` / `install.sh` / `install.ps1`）
3. 验证 `checksum` 字段等于 `SHA256({channel}/{tag}/SHA256SUMS)`
4. **严格按客户端语义解析 `SHA256SUMS`**：逐行 `strings.TrimSpace` 后用 **`strings.SplitN(line, "  ", 2)`**（**两个空格分隔符**，与客户端 `parse_checksums` 的 `line.split_once("  ")` 完全一致 —— `src/commands/upgrade.rs:345`）。确认存在 `install.sh` 和 `install.ps1` 两行，且各自 hash 等于服务端重算的对应文件 SHA256
5. 写 `{channel}/.current.tmp-{nonce}` → 原子 `rename` 到 `{channel}/current.json`

前 4 步中任意失败都不更新 `current.json`；这样客户端永远不会看到链路断裂的 release。

> **为什么解析格式必须严格对齐**：客户端只认两空格分隔。如果服务端用 `strings.Fields` / 正则 `\s+` / 单空格等更宽松语义，可能放行一个自身"看起来有 install.sh 条目"的 `SHA256SUMS`，但客户端 `parse_checksums` 看不到该条目 → `fetch_and_verify_install_script` 的 `checksums.get(script_name)` 返回 `None` → 升级失败。这是一个典型的双端解析不对称坏状态，必须在服务端实现时锁死格式。

**安全与正确性**：

- `channel` 必须属于 `{latest, next, enterprise-latest, enterprise-next}` allowlist
- `tag` 正则 `^[A-Za-z0-9._+-]+$`，并额外拒绝：`..`、`.` 开头、`-` 开头、空字符串、长度 > 64
- `name` 只能是 `SHA256SUMS` / `install.sh` / `install.ps1`
- 路径穿越防护：禁止 `/`、`\`、`..` 出现在 `tag` 和 `name` 中

**依赖**：无

---

### 任务 2：写入侧 API（供外部 shell 调用）

**新增文件**：
- `server-go/internal/handler/release_admin.go`
- `server-go/internal/middleware/upload_token.go`

**鉴权中间件**（~25 行）：

```go
func UploadTokenAuth(token string) gin.HandlerFunc {
    return func(c *gin.Context) {
        if token == "" {
            c.AbortWithStatusJSON(http.StatusServiceUnavailable,
                gin.H{"error": "release upload disabled: RELEASE_UPLOAD_TOKEN not set"})
            return
        }
        got := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
        if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
            c.AbortWithStatus(http.StatusUnauthorized); return
        }
        c.Next()
    }
}
```

不复用 `JWTAuthMiddleware` / `adminOnly()`，避免长期 JWT 签发机制。

**端点**：

> **路由形态说明**：在同一层级既注册 `:tag` 参数段又注册 `current.json` 静态段，会触发 Gin 的 wildcard/static sibling panic。因此 artifact 上传放到 `artifacts/` 静态前缀下，与 `current.json` 解耦。

#### PUT /api/releases/:channel/artifacts/:tag/:name

上传单个 artifact 文件（body 是文件原始字节）。

- 请求头：`Authorization: Bearer <token>`
- 限制：单文件 1 MiB。**实现必须用 `http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)` 包装请求体后再读**，避免客户端发超大 body 把进程内存打爆
- 超限响应：`413 Payload Too Large`
- 其他响应：`200 OK`（成功或幂等重传内容一致）、`409 Conflict`（内容不一致）、`400`（非法 channel / tag / name）

#### PUT /api/releases/:channel/current.json

上传/替换 channel 的当前指针。**这一步本身就是 promote**，无需额外接口。

- Body（`application/json`）：

```json
{
  "tag": "v1.3.2",
  "checksum": "<sha256 of SHA256SUMS>",
  "updated_at": "2026-04-20T00:00:00Z"
}
```

- 服务端校验（强校验，防止客户端升级链路被写坏）：
  - JSON 解析成功、`tag` 字段合法
  - `{tag}/` 下 3 个文件齐全（`SHA256SUMS` / `install.sh` / `install.ps1`）
  - `checksum` 等于服务端重算的 `SHA256({channel}/{tag}/SHA256SUMS)`
  - **解析 `SHA256SUMS`**，验证其中存在 `install.sh` 和 `install.ps1` 两行条目，且各自 hash 等于服务端重算的 `SHA256({channel}/{tag}/install.{sh,ps1})`
- 响应：`200 OK`、`400`（tag 下文件不齐 / SHA256SUMS 缺少脚本条目）、`409`（checksum 不匹配）

> **为什么要强校验到脚本级**：API 是通用上传口，token 持有者或合成测试数据可能 promote 一个客户端必失败的 release。客户端流程里会用 `SHA256SUMS` 中的条目校验 `install.sh` / `install.ps1`（见 `src/commands/upgrade.rs:395、415`），这两项对不上直接导致升级失败。服务端做这一步验证是廉价的，可以彻底闭合校验链。

#### GET /api/releases/:channel/current.json

幂等跳过辅助端点。返回当前 `current.json` 原始字节；没有则 `404`。

- 需要 Bearer token（与写入一致，防止暴露）

**依赖**：任务 1

---

### 任务 3：读取侧 API 改造

**重写文件**：`server-go/internal/handler/releases.go`

**Handler 依赖注入**：

```go
type ReleaseHandler struct { Store *service.ReleaseStore }
```

#### Channel fallback 统一策略（P0 必须覆盖所有读取路径）

客户端 `UpdateChannel` 已支持 `enterprise-latest` / `enterprise-next`（`src/config.rs:90`），若 `/worker/releases` 响应中没有对应字段会直接报 `Channel ... not found`（`src/commands/upgrade.rs:454`）。更关键的是，客户端拿到 `/worker/releases` 的 channel 结果后，**后续下载端点仍用请求时的原始 channel 字符串**（`src/commands/upgrade.rs:748、761`）。因此 fallback 必须在读列表和下载两条路径上应用同一套解析，否则第一跳 200、第二跳 503。

**抽一个统一的解析函数**：

```go
// 所有读取端点都走这一把解析：返回实际该读哪个 channel 目录
// 注意：requested 来自 URL 原始字符串，必须先 allowlist 再碰文件系统
func (s *ReleaseStore) ResolveEffectiveChannel(requested string) (effective string, ok bool) {
    if !knownReleaseChannel(requested) {
        return "", false
    }
    if s.hasCurrent(requested) { return requested, true }
    switch requested {
    case "enterprise-latest":
        if s.hasCurrent("latest") { return "latest", true }
    case "enterprise-next":
        if s.hasCurrent("next") { return "next", true }
    }
    return "", false
}

// 同时保留包级别的纯函数，便于 handler / store 共用，也便于单测表驱动
func knownReleaseChannel(c string) bool {
    switch c {
    case "latest", "next", "enterprise-latest", "enterprise-next":
        return true
    }
    return false
}
```

所有读取路径（`GET /worker/releases`、`GET /worker/releases/:channel/download/:name`）**必须**共享这个函数。`hasCurrent` 自身也应假定调用方已过 allowlist —— 这是纵深防御，避免未来有人把 `hasCurrent` 搬走或改造时丢掉 allowlist。

#### GET /worker/releases

遍历 4 个 channel，对每个请求 channel 跑 `ResolveEffectiveChannel`：

- 找到 effective → 读 `{effective}/current.json`，把 `tag` / `checksum` 填到**请求的 channel key** 下（不是 effective key）返回
- 没找到 → 该 channel 从响应中省略

| 请求的 channel | 私服有对应 `current.json` | 没有 |
|---|---|---|
| `latest` / `next` | 返回该 channel 数据 | 省略（预期失败） |
| `enterprise-latest` | 返回该 channel 数据 | 若 `latest` 有数据 → 用 latest 的 `version` / `checksum` 填到 `enterprise-latest` 返回 |
| `enterprise-next` | 返回该 channel 数据 | 若 `next` 有数据 → 用 next 的数据填到 `enterprise-next` 返回；都没有则省略 |

首版 shell 脚本不同步 enterprise channel，但响应层 + 下载层都做 fallback 兼容客户端。

响应结构保持不变（客户端契约）：

```json
{ "channels": { "latest": { "version": "v1.3.2", "checksum": "abc123..." } } }
```

无任何 channel 时返回 `{"channels": {}}`。

#### GET /worker/releases/:channel/download/:name

`:name` 只接受 `SHA256SUMS` / `install.sh` / `install.ps1`，其他 `404`。

实现：
1. `effective, ok := Store.ResolveEffectiveChannel(channel)`；`ok == false` 时 503
2. 读 `{effective}/current.json` 拿到 `tag`
3. 读 `{effective}/{tag}/{name}` 流式返回

**关键**：下载端点必须和 `/worker/releases` 共用同一个 `ResolveEffectiveChannel`。例如客户端请求 `enterprise-latest`，若私服只同步了 `latest`，第一跳会把 latest 数据以 `enterprise-latest` key 返回，第二跳 `GET /worker/releases/enterprise-latest/download/SHA256SUMS` 也必须解析到 `latest` 目录才能返回文件。否则客户端升级链路在第二跳断开。

Content-Type：
- `SHA256SUMS` → `text/plain; charset=utf-8`
- `install.sh` → `text/x-shellscript; charset=utf-8`
- `install.ps1` → `text/plain; charset=utf-8`

`current.json` 不存在（且 fallback 也无可用 channel）时返回 `503 Service Unavailable`（表达"私服尚未接收任何同步"）。

`current.json` 存在但目标文件缺失时返回 `500 Internal Server Error`（数据不一致，理论上被 `PutCurrent` 的强校验挡住）。

保留 `/worker/*` 和 `/workers/*` 双前缀注册。

**Promote race**（P2-5，首版接受）：

客户端先 `GET /worker/releases` 拿到 `checksum_A`，再 `GET SHA256SUMS`。如果 cron 正好在两次请求之间 promote 了新版本，客户端会用旧 checksum 校验新 SHA256SUMS，校验失败。**首版接受该窗口**，客户端会返回非零退出、下次重试即成功；无需引入 tag-pinned 下载或客户端改动。

**依赖**：任务 1

---

### 任务 4：路由注册

**修改文件**：`server-go/cmd/server/main.go`

```go
releaseStore := &service.ReleaseStore{Root: cfg.ReleaseStoragePath}
releaseH := &handler.ReleaseHandler{Store: releaseStore}
releaseAdminH := &handler.ReleaseAdminHandler{Store: releaseStore}
uploadAuth := middleware.UploadTokenAuth(cfg.ReleaseUploadToken)

// 读取侧：替换已有占位注册
for _, prefix := range workerRoutes {
    r.GET("/"+prefix+"/releases", releaseH.GetReleases)
    r.GET("/"+prefix+"/releases/:channel/download/:name", releaseH.Download)
}

// 写入侧
// 注意：artifact 上传用 /artifacts/:tag/:name 放在静态前缀下，
// 避免与 /current.json 在同一层级发生 Gin wildcard/static sibling 冲突
api.PUT("/releases/:channel/artifacts/:tag/:name", uploadAuth, releaseAdminH.PutArtifact)
api.PUT("/releases/:channel/current.json", uploadAuth, releaseAdminH.PutCurrent)
api.GET("/releases/:channel/current.json", uploadAuth, releaseAdminH.GetCurrent)
```

**依赖**：任务 1、任务 2、任务 3

---

### 任务 5：Shell 同步脚本

**新增文件**：`server-go/scripts/sync-releases.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail

: "${PRIVATE_SERVER:?}"            # e.g. https://gitai.example.com
: "${RELEASE_UPLOAD_TOKEN:?}"
REPO="${GITHUB_REPO:-git-ai-project/git-ai}"
AUTH_GH=()
[ -n "${GITHUB_TOKEN:-}" ] && AUTH_GH=(-H "Authorization: Bearer $GITHUB_TOKEN")

put() {
    local url="$1" file="$2" ctype="${3:-application/octet-stream}"
    # 注意：-f 与 --fail-with-body 互斥。保留 --fail-with-body 以便错误时看到 server 响应体
    curl -sSL --fail-with-body -X PUT \
        -H "Authorization: Bearer $RELEASE_UPLOAD_TOKEN" \
        -H "Content-Type: $ctype" \
        --data-binary "@$file" \
        "$url"
}

sync_channel() {
    local channel="$1" api="$2" filter="$3"

    local meta
    meta=$(curl -sfL "${AUTH_GH[@]}" "$api")
    local tag published
    tag=$(jq -r "$filter" <<< "$meta")
    [ -z "$tag" ] || [ "$tag" = "null" ] && { echo "$channel: no release"; return 0; }
    published=$(jq -r "$(sed 's/\.tag_name/.published_at/' <<< "$filter")" <<< "$meta")

    # 幂等跳过
    local cur
    cur=$(curl -sfL -H "Authorization: Bearer $RELEASE_UPLOAD_TOKEN" \
          "$PRIVATE_SERVER/api/releases/$channel/current.json" 2>/dev/null \
          | jq -r .tag 2>/dev/null || echo "")
    if [ "$cur" = "$tag" ]; then
        echo "$channel: already at $tag"; return 0
    fi

    local work; work=$(mktemp -d); trap "rm -rf '$work'" RETURN
    local base="https://github.com/$REPO/releases/download/$tag"

    # 1. 下载 3 个文件
    for f in SHA256SUMS install.sh install.ps1; do
        curl -sfL -o "$work/$f" "$base/$f"
    done

    # 2. 先 PUT artifact 文件（注意路径为 /artifacts/:tag/:name）
    for f in SHA256SUMS install.sh install.ps1; do
        put "$PRIVATE_SERVER/api/releases/$channel/artifacts/$tag/$f" "$work/$f"
    done

    # 3. 生成 current.json
    local checksum
    checksum=$(sha256sum "$work/SHA256SUMS" | awk '{print $1}')
    cat > "$work/current.json" <<EOF
{"tag":"$tag","checksum":"$checksum","updated_at":"$published"}
EOF

    # 4. 最后 PUT current.json（这一步 = promote）
    put "$PRIVATE_SERVER/api/releases/$channel/current.json" \
        "$work/current.json" "application/json"

    echo "$channel: synced $cur -> $tag"
}

sync_channel latest \
    "https://api.github.com/repos/$REPO/releases/latest" \
    '.tag_name'

sync_channel next \
    "https://api.github.com/repos/$REPO/releases?per_page=20" \
    '[.[] | select(.prerelease)] | first | .tag_name // empty'
```

Crontab：

```cron
*/15 * * * * /opt/git-ai/sync-releases.sh >> /var/log/git-ai-sync.log 2>&1
```

**依赖**：任务 4

---

### 任务 6：测试 + 冒烟

**Go 单测**：

- `ReleaseStore.PutArtifact` 正常路径写入并原子落盘
- 相同内容重传 200、不同内容重传 409
- **并发上传同 tag 同 name 不同内容**：双 goroutine 同时 `PutArtifact`，两者之一必然 200、另一方必然 409；落盘内容与 200 那一方一致（验证 `os.Link` 的不可覆盖语义）
- **SHA256SUMS 解析严格双空格**：提交一个 `hash\tinstall.sh`（tab 分隔）或 `hash install.sh`（单空格）的 SHA256SUMS，PUT current.json 应返回 400（与客户端 `split_once("  ")` 语义一致）
- **`ResolveEffectiveChannel` 拒绝非 allowlist**：传入 `"foo"` / `"../etc"` / `""` 返回 `ok=false`，不触达文件系统
- 非法 channel / tag（含 `..`、`.` 开头、`-` 开头、空、过长）被拒
- 未知 filename 返回 400
- **超过 1 MiB body 返回 413**（直接给 handler 发一个 >1MiB 的请求体）
- `PutCurrent` 在 artifact 不齐时 400
- `PutCurrent` 在 SHA256SUMS 缺 `install.sh` 条目时 400
- `PutCurrent` 在 SHA256SUMS 含 `install.sh` 但 hash 与实际文件不符时 400
- `PutCurrent` 在顶层 `checksum` 字段与实际 `SHA256SUMS` 不匹配时 409
- `GET /worker/releases` 没 `current.json` 时返回 `{"channels":{}}`
- `GET /worker/releases` 在仅有 `latest` 时 `enterprise-latest` fallback 到 `latest` 数据（data 取自 latest，key 填在 `enterprise-latest` 下）
- `GET /worker/releases` 在仅有 `next` 时 `enterprise-next` fallback 到 `next` 数据
- **`GET /worker/releases/enterprise-latest/download/SHA256SUMS` 在只有 `latest` 同步时，也能正确返回 latest 的文件**（第二跳 fallback）
- `GET /worker/releases/enterprise-next/download/install.sh` 在只有 `next` 同步时，返回 next 的文件
- `ResolveEffectiveChannel` 在所有 channel 均无数据时返回 `ok=false`
- `GET .../download/:name` 在无 fallback 可用时 503
- Gin 启动不 panic（路由无 wildcard/static sibling 冲突）

**smoke-test.sh 增补**（完整校验链）：

1. 合成 `install.sh` / `install.ps1` 测试内容，计算各自 SHA256
2. 构造 `SHA256SUMS` 包含 `install.sh` 和 `install.ps1` 两行真实 hash
3. PUT 3 个文件到 `/api/releases/latest/artifacts/vtest/{name}`
4. 算顶层 checksum = `sha256(SHA256SUMS 文件内容)` → 构造 `current.json` → PUT `/api/releases/latest/current.json`
5. `GET /worker/releases` 返回 `latest.version=vtest` 且 checksum 一致
6. `GET /workers/releases`（兼容 prefix）结果等同
7. 本地校验 `SHA256(下载的 SHA256SUMS) == releases.latest.checksum`
8. 下载 `install.sh`，校验其 hash 等于 `SHA256SUMS` 中记录（完整走一遍客户端校验链）
9. 下载 `install.ps1`，同上
10. 未带 token 访问任何 `/api/releases/...` 路径 → 401
11. 未设置 `RELEASE_UPLOAD_TOKEN` 启动时访问写入端点 → 503
12. 故意构造一个 `SHA256SUMS` 缺 `install.sh` 条目的上传，PUT current.json → 400
13. PUT 一个 > 1 MiB 的 body → 413
14. 客户端以 enterprise-latest channel 升级（只有 `latest` 被同步）跑通整条链：`GET /worker/releases/enterprise-latest/download/SHA256SUMS` 返回 latest 的 SHA256SUMS、hash 匹配、install 脚本也能下载

**端到端冒烟**：

```bash
# 服务端启动，export RELEASE_UPLOAD_TOKEN=test
./scripts/sync-releases.sh                    # 真跑一遍
git-ai config set api_base_url http://localhost:3000
git-ai upgrade --force                         # 验证升级流程
```

**依赖**：任务 4、任务 5

---

## 任务依赖图

```text
任务 1（存储层）
  ├── 任务 2（写入 API）──┐
  └── 任务 3（读取改造）──┼── 任务 4（路由注册）──► 任务 5（shell）──► 任务 6（测试）
```

---

## 环境变量汇总

### 服务端

| 变量 | 必须 | 默认值 | 说明 |
|---|---|---|---|
| `RELEASE_STORAGE_PATH` | 否 | `/opt/git-ai/releases` | 存储根 |
| `RELEASE_UPLOAD_TOKEN` | 生产必须 | 空 | 写入鉴权 Bearer token |

### Shell 脚本

| 变量 | 必须 | 说明 |
|---|---|---|
| `PRIVATE_SERVER` | 是 | 私服 URL |
| `RELEASE_UPLOAD_TOKEN` | 是 | 与服务端一致 |
| `GITHUB_REPO` | 否 | 默认 `git-ai-project/git-ai` |
| `GITHUB_TOKEN` | 否 | GitHub PAT，可选；匿名 60/hr 通常够用 |

---

## 客户端侧

**零改动**。用户操作：

```bash
# 首次（走 GitHub install.sh）
curl -fsSL https://github.com/git-ai-project/git-ai/releases/latest/download/install.sh | bash

# 切换到私服
git-ai config set api_base_url https://gitai.example.com

# 之后升级走私服
git-ai upgrade
```

`install.sh` 内部仍从 github.com 下载二进制 —— 需要客户端网络可达 GitHub。

---

## 关键约束与权衡

| 事项 | 决策 |
|---|---|
| 是否改写 install.sh / install.ps1 | **不改**，直接镜像 GitHub 原件 |
| 二进制是否镜像 | **否**，客户端从 GitHub 下载 |
| SHA256SUMS 是否重算 | **否**，信任 GitHub 原件 |
| `current.json` 由谁生成 | **shell**；服务端强校验（tag 文件齐全 + checksum + SHA256SUMS 中脚本条目一致）|
| 鉴权机制 | 单 Bearer token（`RELEASE_UPLOAD_TOKEN`），不复用 JWT / adminOnly |
| 同步模式 | 外部 cron shell，服务端不做主动拉取 |
| 写入粒度 | **PUT 逐文件**，最后 PUT `current.json` 作为 promote，无 commit 端点 |
| 写入路径形态 | `/api/releases/:channel/artifacts/:tag/:name` 与 `/api/releases/:channel/current.json` 分属两个静态前缀，避免 Gin wildcard/static sibling 冲突 |
| 企业 channel | 首版 shell 不同步 `enterprise-*`；服务端通过 `ResolveEffectiveChannel` 在 `/worker/releases` 与下载端点**两处**同时做 fallback（`enterprise-latest`→`latest`、`enterprise-next`→`next`），保证整条链路不在第二跳断开 |
| 同 tag 重传（含并发） | 内容一致 200，不一致 409；实现用 `os.Link(tmp, target)` 保证已存在目标不会被静默覆盖 |
| 写入 body 限制 | `http.MaxBytesReader` 强制单文件 1 MiB，超限 413 |
| Promote race | 客户端两跳请求之间 cron 切换版本可能导致校验失败；首版接受，客户端重试即成功 |
| air-gapped 客户端 | 不支持；客户端需访问 github.com |
| Gin 路由冲突 | 读取侧不引入 tag-pinned 下载端点；写入侧用 `artifacts/` 静态前缀；零冲突 |
| EXTERNAL_URL | 本方案不需要 |

---

## 工作量估算

| 模块 | 代码量 |
|---|---|
| `internal/service/release_store.go` | ~125 行（含 `ResolveEffectiveChannel` + `knownReleaseChannel` allowlist + 并发安全 `os.Link` + 严格双空格 SHA256SUMS 解析） |
| `internal/handler/release_admin.go` | ~70 行（含 `MaxBytesReader` 包装） |
| `internal/handler/releases.go` 重写 | ~80 行（两处读取共用 `ResolveEffectiveChannel`） |
| `internal/middleware/upload_token.go` | ~25 行 |
| `internal/config/config.go` 增补 | ~5 行 |
| `cmd/server/main.go` 路由注册 | ~15 行 |
| `scripts/sync-releases.sh` | ~60 行 shell |
| 单测 + 冒烟脚本 | ~260 行（新增并发、413、enterprise 二跳 fallback 三组用例） |
| **合计** | **~635 行** |
