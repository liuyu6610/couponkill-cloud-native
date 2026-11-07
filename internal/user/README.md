# user 服务（:8081）

注册、登录、登出、当前用户资料、压测造用户，以及供其他服务取用户展示名的内部端点。
规格见 `docs/GO-REWRITE-DESIGN.md` §5、§10；数据在 PostgreSQL 的 `usr` schema（`internal/user/migrations`）。

分层：`handler`（HTTP 适配）→ `service`（业务规则，自己定义 `Repo`/`Sessions`/`Tokens`/`PasswordHasher`/`IDGenerator` 接口）→ `repo`（pgx）。
`internal/user/server.go` 负责装配，`cmd/user/main.go` 只做进程级接线。`usertest` 是 handler 测试用的内存仓储。

## 配置（环境变量，设计 §3）

| 变量 | 默认 | 说明 |
|---|---|---|
| `CK_ENV` | `dev` | `prod` 时密钥缺失或等于开发默认值会启动失败；同时禁止演示种子 |
| `CK_HTTP_PORT` | `8081` | 监听端口 |
| `CK_LOG_LEVEL` | `info` | `debug/info/warn/error` |
| `CK_WORKER_ID` | `-1` | 雪花节点号 0..1023，`-1` 按主机名哈希；多副本生产环境请显式给不同值，否则可能 ID 碰撞（碰撞表现为主键冲突 → 500，不会覆盖数据） |
| `CK_PG_DSN` | 开发默认 | PostgreSQL 连接串（敏感） |
| `CK_REDIS_ADDR` / `CK_REDIS_PASSWORD` / `CK_REDIS_DB` | `127.0.0.1:6379` / 空 / `0` | 登录会话 `ck:sess:{jti}` 存放处 |
| `CK_JWT_SECRET` | 开发默认 | HS256 密钥，≥32 字节，必须与网关一致 |
| `CK_JWT_ISSUER` / `CK_JWT_AUDIENCE` | `couponkill` / `couponkill-api` | 必须与网关一致 |
| `CK_JWT_TTL` | `24h` | 令牌与会话寿命 |
| `CK_INTERNAL_TOKEN` | 开发默认 | `/internal/v1/**` 的 `X-Internal-Token` |
| `CK_GATEWAY_SIGN_KEY` | 开发默认 | 校验网关身份签名，≥32 字节，必须与网关一致 |
| `CK_USER_SEED_DEMO` / `CK_SEED_DEMO` | `false` | 任一为 `true` 时启动写入演示用户（幂等）；`CK_ENV=prod` 下请求种子会直接启动失败 |

演示用户（公开测试口令，不是秘密）：`demo / demo1234`（普通用户）、`admin / admin1234`（管理员）。

## 启动

```bash
# 本地：需要 PostgreSQL（已建库）与 Redis
export CK_PG_DSN='postgres://couponkill:couponkill@127.0.0.1:5433/couponkill?sslmode=disable'
export CK_REDIS_ADDR=127.0.0.1:6379
export CK_USER_SEED_DEMO=true
go run ./cmd/user
```

启动顺序：加载配置 → `dbx.Open` → `dbx.Migrate(usr)`（advisory lock 下，多副本安全）→（可选）种子 → 路由 → `app.Run`。
`/healthz` 存活，`/readyz` 检查 PostgreSQL 与 Redis，`/metrics` Prometheus。

## 身份与鉴权

- 公开：`register`、`login`（网关免登录白名单）。
- `logout`、`profile`、`batch/generate`：必须带网关签名的身份头（`httpx.RequireGatewayIdentity`），handler 不读原始 `X-User-*` 头，直连伪造无效。
- `batch/generate` 额外要求 `admin` 角色（来自 `usr.users.role`，令牌里的 roles：管理员 `["admin","user"]`，普通用户 `["user"]`）。
- `/internal/v1/**`：`X-Internal-Token`。
- **网关必须把 `Authorization` 头原样转发给 user 服务**：网关签名的身份头里没有 `jti`，登出要靠它重新验签得到 `jti`，并核对令牌 `sub` 与网关确认的用户一致。

信封与状态码见设计 §4；失败示例 `{"code":20004,"message":"用户名已存在","data":null,"requestId":"..."}`。

## 端点

### POST /api/v1/user/register

请求（JSON，口令不接受查询串，URL 里出现 `password` 参数直接 400）：

```json
{"username": "alice", "password": "secret1", "phone": "13800000000"}
```

