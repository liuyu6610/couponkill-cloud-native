# internal/platform —— 共享基础设施 API 速查

> 本文是 platform-core 的**对外接口草图**，所有服务（gateway/user/coupon/order/connector）只依赖这里列出的入口。
> 包内细节（实现、测试辅助）不在此列。dbx/dbxtest 由 platform-data 负责，不在本文范围。
> 模块路径前缀：`github.com/liuyu6610/couponkill-cloud-native/internal/platform/`。

依赖方向（无环）：`apperr` ← `config` / `logx` / `idgen` / `limiter` ← `authx` ← `redisx` / `kafkax` ← `httpx` ← `app`；`testkit` 依赖 authx/kafkax，**不**依赖 httpx/app/redisx，因此各包的外部测试包（`xxx_test`）都可以安全 import testkit。

## config —— 环境变量加载与校验

```go
type Config struct {
    config.Common                        // CK_ENV / CK_HTTP_PORT / CK_LOG_LEVEL / CK_WORKER_ID（每个服务都内嵌）
    config.Postgres                      // CK_PG_DSN          按需内嵌
    config.Redis                         // CK_REDIS_ADDR / CK_REDIS_PASSWORD / CK_REDIS_DB
    config.Kafka                         // CK_KAFKA_BROKERS
    config.JWT                           // CK_JWT_SECRET / ISSUER / AUDIENCE / TTL
    config.Internal                      // CK_INTERNAL_TOKEN
    config.GatewaySign                   // CK_GATEWAY_SIGN_KEY
    config.Peers                         // CK_USER_URL / COUPON_URL / ORDER_URL / CONNECTOR_URL
    SeckillCooldown time.Duration `env:"CK_ORDER_SECKILL_COOLDOWN" default:"0s"`   // 服务私有开关
}
var cfg Config
err := config.Load(&cfg, config.WithDefaults(map[string]string{"CK_HTTP_PORT": "8082"})) // 一次报告全部错误
```

- tag：`env:"CK_X"`、`default:"..."`、`required:"true"`、`oneof:"a,b"`、`min:"N"`/`max:"N"`（字符串/切片=长度，数字=值，Duration=时长）。支持 string/bool/int*/uint*/float*/`time.Duration`/`[]string`（逗号分隔）/`config.Secret`。
- `config.Secret` 的 `String()/GoString()/LogValue()/MarshalJSON()` 一律输出 `[REDACTED]`；取值用 `s.Reveal()`。**所有密钥、口令、含口令的 DSN 都应使用该类型。**
- `CK_ENV=prod`：`Secret` 字段只要带了 `default`（开发默认值）或 `required:"true"`，就必须显式设置、不得等于默认值、且满足 `min`。dev 模式用了默认值则 `config.Warnings(&cfg)` 返回告警，由 main 打日志。
- 其他：`config.Report(logger, &cfg)`（main 在日志初始化后调用：输出脱敏配置快照 + 对仍用开发默认密钥的字段逐条 Warn）、`config.Summary(&cfg) map[string]string`、`cfg.IsProd()`、测试用 `config.WithMap(m)` 注入环境。
- 错误类型 `*config.ValidationError{Problems []string}`。

## logx —— slog 初始化与 ctx 日志

```go
logger := logx.Setup(logx.Options{Service: "order", Level: cfg.LogLevel}) // JSON handler，同时 slog.SetDefault
ctx = logx.With(ctx, "coupon_id", id)                                      // 给 ctx 里的 logger 追加字段
logx.From(ctx).Info("deducted", "request_id", rid)                          // 没有就回落到 slog.Default()；自带 request_id
```
`logx.WithRequestID(ctx, id)` / `logx.RequestID(ctx)` / `logx.NewRequestID()` / `logx.NewContext(ctx, l)`。键名含 password/secret/token/authorization/sign 的属性自动打码。

## httpx —— gin 引擎、信封、中间件、HTTP 客户端

