# 登录功能实现文档

本文梳理当前登录功能的实现路径，覆盖前端入口、后端路由、密码登录、两步验证、第三方登录、Passkey 登录、会话鉴权与扩展点。

## 1. 总体架构

登录体系由前端 React 表单和后端 Gin API 共同完成：

- 前端入口：`web/src/components/auth/LoginForm.jsx`
- 前端路由：`web/src/App.jsx`
- OAuth 回调页：`web/src/components/auth/OAuth2Callback.jsx`
- API 封装：`web/src/helpers/api.js`
- 后端 API 路由：`router/api-router.go`
- 用户登录控制器：`controller/user.go`
- 2FA 控制器：`controller/twofa.go`
- OAuth 控制器：`controller/oauth.go`
- Passkey 控制器：`controller/passkey.go`
- 会话鉴权中间件：`middleware/auth.go`
- 用户模型：`model/user.go`

所有登录成功路径最终都会调用 `controller.setupLogin(user, c)`，写入后端 session 并返回前端需要的用户摘要。

## 2. 前端入口与展示逻辑

`/login` 路由由 `AuthRedirect` 包裹：

- 如果 `localStorage` 中已有 `user`，直接跳转 `/console`。
- 否则渲染 `LoginForm`。

`LoginForm` 会读取 `StatusContext` 或 `localStorage.status` 中的状态，决定展示哪些登录方式：

- `github_oauth`
- `discord_oauth`
- `oidc_enabled`
- `wechat_login`
- `linuxdo_oauth`
- `telegram_oauth`
- `passkey_login`
- `custom_oauth_providers`
- `turnstile_check`
- `user_agreement_enabled`
- `privacy_policy_enabled`

这些状态由后端 `GET /api/status` 返回，核心实现在 `controller/misc.go:GetStatus`。

如果存在第三方登录方式，默认先展示 OAuth/Passkey/Telegram/微信登录选项，并提供“使用 邮箱或用户名 登录”切换到密码登录表单。若没有第三方登录方式，则直接展示密码登录表单。

## 3. 密码登录流程

### 3.1 前端请求

用户提交用户名和密码后，`LoginForm.handleSubmit` 调用：

```http
POST /api/user/login?turnstile=<turnstileToken>
Content-Type: application/json

{
  "username": "alice",
  "password": "password"
}
```

前端在发起前会做这些检查：

- 如果启用了用户协议或隐私政策，必须先勾选同意。
- 如果启用了 Turnstile，必须先拿到 Turnstile token。
- 用户名和密码不能为空。

### 3.2 后端路由与中间件

路由定义在 `router/api-router.go`：

```go
userRoute.POST("/login", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Login)
```

含义：

- `CriticalRateLimit()`：限制关键接口请求频率。
- `TurnstileCheck()`：在启用 Cloudflare Turnstile 时校验 `turnstile` 查询参数，校验成功后会在 session 中写入 `turnstile=true`，后续同会话可复用。
- `controller.Login`：处理用户名密码认证。

### 3.3 用户校验

`controller.Login` 的主要步骤：

1. 检查 `common.PasswordLoginEnabled`，未启用则返回错误。
2. 读取 JSON body 到 `LoginRequest`。
3. 校验 `username`、`password` 非空。
4. 构造 `model.User{Username, Password}` 并调用 `ValidateAndFill()`。

`model.User.ValidateAndFill()` 会：

- 对 `username` 做 `strings.TrimSpace`。
- 按 `username = ? OR email = ?` 查询用户。
- 使用 `common.ValidatePasswordAndHash` 校验密码。
- 要求用户状态为 `common.UserStatusEnabled`。

密码哈希使用 `bcrypt`：

- 创建或修改密码：`common.Password2Hash`
- 校验密码：`common.ValidatePasswordAndHash`

## 4. 登录成功与会话写入

认证通过后，如果用户未启用 2FA，后端调用：

```go
setupLogin(&user, c)
```

该函数写入 session：

- `id`
- `username`
- `role`
- `status`
- `group`

