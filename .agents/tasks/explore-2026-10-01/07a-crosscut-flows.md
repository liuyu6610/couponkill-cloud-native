# 07a crosscut-flows：跨模块端到端链路 / 文档对照 / 鉴权链

> 分片：crosscut-flows（只读调查）。路径均相对仓库根。
> 方式：只读静态阅读。未运行服务。基线：`main` @ `3969213`。登录与用户库细节在 01，库存与订单状态机在 02，前端契约在 04，Helm/Istio 在 05。这里只串路径。
> 标注：【已核实】= 读代码或配置；【推断】= 未运行。

## 0. TL;DR

- 这块是什么：从浏览器到网关、再到券 / 订单 / 用户 / connector 的端到端路径，外加文档声明和鉴权链。单服务内部的 SQL 与缓存不在这里展开。
- 整体健康度：本地「登录 → 网关 JWT → 订单双前缀」这条和已提交前端对得上。秒杀被设计成先受理、后落单。受理成功不等于订单仍可取消，库存键也不是 DB 的镜像。身份在网关白名单、直连和 Istio 入口上都可以不靠登录用户本人。
- 最重要的 5 个发现：
  1. 【高·鉴权】`POST /api/v1/auth/token/mock` 在白名单里，按请求体签发任意 `roles`。白名单请求不清除客户端自带的 `X-User-Id`。Istio 入口不经过网关（05 R16）。下游 `UserContextFilter` 把非空 `X-User-Id` 当成已登录。证据在第 3 节，机制细节不重复 01。
  2. 【高·正确性】秒杀成功的订单会被 order-service 自己消费的结果消息写成 `status=2`（已使用）。取消只接受 `status=1`。用户看到 QUEUED 之后再取消，窗口很短。见 02 第 6 节第 2 行，路径在第 2b 节。
  3. 【高·一致性】常驻领取的扣库存 SQL 没有分片键，演示券 1002 一次会动 16 个分片行。秒杀 Redis 预扣在限领失败、已领重入、发送超时、预热覆盖时和 DB 分叉。见 02 第 6 节第 1、3 行。
  4. 【高·正确性】`status=0` 的券仍能走进秒杀和常驻下单。上下架会删掉 `coupon:stock`，下一次读详情按 DB 剩余把键建回来。见 02 第 6 节第 4 行。
  5. 【高·可用性】分片配置推送会关闭 Spring 仍持有的数据源（coupon / order / user 同一写法，01 与 02 第 5 点）。Helm 注入的 Nacos 地址和命名空间与应用 yml 不一致时，这条链在集群里到不了第 2 节的任何一步（05 R13、R10）。

## 1. 文档声称 vs 代码现实

下表只收跨模块声明。部署制品对不上文档的部分以 05 为准，不把 R01–R51 再抄一遍。