```go
eng := httpx.NewEngine(httpx.Options{Service: "order", Logger: logger})  // recover→request-id→access log→metrics；已挂 /healthz /readyz /metrics
eng.Health.Add("redis", redisx.PingFn(rdb))                                // 就绪检查（带超时）；eng.Registry 是该引擎的 prometheus 注册表
signer, err := authx.NewGatewaySigner(cfg.GatewaySignKey.Reveal())          // 启动时构造一次（密钥 <32 字节在此失败）
v1 := eng.Group("/api/v1/order", httpx.RequireGatewayIdentity(signer))
```
- 引擎：`*httpx.Engine`（内嵌 `*gin.Engine`，字段 `Health *Health`、`Registry *prometheus.Registry`），直接作为 `http.Handler`。未匹配路由 404 / 405 也返回信封。`Options`：`Service`、`Logger`、`MaxBodyBytes`（默认 1MiB，超限 400「请求体过大」）、`TrustedProxies`（默认**不信任**任何代理，`X-Forwarded-For` 被忽略，防止伪造 IP 绕过限流；网关放在 Ingress 后面时填入其 CIDR）。`httpx.RequestID(c)` 取当前请求 id。
- 信封：`httpx.OK(c, data)` 200；`httpx.Accepted(c, data)` 202；`httpx.Page(c, items, total, page, size)`（`items` 为 nil 时输出 `[]`）；`httpx.Fail(c, err)`：`apperr.Resolve` → 对应 HTTP 状态 + `{code,message,data:null,requestId}`，原因只写日志，响应体不泄露，并 `c.Abort()`。
- 绑定：`req, ok := httpx.BindJSON[CreateReq](c)` / `BindQuery[T]` / `BindURI[T]`；失败已写好 400 信封（message 含字段名与规则，如 `请求参数错误: couponId 为必填项; size 不能大于 100`，最多列 5 项），调用方 `if !ok { return }`。校验用 `binding:"required,min=1"`，字段名取 json/form/uri tag。
- 中间件：`RequireInternalToken(token)`（`X-Internal-Token` 常量时间比较，失败 401）、`RequireGatewayIdentity(signer *authx.GatewaySigner)`（验 `X-Gateway-Sig` 与 ±60s，同时把身份放进 `authx.IdentityFrom(c.Request.Context())` 并让后续日志带 `user_id`，通过后 `httpx.IdentityFrom(c)` 取 `authx.Identity`，失败 401）、`RequireRole("admin")`（403）、`RateLimit(limiter, keyFn)`（超限 429 + `Retry-After`）。
- 就绪：`/readyz` 并发执行全部检查，任一失败 503；`eng.Health.SetShuttingDown()` 使其立即变为 503。
- 出站客户端：
```go
cl := httpx.NewClient(httpx.ClientOptions{BaseURL: cfg.CouponURL, InternalToken: cfg.InternalToken.Reveal(), Timeout: 2 * time.Second})
err := cl.Post(ctx, "/internal/v1/stock/deduct", req, &resp, httpx.Idempotent())  // 幂等调用才允许重试
```
  自动传播 `X-Request-Id`；非 2xx 或信封 code≠200 → `*apperr.Error`（码/状态取自对方，`errors.Is(err, apperr.OutOfStock)` 可用）；网络错误/超时/熔断 → `apperr.SystemBusy`（503，带 cause）。GET/HEAD/PUT/DELETE 默认重试（仅网络错误与 502/503/504，指数退避，次数有上限）；POST 仅在 `httpx.Idempotent()` 时重试；`httpx.Retries(n)`、`httpx.CallTimeout(d)`、`httpx.Header(k,v)`、`httpx.AsIdentity(signer, id)`（附网关签名头）可选。`ClientOptions.Breaker`（可选 `*limiter.Breaker`）：打开时不发请求直接 `SystemBusy`；只有网络错误/超时/5xx 计入失败，业务拒绝（4xx）与调用方自己取消不计。调用方 ctx 取消/超时不重试。