然后返回前端用户摘要：

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 1,
    "username": "alice",
    "display_name": "alice",
    "role": 1,
    "status": 1,
    "group": "default"
  }
}
```

注意：返回内容刻意不包含 `password`、`access_token` 等敏感字段。

前端收到成功响应后会：

- `userDispatch({ type: 'login', payload: data })`
- `setUserData(data)`，将用户摘要写入 `localStorage.user`
- `updateAPI()`，刷新 Axios 实例中的 `New-API-User` 请求头
- 跳转 `/console`

## 5. Session 与后续鉴权

后端在 `main.go` 中初始化 cookie session：

```go
store := cookie.NewStore([]byte(common.SessionSecret))
store.Options(sessions.Options{
    Path:     "/",
    MaxAge:   2592000,
    HttpOnly: true,
    Secure:   false,
    SameSite: http.SameSiteStrictMode,
})
server.Use(sessions.Sessions("session", store))
```

会话有效期为 30 天，Cookie 名称为 `session`。

受保护 API 使用 `middleware.UserAuth()`、`AdminAuth()` 或 `RootAuth()`。核心逻辑在 `authHelper`：

1. 优先读取 session 中的 `username`、`role`、`id`、`status`。
2. 如果没有 session 用户，尝试读取 `Authorization` 请求头并调用 `model.ValidateAccessToken()`。
3. 要求请求头包含 `New-Api-User`，并且该值必须与 session 或 access token 对应的用户 ID 一致。
4. 校验用户未封禁、角色满足最小权限要求。
5. 将 `username`、`role`、`id`、`group`、`user_group`、`use_access_token` 写入 `gin.Context`。

前端 `updateAPI()` 会从 `localStorage.user.id` 生成 `New-API-User` 请求头。Go 的 HTTP header 名大小写不敏感，所以后端读取 `New-Api-User` 可以匹配前端的 `New-API-User`。

## 6. 2FA 登录流程

密码校验通过后，`controller.Login` 会调用：

```go
model.IsTwoFAEnabled(user.Id)
```

如果用户启用了 2FA，后端不会立即登录，而是写入 pending session：

- `pending_username`
- `pending_user_id`

并返回：

```json
{
  "success": true,
  "message": "需要两步验证",
  "data": {
    "require_2fa": true
  }
}
```

前端检测 `data.require_2fa` 后展示 `TwoFAVerification` 弹窗。用户输入 6 位 TOTP 或 8 位备用码后请求：

```http
POST /api/user/login/2fa
Content-Type: application/json

{
  "code": "123456"
}
```

后端 `Verify2FALogin` 会：

1. 从 session 读取 `pending_user_id`。
2. 查询用户与 2FA 记录。
3. 优先按 TOTP 验证并更新使用记录。
4. TOTP 失败时尝试备用码验证并更新使用记录。
5. 验证成功后删除 pending session。
6. 调用 `setupLogin(user, c)` 完成正式登录。

## 7. 标准 OAuth 登录流程

标准 OAuth 入口覆盖 GitHub、Discord、OIDC、LinuxDO 和自定义 OAuth provider。

### 7.1 前端发起

点击第三方登录按钮时，前端先调用：

```http
GET /api/oauth/state
```

后端 `GenerateOAuthCode` 生成随机 `state`，写入 session 的 `oauth_state`，并返回给前端。若存在邀请参数 `aff`，也会写入 session。

随后前端跳转到对应 provider 的授权地址，例如：

- GitHub：`https://github.com/login/oauth/authorize?...`
- Discord：`https://discord.com/oauth2/authorize?...`
- OIDC：由后端状态返回的 `oidc_authorization_endpoint`
- LinuxDO：`https://connect.linux.do/oauth2/authorize?...`
- 自定义 OAuth：`custom_oauth_providers[].authorization_endpoint`

### 7.2 前端回调页

OAuth provider 回调到前端路由：

- `/oauth/github`
- `/oauth/discord`
- `/oauth/oidc`
- `/oauth/linuxdo`
- `/oauth/:provider`

`OAuth2Callback` 读取 URL 中的 `code` 和 `state`，再请求后端：

```http
GET /api/oauth/<provider>?code=<code>&state=<state>
```

### 7.3 后端回调处理

`controller.HandleOAuth` 的核心流程：

1. 根据 `:provider` 从 `oauth` registry 获取 provider。
2. 校验 `state` 与 session 中的 `oauth_state` 一致，用于 CSRF 防护。
3. 如果 session 中已有 `username`，进入账号绑定流程 `handleOAuthBind`。
4. 校验 provider 已启用。
5. 用 `code` 交换 provider token。
6. 拉取 OAuth 用户信息。
7. 调用 `findOrCreateOAuthUser` 查找或创建本地用户。
8. 校验用户状态未禁用。
9. 调用 `setupLogin(user, c)` 完成登录。