| 声明 | 来源 | 状态 | 证据 |
|---|---|---|---|
| 网关统一入口 + JWT 认证鉴权 | README.md:106-109, 219-224 | 部分落地 | 网关侧 `JwtAuthGlobalFilter` 存在（couponkill-gateway/.../security/JwtAuthGlobalFilter.java:29-205）；但白名单内的 `/api/v1/auth/token/mock` 可匿名签发任意 userId/roles 的 JWT（couponkill-user-service/.../controller/AuthController.java:36-62，JwtAuthGlobalFilter.java:35），Istio 入口也绕开网关（见 05-deploy-cicd.md 第 3.4 节） |
| 通过 Nacos 实现网关路由动态更新 | README.md:222 | 已落地（配置层） | gateway application.yml:9-12 以 `refreshEnabled=true` 导入 `gateway-routes.yaml`；路由定义 nacos/DEFAULT_GROUP/gateway-routes.yaml:8-45。Helm 链路发不出这份 dataId（05 R10） |
| Java 打满后自动把请求切到 Go | README.md:229 | 未落地 | `ServiceGoConfig.shouldRouteToGo()` 要求 `go.enabled` 与 `fallback-to-go` 同时为 true，默认都是 false（ServiceGoConfig.java:55-59；nacos/.../couponkill-order-service-dev.yaml:81-85）。README 同一页下一句又写「Go 旁路默认关闭」（:230），两句互相矛盾，以代码为准 |
| Lua 同时完成扣库存和创建订单 | README.md:195 | 未落地 | `enter_seckill.lua` 只 DECR `coupon:stock` 并写冷却/占位键。订单 insert 在 Kafka 消费之后（02 第 3.1 节） |
| 消费幂等保证至少一次下的业务正确性 | README.md:208 | 部分落地 | `seckill:consume:{requestId}` 能挡住同一条命令的重复消费（OrderServiceImpl.java:948-955）。Redis 回补不守恒、结果消息把成功单写成已使用，见 02 第 6 节 |
| 零停机切换中间件，失败自动回滚 | README.md:252-257 | 未落地 | 分片监听器关闭旧数据源后替换字段，没有排空，也没有失败回滚（02 第 3.5 节，01 的 user-service 相同）。监听的 dataId `middleware-cluster-config.yaml` 在仓库里不存在（07b 第 1.3 节第 3 条） |
| 优惠券发放与使用规则由 coupon-service 控制 | README.md:93 | 部分落地 | `grantCoupons` 直接 `return true`（CouponServiceImpl.java:283-286）。`per_user_limit` 只存储，限领常量在 order-service（02 第 6 节） |
| 生产入口是 Helm，本地入口是 compose + 导入脚本 | docs/DEPLOYMENT-SOURCE-OF-TRUTH.md（05 已引） | 文档与本地链路一致，与 Helm 运行态不一致 | 05 的结论。脚本步骤在 06 第 3.1 节 |

## 2. 端到端链路

### 2a. 登录与身份传递

```
浏览器（04：Web 把口令放在 query）
  -> POST /api/v1/user/login          网关白名单
  -> user-service 校验 BCrypt，签发 JWT（claims：userId、roles）
  -> 之后业务请求 Authorization: Bearer
  -> 网关验签，覆写 X-User-Id / X-User-Roles / X-Authenticated
  -> 订单、券、用户资料用 UserContext 读这个头
```

注册、口令出现在 URL、`/profile` 回传密码哈希、用户名唯一约束按库而不是全局，都在 01 第 3、6 节。这里只补跨服务的几步：

- 订单 Feign 调 user-service 的券计数时走服务名，不经过网关封禁（01 第 4 节末段，07b 端口表）。
- Feign 拦截器只在已有 userId 时转发 `X-User-Id`，不转发 roles（01 引用的 `FeignConfig`）。订单扣库存因此不靠角色头。
- connector 不扫描 common，管理写看的是原始请求头，不是 `UserContext`（02 第 3.4 节）。
- 前端清 localStorage 不等于服务端作废 token。`user:login:{id}` 只写不读（01）。

### 2b. 秒杀抢券 / 下单

```
POST /api/v1/order/seckill?couponId     或兼容前缀 /order/seckill
  网关 JWT -> order-service
  时间窗（若配置了）-> Lua 预扣 coupon:stock -> Kafka seckill_order_create
  响应 code=0, data.status=QUEUED, data.requestId
  前端轮询 GET /api/v1/order/seckill/result?requestId     （04 与 e7ee0a2）

消费端：DB 分片扣 seckill_remaining_stock -> 订单 status=1
  -> 再发 seckill_order_result SUCCESS
  -> 同一进程的监听器把订单改成 status=2
```

常驻券不走 Lua：`POST /api/v1/order/create?couponId` → Redis 占位 → Feign `deduct` → 订单 status=1。type=2 的券会被这条接口拒绝，错误码 `10012`（02 第 3.2 节）。

Go 服务不在默认链上。只有两个开关同时打开才从 `OrderController` 打到 `http://seckill-go-svc:8083`。该 URL 与 Helm Service 的 80 端口不一致（07b 第 1.3 节第 5 条），默认关闭，所以主路径不受影响。

用户能感知到的断点，对应 TL;DR 的第 2–4 条：