## authx —— JWT、口令、网关身份

```go
tk, err := authx.NewTokens(authx.TokenConfig{Secret: cfg.JWTSecret.Reveal(), Issuer: cfg.JWTIssuer, Audience: cfg.JWTAudience, TTL: cfg.JWTTTL})
tok, err := tk.Issue(authx.Identity{UserID: 42, Roles: []string{"user"}})   // tok.Raw / tok.JTI / tok.ExpiresAt
sess, err := tk.Verify(raw)                                                   // sess.Identity / sess.JTI / sess.ExpiresAt；错误为 apperr.AuthFail(过期)/TokenInvalid
```
- HS256 固定、必须 exp、30s 宽限、校验 iss/aud、`jti` 自动生成；`sub`=用户 ID 字符串；密钥 <32 字节拒绝构造。
- `Identity{UserID int64; Roles []string}`：`IsAdmin()`、`HasRole(r)`；`authx.WithIdentity(ctx, id)` / `authx.IdentityFrom(ctx)`。
- 口令：`authx.HashPassword(plain) (string, error)`（bcrypt cost 10）、`authx.VerifyPassword(hash, plain) bool`、`authx.VerifyOrDummy(hash, plain) bool`（`hash==""` 时做等价工作量的哑比较并返回 false，用于未知用户）。
- 会话吊销接口（由 Redis 实现，见 `redisx.NewSessionStore`；测试用 `authx.NewMemoryRevoker()`）：
```go
type Revoker interface {
    Open(ctx, jti string, ttl time.Duration) error   // 登录时写入 ck:sess:{jti}
    Revoke(ctx, jti string) error                    // 登出：只删这一个 jti
    Active(ctx, jti string) (bool, error)            // 网关校验：键存在才有效
}
```
- 网关身份签名（design §5）：
```go
gs, err := authx.NewGatewaySigner(signKey)         // 密钥 <32 字节拒绝
gs.Apply(req.Header, id)                           // 网关转发前：先 Strip，再写 X-User-Id / X-User-Roles / X-Gateway-Ts / X-Gateway-Sig
id, err := gs.Verify(r.Header)                     // 下游：验签 + |now-ts|≤60s，常量时间比较
authx.StripIdentityHeaders(h)                      // 删除 X-User-Id/X-User-Roles/X-Authenticated/X-Gateway-*
sig := authx.SignGateway(key, userID, roles, ts)   // 底层：HMAC-SHA256(key, "{userId}|{roles}|{ts}")，十六进制
```

## redisx —— go-redis、Lua、分布式锁

```go
rdb := redisx.New(redisx.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword.Reveal(), DB: cfg.RedisDB}) // 或 redisx.FromConfig(cfg.Redis)
scripts := redisx.NewScripts(rdb); enter := scripts.Add("enter", enterLua) // 传入 go:embed 的源码
n, err := enter.Run(ctx, []string{stockKey, markerKey}, rid).Int64()        // EVALSHA，NOSCRIPT 时透明重载；scripts.Preload(ctx) 启动预热
```
- 锁：`lk := redisx.NewLocker(rdb)`；`l, err := lk.Acquire(ctx, "seckill-sweeper", 30*time.Second)`（已被持有 → `redisx.ErrNotAcquired`；键 `ck:lock:{name}`）；`l.Renew(ctx, ttl)`、`l.Release(ctx)`（令牌校验，已丢失 → `redisx.ErrLockLost`）；`lk.Run(ctx, name, ttl, func(ctx) error)`：持锁执行并自动续租，续租失败/锁丢失时取消传给 fn 的 ctx；未抢到返回 `ErrNotAcquired`（定时任务据此跳过）。
- 小工具：`redisx.PingFn(rdb)`（给 `Health.Add`）、`redisx.IsNil(err)`、`redisx.GetJSON[T]` / `redisx.SetJSON`、`redisx.NewSessionStore(rdb)`（实现 `authx.Revoker`，键 `ck:sess:{jti}`）。