`findOrCreateOAuthUser` 会优先按 provider user ID 查找已有绑定；找不到时，如果允许注册，则创建普通用户并写入 provider 绑定。自定义 OAuth provider 使用 `user_oauth_bindings` 表；内置 provider 会写到 `users` 表对应字段，如 `github_id`、`discord_id`、`oidc_id`、`linux_do_id`。

## 8. 微信与 Telegram 登录

微信和 Telegram 是非标准 OAuth 路由，仍然复用 `setupLogin` 完成登录。

微信登录：

- 前端展示公众号二维码，用户输入验证码。
- 前端请求 `GET /api/oauth/wechat?code=<code>`。
- 后端通过配置的微信服务换取 `wechatId`。
- 如果 `wechatId` 已绑定，加载对应用户。
- 如果未绑定且允许注册，创建 `wechat_<id>` 用户。
- 校验用户状态后调用 `setupLogin`。

Telegram 登录：

- 前端使用 `react-telegram-login` 获取 Telegram 回传字段。
- 前端请求 `GET /api/oauth/telegram/login`，携带 Telegram 字段。
- 后端用 bot token 校验 HMAC。
- 按 `telegram_id` 查询用户并调用 `setupLogin`。

## 9. Passkey 登录流程

Passkey 登录使用 WebAuthn 的 discoverable login。

前端点击 Passkey 登录后：

1. 请求 `POST /api/user/passkey/login/begin`。
2. 后端 `PasskeyLoginBegin` 检查系统是否启用 Passkey。
3. 后端构建 WebAuthn 配置并调用 `BeginDiscoverableLogin()`。
4. 后端把 WebAuthn session data 保存到 session key `passkey_login_session`。
5. 前端调用 `navigator.credentials.get({ publicKey })`。
6. 前端把 assertion 转为后端 payload。
7. 请求 `POST /api/user/passkey/login/finish`。

`PasskeyLoginFinish` 会：

1. 读取并删除 `passkey_login_session`。
2. 按 credential ID 找到 Passkey 凭证。
3. 加载对应用户，并校验用户未禁用。
4. 校验 userHandle 与用户 ID 是否匹配。
5. 完成 WebAuthn 认证。
6. 更新凭证 `LastUsedAt`。
7. 调用 `setupLogin(modelUser, c)`。

## 10. 退出登录

退出登录接口：

```http
GET /api/user/logout
```

后端 `controller.Logout` 调用：

```go
session.Clear()
session.Save()
```

前端常见退出路径会清理 `localStorage.user` 并调用 `updateAPI()`，避免继续发送旧的 `New-API-User`。

## 11. Access Token 登录补充

`GET /api/user/token` 可为当前登录用户生成管理用 `access_token`：

- 路由位于 `/api/user/token`
- 需要 `middleware.UserAuth()`
- 后端生成随机 key 后保存到 `users.access_token`

后续受保护接口如果没有 session，可以通过 `Authorization: Bearer <access_token>` 进入 `authHelper` 的 fallback 分支。但仍然需要提供匹配的 `New-Api-User` 请求头。

注意：这里的 `access_token` 是系统管理用户 token，不是 `tokens` 表里的 API key，也不是 OAuth provider 返回的 access token。

## 12. 扩展登录方式时的关键点

新增登录方式建议遵循以下约定：

1. 登录成功后统一调用 `setupLogin(user, c)`，保持 session 和前端数据结构一致。
2. 返回给前端的用户数据只包含必要摘要，不返回密码、provider token、系统 access token 等敏感信息。
3. 如果有重定向登录流程，必须使用 session 保存随机 `state` 或等价的一次性挑战值。
4. 如果允许自动注册，需要遵守 `common.RegisterEnabled`。
5. 创建用户时应复用 `model.User.Insert` 或 `InsertWithTx`，确保密码哈希、默认额度、邀请奖励和默认设置逻辑一致。
6. 受保护 API 需要前端 `localStorage.user.id` 与后端 session 用户 ID 一致，否则 `authHelper` 会拒绝请求。

## 13. 当前实现注意事项

- 后端 session cookie 当前 `Secure` 设置为 `false`。如果部署在 HTTPS 生产环境，建议评估是否应按部署环境启用 `Secure`。
- `SameSite` 为 `Strict`，对同站前端回调到后端 API 是可行的；如果跨站部署前后端，需要重新评估 cookie 策略。
- 密码登录、注册和 Turnstile 都由系统配置开关控制，前端展示依赖 `/api/status`，后端仍会在接口层做最终校验。
- 前端的路由守卫主要依赖 `localStorage.user` 做页面级判断；真正权限校验在后端中间件。