- 轮询到 SUCCESS 时，取消接口已经要求 status=1，多半返回 false。
- 常驻演示券 1002 的一次领取在 DB 上不是减 1。
- 管理端把券停用后，缓存一重建，秒杀 Lua 仍可能 DECR。

库存数字怎么漂，02 第 6 节有四种子路径，此处不重画。

### 2c. 通知 / 比价 / 预约到点触发

```
预约：POST /api/v1/order/reservations（活动未开始）
  -> order-service 每秒扫描 PENDING
  -> 认领后调用与 2b 相同的 enterSeckillAsync
  -> QUEUED 后靠 seckill:req:{requestId} 同步成 SUCCESS 或 FAILED
  -> 成功/失败/过期写 user_notification（表在 order_db_0）
  -> 前端铃铛读 /api/v1/order/notifications/unread-count 与 /mine
```

调度默认开启（`couponkill.reservation.enabled` 缺省 true，02 第 3.3 节）。成功通知和 `markSuccess` 不是一次事务。E2E 脚本把仍为 QUEUED 的预约算 PASS，通知缺失只打 WARN（06 第 3.2、第 6 节第 3 行）。

比价不进入秒杀：

```
管理端（admin JWT 或 X-Admin-Token）写绑定和 coupon_price_map
  -> GET /api/v1/connector/price-compare?couponId    网关白名单，可匿名
  -> connector 读绑定 + 价表，可选 probe 平台
  -> 不修改 coupon:stock
```

库存同步是另一条定时路径：connector 每分钟拿平台库存，Feign 到 coupon 的 `/internal/sync-stock`，只改 Redis（02 第 3.4 节）。它会覆盖 2b 正在预扣的键。令牌在 Helm 上只注入 connector、不注入 coupon（02 第 4 节），集群里这条同步会 403【推断】。

06 的 `local-reservation-firedue-e2e.ps1` 与 `local-price-compare-smoke.ps1` 就是这两条链的本机驱动。它们直连 coupon 改时间窗，是因为网关把管理写和预热拦住了，服务自己没有拦住。

## 3. 鉴权链专项

- 网关过滤器顺序：`InternalApiBlockFilter` order=-200 先执行（InternalApiBlockFilter.java:81-83），`JwtAuthGlobalFilter` order=-100（JwtAuthGlobalFilter.java:202-204）。
- 网关白名单为前缀匹配 `startsWith`（JwtAuthGlobalFilter.java:31-41, 189-191）：`/fallback/`、`/api/v1/user/register`、`/api/v1/user/login`、`/api/v1/auth/token/mock`、`/api/v1/connector/health`、`/api/v1/connector/bindings/by-coupon/`、`/api/v1/connector/price-compare`。
- 白名单路径直接 `chain.filter(exchange)`（JwtAuthGlobalFilter.java:65-69），不清洗客户端自带的 `X-User-Id` / `X-User-Roles` / `X-Authenticated`。
- 非白名单：取 `Authorization: Bearer`，校验 HS256 签名后用 `.header(...)` 覆写 `X-User-Id`、`X-Authenticated`、`X-User-Roles`（JwtAuthGlobalFilter.java:84-119）。
- 下游 `UserContextFilter` 无条件读 `X-User-Id`（或 `X-User-ID`）/`X-User-Roles`，只要 `X-User-Id` 非空就视为已认证（couponkill-common/.../filter/UserContextFilter.java:28-33）。
- 严重：`POST /api/v1/auth/token/mock` 无任何开关/Profile 限制，按请求体里的 `userId`、`roles` 直接签发合法 JWT（AuthController.java:24-62），且在网关白名单中（JwtAuthGlobalFilter.java:35）。任何人经网关即可拿到 `roles=["admin"]` 的 token。

业务侧补三条，避免和 01 的用户接口重复：