## kafkax —— franz-go 生产者与消费组

```go
p, err := kafkax.NewProducer(kafkax.ProducerConfig{Brokers: cfg.KafkaBrokers})   // 幂等 + acks=all；p.Close() error；p.Ping(ctx) 可挂 /readyz
err = p.Publish(ctx, "seckill_order_create", key, payload)                       // 同步等 ack；ctx 超时即返回错误；kafkax.WithHeader(k, v)
err = kafkax.Consume(ctx, kafkax.ConsumerConfig{Brokers: b, Group: "seckill-order-create-group", Topics: []string{"seckill_order_create"}, DLT: p}, handler)
```
- `kafkax.Producer` 是接口（`Publish`、`Close() error`），服务内依赖它，测试注入 `FakeBroker.Producer()`。**`Publish` 因 ctx 超时/取消而失败时，记录可能已送达**（结果未知）：调用方必须配幂等键（如 requestId）让消费端去重，不能假定“失败=没发出去”。`ProducerConfig.AutoCreateTopics` 仅供开发环境。
- `Handler func(ctx, kafkax.Message) error`：成功才提交位点；返回错误则指数退避重试 `MaxRetries` 次（默认 5，`BackoffBase` 100ms、`BackoffMax` 5s）；`kafkax.Permanent(err)` 跳过重试直接死信；耗尽后发布到 `<topic>_dlt`（保留 key/value/headers，并追加 `x-original-topic/partition/offset`、`x-error`、`x-attempts`、`x-consumer-group`、`x-failed-at`），死信发布成功才提交；死信发布失败 → `Consume` 返回错误且**不提交**。handler panic 视为错误。`ctx` 取消 → 优雅停止并返回 nil（未处理完的消息不提交，重启后重投）。
- 并发：`ConsumerConfig.Concurrency`（默认 1）= 同时处理的分区数，分区内严格顺序；`PollMax`（默认 100）= 每批最大记录数；位点按“每批每分区最后一条成功”提交，提交失败重试 3 次后 `Consume` 返回错误。`kafkax.BackoffDelay(attempt, base, max)` 是退避公式（指数、封顶）。
- 语义是**至少一次**：崩溃、再平衡、重放都会重复投递，handler 必须幂等。
- 测试（`testkit`）：
```go
b := testkit.NewFakeBroker()
cfg := b.ConsumerConfig("grp", "topic")      // 已带 Source、DLT、1ms 退避
b.CrashBeforeCommit("grp", 1)                // 下 1 个消费者实例在首次提交时"进程崩溃"：Consume 返回 testkit.ErrInjectedCrash，位点不动；再调一次 b.ConsumerConfig 即“重启”并重投
b.FailPublishes(2)                           // 接下来 2 次 Publish 返回 testkit.ErrInjectedPublish
b.Messages("topic") / b.Committed("grp", "topic", 0) / b.ResetGroup("grp")(重放) / b.PublishCalls()
b.WaitForMessages(t, "topic", n, timeout) / b.WaitForCommit(t, "grp", "topic", 0, n, timeout)   // 等待辅助，超时即 t.Fatal
testkit.NewFakeBroker(testkit.WithPartitions(4))   // 多分区：按 key 哈希分区，分区内偏移连续
```

## idgen —— snowflake

```go
g, err := idgen.New(idgen.ResolveWorkerID(cfg.WorkerID))   // WorkerID<0 → 主机名哈希；范围 0..1023
id := g.Next()                                              // int64，同一实例严格递增、并发唯一、时钟回拨不倒退不重复
s := g.NextString()                                         // 十进制字符串
var id idgen.ID                                             // JSON 输出字符串；输入接受字符串或数字（D8：ID 一律字符串）
```

## limiter —— 令牌桶与熔断器