规则（移植自 Java，另补上限）：用户名 1..50 个字符且不含空白/控制字符；密码至少 6 个字符、至多 72 字节（bcrypt 上限）；手机号 `^1[3-9][0-9]{9}$`。角色永远是 `user`，请求体里多余的 `role`/`status`/`id` 被忽略。

成功 200（`id` 为字符串，不含任何口令字段）：

```json
{"code":200,"message":"ok","data":{"id":"7391...","username":"alice","phone":"13800000000","role":"user","status":1,"createdAt":"2026-11-06T08:00:00Z","updatedAt":"2026-11-06T08:00:00Z"}}
```

失败：校验 400 / `10004`；用户名已存在 409 / `20004`（由数据库唯一约束保证，并发同名恰有一个成功）。

### POST /api/v1/user/login

```json
{"username": "demo", "password": "demo1234"}
```

成功 200，响应带 `Cache-Control: no-store`：

```json
{"code":200,"message":"ok","data":{"token":"eyJ...","userId":"10000","username":"demo","roles":["user"],"expiresAt":"2026-11-07T08:00:00Z"}}
```

- 未知用户与口令错误返回完全相同的 401 / `20005` / `用户名或密码错误`，并且两条路径都恰好做一次 bcrypt 比较（`authx.VerifyOrDummy`），无法靠内容或耗时枚举用户名。
- 账号被停用（`status=0`）且口令正确 → 403 / `20006`（口令错误时不暴露停用状态）。
- 成功后登记会话 `ck:sess:{jti}`（TTL = 令牌寿命）；会话登记失败 → 503，不会发出一个网关不认的令牌。
- `last_active_at` 每用户每小时至多刷新一次（数据库条件更新，多实例并发也不突破）；刷新失败不影响登录。
- 不再支持表单提交、GET、查询串凭证；没有 mock-token 接口。

### POST /api/v1/user/logout

头：`Authorization: Bearer <token>`（经网关到达）。只吊销**这个令牌**的会话，同一用户的其他登录不受影响，重新登录也不会复活已登出的令牌；幂等。成功 200 `data:null`。令牌缺失/无效/与网关身份不一致 → 401 / `20002`。

### GET /api/v1/user/profile

返回网关确认的当前用户（字段同 register，登录过则含 `lastActiveAt`），查询串里的 `userId` 等参数一律忽略。用户不存在 404 / `20003`。

### POST /api/v1/user/batch/generate（admin）

压测造用户：创建 `{prefix}{start}` … `{prefix}{start+count-1}`，参数只认 JSON 请求体：

```json
{"prefix": "load", "start": 0, "count": 10000, "password": "password123"}
```

`prefix` 为 1..32 位字母/数字/下划线/连字符；`count` 1..100000；`start` 0..1000000000；`password` 可省略（默认 `password123`，规则同注册）。整批共用一个 bcrypt 哈希（逐用户 bcrypt 造 1 万人要数分钟），按 2000 行一块用 `INSERT ... SELECT unnest(...) ON CONFLICT (username) DO NOTHING` 插入，所以幂等、可重叠重跑、绝不覆盖已有用户（本机 1 万用户约 0.1 秒）。

```json
{"code":200,"message":"ok","data":{"requested":10000,"created":10000,"existing":0}}
```

非 admin 403 / `20006`，无身份 401，参数越界 400。

### GET /internal/v1/users/:id

内部端点（`X-Internal-Token`），返回 `internalapi.UserInfo`：

```json
{"code":200,"message":"ok","data":{"id":"10000","username":"demo","role":"user","status":1}}
```

不存在 404 / `20003`，`id` 非正整数 400。

## 测试

```bash
export CK_TEST_PG_DSN='postgres://postgres@127.0.0.1:54329/postgres?sslmode=disable'   # 真实 PG 的测试需要
go test -count=1 ./internal/user/...
```

- `service`：内存假仓储/假会话，覆盖校验表、重名、登录等价性（`VerifyOrDummy` 调用次数）、停用账号、会话登记失败、登出只吊销当前 jti、`last_active_at` 节流、批量造用户边界/分块/幂等。
- `handler`：`httptest` + `testkit` 网关签名，覆盖 401/403、信封形状、查询串凭证、伪造身份头、全部响应体无口令哈希泄露。
- `repo`：真实 PostgreSQL（`dbxtest` 一次性库），覆盖唯一约束并发（20 协程同名恰一个成功）、角色 CHECK、条件更新并发、种子幂等、1 万用户批量。
- 根目录 `e2e_test.go`：真实 handler+service+repo+miniredis，注册→登录→经网关签名取 profile→登出→会话失效。