- 券的库存扣减、预热、`/internal/sync-stock`、`/admin/grant`、`/compensation` 在 coupon-service 里不识别调用者。同步接口只比较 `X-Internal-Token`（`CouponController.java:236-237`）。网关用路径片段封住 `/api/v1/coupon/` 下这些字样（`InternalApiBlockFilter.java:33-44`）。order-service 的 Feign 直连，不受这段封禁【推断】。
- `/order/admin` 与 `/api/v1/order/admin` 被网关前缀封禁（`InternalApiBlockFilter.java:29-30`）。`OrderController.java:102-109` 本身不看角色。直连订单 Pod 可以列出订单。
- connector 的管理写在服务内再查一次 admin 头或 `X-Admin-Token`（`ConnectorAdminInterceptor.java:52-59`）。比价和按券查绑定被放行，与网关白名单一致。01 已写：网关的管理令牌短路不覆盖客户端带来的 `X-User-Id`。

白名单上的登录、注册、比价因此会把客户端自带的 `X-User-*` 原样转给下游。比价只读，危害有限。登录接口本身不用这个头。危害集中在「白名单路径若以后挂上信任 `UserContext` 的逻辑」以及 mock token 这条已经存在的签发能力。

## 4. 系统级风险

跨模块才成立的风险。单模块表格仍以 01 第 6 节、02 第 6 节、05 第 6 节为准。

| 级别 | 风险 | 路径 | 依据 |
|---|---|---|---|
| 高 | 未登录即可拿到 admin JWT，或绕过网关伪造 `X-User-Id` | 第 3 节；Istio 见 05 R16 | 【已核实】白名单与 mock；Istio 路由是 05 已核实，下游信任头是 01/07a 已核实 |
| 高 | 秒杀「成功」之后订单不可取消，库存键与 DB 分叉，停用券仍可下单 | 第 2b 节 | 02 第 6 节四条，本分片核对了控制器到监听器的调用顺序 |
| 高 | 集群里网关找不到实例、分片配置一推送就拆掉数据源 | 第 2 节之前的配置装载 | 05 R10、R13；02 第 3.5 节 |
| 中 | 预约成功通知可能丢；E2E 仍可能报 PASS | 第 2c 节 | 02 第 6 节通知行；06 第 6 节第 3 行 |
| 中 | connector 定时同步在 Helm 上因令牌不一致失败，或在令牌一致时覆盖秒杀预扣 | 第 2c 节 | 02 第 4 节与 `syncRedisStock` |
| 中 | 结果查询不校验 requestId 归属；订单管理列表只靠网关封禁 | 第 3 节补充 | `OrderController.java:102-109,167-171` |
| 低 | README 仍描述自动切 Go、Lua 创建订单、零停机回滚 | 第 1 节 | 与代码对照 |

06 的结论是本机脚本没有单独的「高」级问题。脚本会把第 3 节的缺口暴露出来（firedue 直连写券），它们不是新的生产入口。

## 5. 未验证 / 不确定项

- 没有起网关。`mutate().header` 是否覆盖同名 `X-User-*`，仍是 01 里的框架语义推断。
- 没有走 Istio。05 R16 的「入口绕过 JWT」是读 VirtualService 和下游过滤器得到的。
- 没有并发跑秒杀。第 2b 节的用户可见后果引用 02 的静态结论，本分片没有新的运行证据。
- Feign 是否永远不经过网关，按 `FeignClient.name` 推断。
- 前端轮询失败、取消按钮的具体文案以 04 为准，这里没有再读组件。
- mock token 没有 profile 开关这一点已在第 3 节读过 `AuthController`，没有在不同 `spring.profiles.active` 下启动核对。

## 6. 候选 OpenSpec capability（业务级）

只写跨服务的现状，不建 `openspec/specs`。服务内部的 capability 名见 01 第 9 节和 02 第 9 节。

- `edge-auth`：网关 JWT、白名单、内部路径封禁、admin 写。现状是 mock 签发、白名单不清洗身份头、Istio 可以不走网关。
- `seckill-accept-to-order`：从 `POST /seckill` 到结果轮询、落单、结果消息改状态。现状是 QUEUED 与「可取消的已创建订单」不是同一状态。
- `reservation-to-notification`：预约调度复用秒杀热路径，再写站内通知。现状是通知及状态回写分离，QUEUED 无超时。
- `price-compare-read`：匿名比价。现状是不改库存；写绑定仍要 admin。