```go
l := limiter.NewBuckets(limiter.BucketConfig{Rate: 50, Burst: 100})   // 每秒补充速率/突发；按 key 独立；空闲 key 自动清理（无后台 goroutine）
ok := l.Allow(ip); d := l.Take(ip) // d.Allowed / d.RetryAfter
br := limiter.NewBreaker(limiter.BreakerConfig{FailureThreshold: 5, OpenTimeout: 30 * time.Second})
err := br.Execute(func() error { return call() })                      // 打开时返回 limiter.ErrOpen；半开放行探测
permit, err := br.Allow(); permit.Done(success)                        // 手动版
```
时钟可注入：`BucketConfig.Now` / `BreakerConfig.Now`（测试用 `testkit.NewClock`）。`BreakerConfig`：`FailureThreshold`(5)、`OpenTimeout`(30s)、`HalfOpenMax`(1)、`SuccessThreshold`(1)、`IsFailure`（默认忽略 `context.Canceled`）、`OnStateChange`（日志/指标）。`BucketConfig.IdleTTL`（默认 10 分钟，且不早于补满时间）。

## app —— 生命周期

```go
err := app.Run(ctx, app.Options{
    Name: "order", Addr: ":8082", Handler: eng, Health: eng.Health,
    Workers: []app.Worker{{Name: "sweeper", Run: sweeper.Run}, {Name: "consumer", Run: consume}}, // func(ctx) error，必须在 ctx 取消时返回
    OnClose: []func(context.Context) error{func(context.Context) error { return p.Close() }},    // 后进先出
})
```
SIGINT/SIGTERM 或 ctx 取消触发优雅关闭，顺序固定：就绪置为 not-ready →（可选 `DrainDelay`）→ 停止接收 HTTP 并等待在途请求 → 取消并等待 workers → 逆序执行 OnClose。worker 在关闭前返回非 nil 错误会触发关闭并作为 `Run` 的返回值；worker panic 视为错误。所有 goroutine 随 ctx 退出，超过 `ShutdownTimeout`（默认 25s）强制返回。`Options.Listener` 可注入监听器（测试用 `:0`）。监听失败/Handler 缺失时 worker 不会启动，但已注册的 OnClose 仍会执行。在途请求的 ctx 与 worker 的 ctx 都**不会**被信号立刻取消，只在各自的关闭阶段取消。其余可调项：`DrainDelay`、`ReadHeaderTimeout/ReadTimeout/WriteTimeout/IdleTimeout`（默认 5s/15s/30s/90s）、`Signals`、`Logger`。

## testkit —— 其他包的测试辅助

```go
rdb, mr := testkit.Redis(t)                                       // miniredis（含 Lua）+ go-redis 客户端，t.Cleanup 自动关闭
res := testkit.DoJSON(t, handler, "POST", "/api/v1/x", body, testkit.User(42, "admin"))   // body: nil / string·[]byte(原样发送，可造非法 JSON) / 其他按 JSON 编码
res.Status / res.Code / res.Message / res.RequestID / res.Header / res.Body; res.Decode(&out) // 解析信封的 data；res.DataIsNull()
// 其他请求选项：testkit.Internal() 附内部令牌、testkit.Header(k, v)、testkit.RemoteAddr("1.2.3.4:5")
signer := testkit.Signer(t)                                        // 用 GatewayKey 构造的 *authx.GatewaySigner，传给 httpx.RequireGatewayIdentity
req = testkit.AsUser(req, 42, "user")  // 直接给 *http.Request 附合法网关签名头（密钥 testkit.GatewayKey）
clk := testkit.NewClock(time.Now()); clk.Advance(time.Minute); clk.Now
b := testkit.NewFakeBroker()                                     // 见 kafkax 一节
```
固定的测试密钥常量：`testkit.GatewayKey`、`testkit.InternalToken`、`testkit.JWTSecret`（均 ≥32 字节，仅限测试）；`testkit.WithInternalToken` 给请求附 `X-Internal-Token`。
