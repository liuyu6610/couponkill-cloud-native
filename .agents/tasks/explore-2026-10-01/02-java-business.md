# 02 · java-business 分片报告（coupon / order / connector）

> 范围：`couponkill-coupon-service/`、`couponkill-order-service/`、`couponkill-connector-service/`（src/main、src/test、resources、Dockerfile、pom.xml），以及它们直接使用的 `couponkill-common` 类。
> 方式：只读静态阅读 + 只读 git。未运行 Maven / 服务 / 脚本。基线：`main` @ `3969213`（2026-07-18）。
> 标注约定：【已核实】= 读代码/配置直接可见；【推断】= 基于框架语义的静态推理，未运行验证。

## 0. TL;DR

- 这块是什么：秒杀券业务三件套。coupon-service 管券模板 + 32 分片库存 + Redis 库存预热；order-service 承载秒杀热路径（Redis Lua 预扣 -> Kafka `seckill_order_create` -> 消费者扣 DB 分片 + 落单）、常驻券领取、取消、预约帮抢（定时到点代抢）和站内通知；connector-service 对接电商平台（JD 真实签名客户端 + MOCK，TB/PDD 桩），做 SKU 绑定、每分钟把平台库存同步进 Redis、同品比价。
- 整体健康度：秒杀主链路骨架清晰，DB 条件更新能兜住"DB 超卖"；但缓存/库存记账、状态机和分片 SQL 有多处实打实的正确性缺陷，测试几乎不覆盖热路径。
- 最重要的 5 个发现：
  1. 【高·bug】常驻券扣减/回补 SQL 不带分片键，ShardingSphere 全路由，一单会扣掉所有分片各 1 件（演示券 1002 = 16 件）；乐观锁用的是缓存里陈旧的 `version`，此后订单全部被判"已抢完"，直到详情缓存过期（`CouponMapper.xml:142-149`，`CouponServiceImpl.java:291-320`）。
  2. 【高·bug】秒杀落单成功后，order-service 自己消费 `seckill_order_result` 把订单改成 `status=2`（已使用），秒杀单因此几乎无法取消（`SeckillOrderResultListener.java:34`，`OrderServiceImpl.java:430`）。
  3. 【高·一致性】Redis 预扣库存记账不守恒：限领失败等后置异常会双倍回补（每次净 +1，用户可反复触发）；已领用户重进时 Lua 扣的 1 件不归还；Kafka 发送超时就补偿但消息仍可能送达；回源/预热/TTL 过期用 DB 值覆盖 Redis。DB 不会超卖，但 Redis 闸门会漂移，表现为"受理后失败"或"假售罄"。
  4. 【高·bug】停用券（`status=0`）挡不住下单和秒杀：三条路径都不校验 status，而且清缓存后 `getCouponById` 回源会立刻重建 `coupon:stock`（`OrderServiceImpl.java:903-924`，`CouponServiceImpl.java:94-116,832-836`）。
  5. 【高·可用性】ShardingSphere 的 Nacos 热更新只换了私有字段、却关闭了旧数据源，而 Spring Bean 仍指向旧对象。任何分片配置变更都会让 DB 全部不可用，与 README 宣称的"零停机切换"相反（`CouponShardingSphereConfig.java:85-93`，`OrderShardingSphereConfig.java:90-98`）。
- 另外值得先看：Helm 下 order/connector 的 Nacos 地址大概率解析成 `localhost:8848`【推断】；connector 的 Secret token 与 coupon 的默认 token 不一致会导致同步 403【推断】；预约成功通知存在竞态丢失；connector 无测试且不在 CI。

## 1. 范围与技术栈

| 项 | 版本 / 事实 | 证据 |
|---|---|---|
| JDK | 25（`release 25`；order 额外 `--enable-preview`） | `pom.xml`（`java.version` 25、compiler `release 25`）；`couponkill-order-service/pom.xml`（compilerArgs `--enable-preview`）；`couponkill-order-service/Dockerfile`（`JAVA_TOOL_OPTIONS ... --enable-preview`） |
| Spring Boot / Cloud / SCA | 4.0.5 / 2025.1.0 / 2025.1.0.0 | `pom.xml` properties `spring.boot.version`、`spring.cloud.version`、`spring.cloud.alibaba.version` |
| ORM / 分片 | MyBatis starter 4.0.1；ShardingSphere-JDBC 5.5.2（coupon/order）；connector 用普通 Hikari 数据源 | `pom.xml`（`mybatis-spring-boot-starter.version`）；coupon/order `pom.xml`（`shardingsphere-jdbc` 5.5.2）；connector `pom.xml`（`spring-boot-starter-jdbc`、`HikariCP`） |
| DB 驱动 | `org.postgresql:postgresql` 42.7.4 | `pom.xml`（`postgresql.version`） |
| MQ | `spring-kafka`（BOM 管理），coupon/order 引入，connector 不引入 | coupon/order `pom.xml` |
| 缓存 | `spring-boot-starter-data-redis`（Lettuce，连接工厂在 common `RedisConfig`）；Caffeine（coupon 显式 3.1.1；order 靠传递依赖） | `couponkill-common/.../config/RedisConfig.java`；coupon `pom.xml` |
| 服务治理 | Nacos discovery/config（`spring.config.import=optional:nacos:`）、OpenFeign + LoadBalancer、Sentinel（coupon/order） | 各服务 `application.yml:6-21`；各 `pom.xml` |
| 其它 | order：`commons-pool2`、`spring-boot-starter-data-jpa`（未使用）、`druid`（未使用）；coupon：`guava 31.1-jre`（覆盖父 POM 33.4.0）、`druid`（未使用）；connector：`spring-boot-jackson2`、springdoc | 对应 `pom.xml` |
| 迁移残留（旧栈） | 三个模块 pom 中没有 MySQL / RocketMQ / RabbitMQ 依赖；代码里只剩注释和兼容字符串（`OrderServiceImpl.java:393-394,1088,1101`） | 已全文检索 |

## 2. 结构地图

```
couponkill-coupon-service/            (Nacos server.port 8080; Dockerfile EXPOSE 8081 <- 漂移)
  controller/CouponController         /api/v1/coupon/** 全部接口（读 + 管理写 + 内部库存接口）
  service/Impl/CouponServiceImpl      分片建券/聚合、缓存、Redis 预热、DB 分片扣减/回补、Connector 同步库存
  mapper/CouponMapper(.xml)           逻辑表 coupon（按 shard_index 分库分表）
  config/CouponShardingSphereConfig   从 Nacos(ns=998) 取 coupon-service-sharding.yaml 建数据源 + 热更新监听
  config/InternalTokenGuard           默认弱 token 在 prod/strict 下拒绝启动
  config/BloomFilterConfig            Guava BloomFilter Bean（无人使用）
couponkill-order-service/             (8082)
  controller/OrderController          /order/** 与 /api/v1/order/** 双前缀：秒杀、结果查询、领取、取消、查询
  controller/ReservationController    /api/v1/order/reservations/** 预约帮抢
  controller/NotificationController   /api/v1/order/notifications/** 站内通知
  service/AsyncSeckillEnterService    热路径：Lua 预扣 + Kafka send.get(500ms) + Redis 补偿 + 结果键
  service/Impl/OrderServiceImpl       普通领取、时间窗校验、Kafka 消费落单、取消、限领、后置异步
  service/ReservationTriggerService   到点认领 -> 调热路径 -> 回写；结果同步；过期；回收卡死 FIRING
  job/ReservationFireJob              @Scheduled 每 1s 扫描 / 每 60s 过期清理
  listener/SeckillOrderCreateListener 消费 seckill_order_create -> fulfillSeckillOrder
  listener/SeckillOrderResultListener 消费 seckill_order_result -> 订单置 2 + 预约回写
  config/KafkaConsumerConfig          两个 listener 容器工厂（虚拟线程执行器）
  config/OrderShardingSphereConfig    order-service-sharding.yaml（order 分片 + SINGLE 预约/通知表）
  feign/*                             CouponServiceFeignClient / UserServiceFeignClient / GoSeckillFeignClient
  support/StructuredFulfillSupport    JDK25 StructuredTaskScope（preview，默认关闭）
  resources/lua/enter_seckill.lua     预扣脚本
couponkill-connector-service/         (8085)
  controller/ConnectorController      /api/v1/connector/**
  service/BindingService              SKU<->券绑定、单条/全量同步、目标库存推导
  service/PriceCompareService         绑定 probe + 手工映射比价
  service/PriceMapService             coupon_price_map CRUD
  job/StockSyncJob                    cron 每分钟 + Redis 锁
  connector/{jd,mock,tb,pdd}          EcommerceConnector SPI 实现（JD 真实，TB/PDD 桩）
  spi/ConnectorRegistry               按 PlatformType 注册
  config/ConnectorAdminInterceptor    管理写接口门禁（X-Admin-Token 或 X-User-Roles 含 admin）
  feign/CouponStockFeignClient        -> coupon /api/v1/coupon/internal/sync-stock
```

直接使用的 common 类：`ApiResponse`/`ErrorCodes`（统一响应与错误码）、`UserContext` + `UserContextFilter`（从 `X-User-Id` 头绑定 ScopedValue）、`FeignConfig`（透传 `X-User-Id`）、`KafkaBaseConfig`（JSON 生产者，acks=all，幂等，delivery 10s）、`RedisConfig`（`RedisTemplate<String,Object>` 带 Jackson 默认类型 + `StringRedisTemplate`）、`TpConfig`（`asyncExecutor` 虚拟线程）、`SnowflakeConfig`、`ServiceGoConfig`、`Backoff`、POJO `Coupon`/`Order`/`OrderMessage`/`SeckillOrderCommand`/`UserCouponCount`、connector SPI 与 DTO（`EcommerceConnector`、`SyncStockRequest/Result`、`SkuBindingCommand`）。

### 2.1 领域模型（按代码现状）

- 券（`Coupon`，表 `coupon_0..15` × `coupon_db_0/1`，主键 `(id, shard_index)`）：模板字段与库存字段放在同一行，没有独立的"批次"概念。`type` 1=常驻、2=秒抢；`status` 0/1；秒杀时间窗 `seckill_start_at/end_at`；`valid_days` 决定订单 `expire_time`；`per_user_limit` 存了但没人用。库存分两套：`total/remaining_stock`（常驻）和 `seckill_total/seckill_remaining_stock`（秒抢）。`createCoupon` 把一张券写成 32 行分片，库存均分、余数给低号分片（`CouponServiceImpl.java:192-280`）；演示种子只写了 16 个分片（`charts/couponkill/scripts/02-seed-demo.sql:36-79`，每片 100，合计 1600）。
- 发放 / 领取：秒抢走 `/order/seckill`；常驻走 `/order/create`；后台批量发放 `grantCoupons` 是桩，直接 `return true`（`CouponServiceImpl.java:283-288`）。
- 订单（`Order`，表 `order_0..15` × `order_db_0/1`，按 `user_id` 分片，`UNIQUE(user_id, coupon_id)`，见 `init-postgres.sql:90-112`）：状态 1=已创建、2=已使用、3=已过期、4=已取消（`couponkill-common/.../pojo/Order.java`）。
- 预约（`seckill_reservation`，SINGLE 表在 order_db_0，活跃态部分唯一索引）与通知（`user_notification`），见 `init-postgres.sql:117-162`。
- Connector：`platform_sku_binding`（`UNIQUE(platform, external_sku_id)`、`UNIQUE(coupon_id)`）和 `coupon_price_map`（`UNIQUE(coupon_id, platform)`），库为 `connector_db`（`charts/couponkill/scripts/03-init-connector.sql:8-41`）。
- `stock_log_0..15` 已建表，但三个服务都不读写。

### 2.2 实际状态机

```
订单 (order.status)
  [1 已创建] --(秒杀: 自身消费 seckill_order_result SUCCESS, SeckillOrderResultListener.java:34)--> [2 已使用]
  [1 已创建] --(本人取消, 仅 status=1, OrderServiceImpl.java:430-439)--> [4 已取消] (+异步回补库存/计数)
  [1 已创建] --(payOrder: 无任何 HTTP 入口, OrderServiceImpl.java:783)--> [2]        (死代码)
  [3 已过期]  代码中从未写入；无超时关单/过期任务（selectExpiredOrders 只在 XML 里，OrderMapper.xml:133）

预约 (seckill_reservation.status)
  PENDING --claimForFire(乐观锁)--> FIRING --enter QUEUED--> QUEUED --req=SUCCESS / Kafka 结果--> SUCCESS
     |  ^                            |   \--COOLING_DOWN/NOT_PREHEATED 且 retry<3--> PENDING
     |  |                            |   \--其它错误 / 重试耗尽--> FAILED
     |  +--reclaimStuckFiring(>2min)-+
     +--本人取消--> CANCELLED    +--活动已结束--> EXPIRED
  QUEUED --req=FAIL--> FAILED ；QUEUED 没有超时回收
```

## 3. 核心流程

### 3.1 秒杀热路径（Redis 预扣 → Kafka → DB 落单）

```
POST /order/seckill 或 /api/v1/order/seckill     身份取 UserContext（X-User-Id）
  -> OrderController：仅当 go.enabled 与 fallback-to-go 同时为 true 才打 Go
     默认两开关都是 false（nacos couponkill-order-service-dev.yaml:81-85，ServiceGoConfig.java:58-59）
  -> enterSeckillAsync：有时间窗才拒绝「未开始 / 已结束」；不看 status
  -> Lua enter_seckill.lua
       已有 seckill:deduct:{user}:{coupon} -> -3（不再 DECR）
       冷却中 -> -2；coupon:stock 不存在 -> -4；库存<=0 -> 0
       否则 DECR coupon:stock，写冷却键和 deduct 键，返回 1
  -> -4 时 Feign 一次 preheat-stock，再跑 Lua
  -> 返回 1：写 seckill:req:{requestId}=PENDING（10 分钟）
             Kafka send seckill_order_create，Future.get(500ms)
             超时或异常：compensateRedis（INCR 库存、删 deduct/冷却、结果改 FAIL）
  -> 客户端拿到 QUEUED + requestId，再 GET /seckill/result

消费 seckill_order_create（组 seckill-order-create-group）
  -> 消费锁 seckill:consume:{requestId} 30 分钟
  -> 已领过：markSuccess(EXISTING)，不回补 Redis
  -> Feign deduct-db-only：按分片乐观锁扣 seckill_remaining_stock，不 DECR Redis
  -> 限领失败等异常：回补该分片 DB，再 compensateRedis
  -> insert 订单 status=1，写 user:received，发 seckill_order_result SUCCESS

消费 seckill_order_result（组 seckill-order-result-group）
  -> SUCCESS：updateOrderStatus(orderId, 2)    // POJO 注释：2=已使用
  -> FAILED：handleSeckillFailure（当前生产代码没有人发 FAILED，见第 6 节）
```

Lua 与补偿的文件：`couponkill-order-service/src/main/resources/lua/enter_seckill.lua:18-50`，`AsyncSeckillEnterService.java:76-183`，`OrderServiceImpl.java:903-1023`，`SeckillOrderCreateListener.java:22-37`，`SeckillOrderResultListener.java:32-36`。

Go 旁路默认不进这条路径。README 里「Java 打满自动切 Go」已经被 `shouldRouteToGo()` 废掉（`ServiceGoConfig.java:55-59`，`OrderController.java:126-128`）。

### 3.2 常驻券领取与取消

```
POST /api/v1/order/create?couponId
  -> Redis Lua 占位 user:received:{user}:{coupon}（TTL 1h，无分片语义）
  -> Feign 读券；type=2 直接拒绝，要求走秒杀接口
  -> 不看 status，不看 per_user_limit
  -> 限领：Redis user:coupon:count，没有则 Feign user-service；总上限 15、秒杀上限 5（硬编码）
  -> Feign POST /api/v1/coupon/deduct/{id}
       getCouponById 的聚合 version
       UPDATE coupon SET remaining_stock = remaining_stock - 1 WHERE id AND version
       （SQL 没有 shard_index）
  -> insert 订单 status=1，virtualId 就是 couponId 字符串
  -> 异步：用户计数 +1、发 order_created

POST /api/v1/order/cancel?orderId
  -> selectById（WHERE 只有订单 id）
  -> 仅 status=1 可取消，改成 4，再按类型回补库存和计数
```

入口：`OrderController.java:51-64`，`OrderServiceImpl.java:192-259,422-439`。常驻扣减：`CouponServiceImpl.java:291-320`，`CouponMapper.xml:142-149`。秒杀券被 `/order/create` 挡住的错误码是 `SECKILL_USE_DEDICATED_API`（`OrderServiceImpl.java:213-214`）。

取消秒杀单时，回补走 `increaseSeckillStockByShardId(virtualId)`。秒杀落单的 `virtualId` 是 `couponId_shardIndex`（`CouponServiceImpl.java:413-417`）。常驻单的 `virtualId` 只是 couponId，取消时按 type=1 走 `increaseStock`，不会误用分片回补。

### 3.3 预约帮抢与站内通知

`CouponkillOrderServiceApplication` 有 `@EnableScheduling`（`:14`）。`ReservationFireJob` 在 `couponkill.reservation.enabled` 缺省为 true 时每 1 秒 `fireDue` + `syncQueuedResults`，每 60 秒过期清理和回收卡死 FIRING（`ReservationFireJob.java:17-38`）。

```
POST /api/v1/order/reservations     活动未开始才能建，状态 PENDING
ReservationFireJob
  -> Redis 锁 reservation:fire:{id} 30s
  -> 已过 seckill_end_at -> EXPIRED + 通知
  -> claimForFire 乐观锁 -> FIRING
  -> enterSeckillAsync（与用户手点同一条热路径）
       QUEUED -> 预约 QUEUED，记下 requestId（这里不发成功通知）
       冷却 / 未预热且 retry<3 -> 回到 PENDING
       其它 -> FAILED + 通知
  -> syncQueuedResults 读 seckill:req:{requestId}
       SUCCESS* -> markSuccess + RESERVATION_SUCCESS 通知
       FAIL*    -> markFailed + RESERVATION_FAILED 通知
QUEUED 没有超时回收。通知写入失败只打日志，不回滚预约状态。
```

状态机草图已在第 2.2 节。通知实现：`NotificationService.java:18-33`。成功通知与 `markSuccess` 的返回值无关（`ReservationTriggerService.java:73-79`）。`seckill:req` 只留 10 分钟（`AsyncSeckillEnterService.java:186`）。

### 3.4 Connector：绑定、每分钟同步、比价

connector 启动类没有扫描 `com.aliyun.seckill.common`（`CouponkillConnectorServiceApplication.java:12`），所以用自己的 `ConnectorExceptionHandler`，不经过 common 的 `UserContextFilter`。管理写由 `ConnectorAdminInterceptor` 看 `X-Admin-Token` 或 `X-User-Roles` 里的 `admin`（`:52-59`）。`/health`、`/bindings/by-coupon/`、`/price-compare` 放行，和网关白名单一致（07a 第 3 节）。

```
POST /api/v1/connector/bindings     只接受 MOCK、JD；同一 couponId 只留一条绑定
StockSyncJob  cron 每分钟，锁 connector:sync:all:lock
  -> 平台库存 -> Feign POST /api/v1/coupon/internal/sync-stock
     头 X-Internal-Token
  -> coupon 侧：force=false 时只允许把 Redis 库存改低或补上缺失键，不抬高
GET /api/v1/connector/price-compare?couponId
  -> 绑定 probe + coupon_price_map 手工价，不写库存
```

TB、PDD 的 `getProduct` / `getStock` 直接抛 `UnsupportedOperationException`，health 恒为 `DISABLED`（`TbStubConnector.java:23-38`，PDD 同结构）。JD 客户端是否真能打通，取决于 `JD_APP_KEY` / `JD_APP_SECRET` / `JD_ACCESS_TOKEN` 是否注入（`application.yml:66-71`），本次没有外呼。

`grantCoupons` 仍是 `return true`（`CouponServiceImpl.java:283-286`）。`stock_log_*` 有 DDL 和 POJO，三个服务的 Java 都不读写。

### 3.5 分片数据源

coupon / order 启动时用独立 Nacos `ConfigService` 拉 `*-service-sharding.yaml`（namespace 来自 yml 的 `shardingsphere.namespace=998`）。内容为空就抛异常，进程起不来（`CouponShardingSphereConfig.java:55-58`）。

监听器收到变更后 `createDataSource`，对旧对象调用 `ShardingSphereDataSource.close()`，再把新实例赋给字段 `dataSource`（`CouponShardingSphereConfig.java:78-93`，`OrderShardingSphereConfig.java:84-98`）。`@Bean` 方法早已把旧引用交给 Spring。推送一次分片 YAML，后续 DB 调用打到已关闭的数据源【推断：关闭语义来自 `close()`，未在运行中触发】。这和 01 报告里 user-service 是同一段写法。

order 还监听 `middleware-cluster-config.yaml`，回调只有日志（`OrderShardingSphereConfig.java:117-128`）。仓库文件名是 `middleware-cluster.yaml`，对不齐的事实见 07b 第 1.3 节第 3 条。README「零停机集群切换」写的是平滑断旧连、失败回滚（`README.md:252-257`）。这段监听器没有排空连接，也没有回滚。

## 4. 对外契约

经网关时：券的 create / seckill-window / status / DELETE 要 admin JWT（`JwtAuthGlobalFilter.java:146-159`）。扣库存、预热、`/internal/sync-stock`、`/admin/grant`、`/compensation` 被 `InternalApiBlockFilter` 整段挡住（`:33-44`，且只对 `/api/v1/coupon/` 做包含匹配）。`/order/admin` 与 `/api/v1/order/admin` 按前缀封禁（`:29-30`）。订单、预约、通知的其它路径要 JWT，不要求 admin。Feign 按服务名直连，不经过这些过滤器【推断：`lb://` 不走网关】。

| 入口 | 方法与参数 | 服务自身鉴权 | 成功体 |
|---|---|---|---|
| `/api/v1/coupon/available`、`/list`、`GET /{id}` | GET | 无 | `ApiResponse<Coupon>` 或列表；详情是 32 分片聚合视图 |
| `/api/v1/coupon/create` | POST，query 拼模板字段 | 无；网关要求 admin | 聚合后的券 |
| `/api/v1/coupon/{id}/seckill-window` | POST，开始/结束时间 | 无；网关要求 admin | 更新后的券 |
| `/api/v1/coupon/{id}/status`、`DELETE /{id}` | status 或删除 | 无；网关要求 admin | 影响行数 |
| `/api/v1/coupon/deduct/{id}`、`/increase/{id}` | POST | 无 | `ApiResponse<Boolean>`；失败也常是 `code=0` 且 data=false |
| `/api/v1/coupon/deduct-db-only/{id}` | POST | 无 | data 为 `couponId_shardIndex` |
| `/api/v1/coupon/preheat-stock/{id}` | POST | 无 | `ApiResponse<Boolean>` |
| `/api/v1/coupon/internal/sync-stock` | POST JSON + `X-Internal-Token` | 令牌常量比较 | `SyncStockResult`；令牌不对是 `code=403` |
| `/order/seckill`、`/api/v1/order/seckill` | POST `couponId` | `UserContext.requireCurrentUserId()` | QUEUED 时 `code=0`；拒绝时 `ApiResponse.fail(10xxx)` |
| `/api/v1/order/seckill/result` | GET `requestId` | 只要已登录，不核对 requestId 属于谁 | data 为 `PENDING` / `SUCCESS:{orderId}` / `FAIL` / `UNKNOWN` |
| `/api/v1/order/create` | POST `couponId` | 当前用户 | `ApiResponse<Order>`，status=1 |
| `/api/v1/order/cancel` | POST `orderId` | 订单 userId 必须等于当前用户 | 非 status=1 时 `code=0` 且 data=false |
| `/api/v1/order/admin` | GET 分页 | 无角色校验 | 订单列表；网关前缀不可达 |
| `/api/v1/order/reservations` | POST JSON `couponId` | 当前用户 | `SeckillReservation` |
| `/api/v1/order/notifications/mine`、`/unread-count` | GET | 当前用户 | 列表或 `{count}` |
| `/api/v1/connector/bindings` | POST JSON | admin 头或管理令牌 | `PlatformSkuBinding` |
| `/api/v1/connector/price-compare` | GET `couponId` | 拦截器放行 | `PriceCompareResult` |
| `/api/v1/connector/sync`、`/sync/{id}` | POST `force` | admin | 批量计数或单条绑定 |

业务错误码真源是 `ErrorCodes`：秒杀/库存 `10001`–`10013`，用户 `20001`–`20005`，系统 `50000` / `50003`（`ErrorCodes.java:11-34`）。`GlobalExceptionHandler` 把 `BusinessException` 收成 `ApiResponse.fail`，不改 HTTP 状态（`:11-15`）。coupon 控制器里不少失败是 `ApiResponse.fail(500, 异常 message)`，数字 `500` 不在 `ErrorCodes` 里（例如 `CouponController.java:187`）。connector 的参数错误是 `code=400`（`ConnectorExceptionHandler.java:15-18`）。网关 401 空 body、403 JSON 的形状见 01 第 4 节，这里不重写。

`GET /seckill/result` 不把 `requestId` 和当前用户绑定（`OrderController.java:167-171`）。知道别人的 requestId 就能读到 `SUCCESS:{orderId}`【已核实：方法体只有登录校验】。

内部同步令牌：coupon 与 connector 的缺省值都是同一段占位串，键名 `connector.internal-token` / `CONNECTOR_INTERNAL_TOKEN`（`CouponController.java:30`，`BindingService.java:37`）。Helm 只给 connector Deployment 注入该环境变量（`charts/couponkill/templates/deploy-connector.yaml:60-64`）。coupon 的 Deployment 环境变量列表里没有这一项（`deploy-coupon.yaml:60-81`）。Secret 若不是缺省占位，同步请求会在 `CouponController.java:236-237` 得到 `code=403`【推断：两边进程拿到的字符串不同；注入面不对称是已核实的】。

订单双前缀是故意的：`@RequestMapping({"/order", "/api/v1/order"})`（`OrderController.java:29`）。网关路由同时挂了 `/order/**`（07b 第 1.1 节）。单测只断言这两条「我的订单」都返回 `code=0`（`OrderControllerPathContractTest.java:58-68`）。

## 5. 依赖关系

```
浏览器 / 前端
  -> couponkill-gateway
       -> lb://couponkill-coupon-service     券读写、管理写
       -> lb://couponkill-order-service      秒杀、领取、预约、通知
       -> lb://couponkill-connector-service  绑定、比价、同步触发
couponkill-order-service
  -> Feign couponkill-coupon-service   读券、扣/回补、预热（CouponServiceFeignClient.java:15-47）
  -> Feign couponkill-user-service     券计数（契约在 01）
  -> Feign url seckill-go-svc:8083     仅沙箱双开
  -> Kafka  seckill_order_create / seckill_order_result / seckill_compensation / order_created
  -> Redis  预扣、结果键、预约锁、领取标记
  -> PG     order_db_0/1 的 order_0..15；预约和通知 SINGLE 在 order_db_0
couponkill-coupon-service
  -> PG     coupon_db_0/1 的 coupon_0..15、stock_log_0..15（日志表无读写）
  -> Redis  详情、库存、分片缓存
  -> Nacos  ns 120 的业务 yaml；ns 998 的分片 yaml
couponkill-connector-service
  -> PG connector_db（03 脚本；Helm db-init 不执行，见 05 R32）
  -> Redis  同步锁
  -> Feign coupon /internal/sync-stock
  -> 可选出站 JD OpenAPI
```

coupon、order 扫描 common，因此带上：带 default typing 的 `RedisTemplate`、`UserContextFilter`、`GlobalExceptionHandler`、Kafka 生产者。库存热路径用的是 `StringRedisTemplate` 和裸 `SET` 字节，避开了那只 ObjectMapper（`AsyncSeckillEnterService.java:34`，`CouponServiceImpl.java:110-114`）。详情缓存 `coupon:detail:{id}` 走 `RedisTemplate`，值带类型信息【已核实】。反序列化风险的机制在 01 第 5 节，本分片只确认券详情用了这只模板。

connector 不扫描 common，Redis 用 Boot 自配置的 `StringRedisTemplate`。它的 pom 没有 Kafka。

coupon 声明了 `@EnableFeignClients`，模块内没有 `@FeignClient`【已核实】。order 的 pom 有 `spring-boot-starter-data-jpa` 和 Druid，源码没有 `JpaRepository` / Druid 数据源用法【已核实：在 order-service 的 src/main 检索类名】。`compensationAmount` 只被注入，没有任何读取（`OrderServiceImpl.java:161-162`）。`BloomFilterConfig` 的 Guava 过滤器没有注入点（`BloomFilterConfig.java:15`）。订单侧自己用 Redis bitmap 做「是否领过」的近似判断（`OrderServiceImpl.java:497-516`），和这个 Bean 不是一回事。

三个模块的 `pom.xml` 都没有 `spring-boot-starter-actuator`。yml 里的 `management.endpoints.web.exposure.include` 因此不会暴露 `/actuator/health`【推断：与 05 R30 同一前提】。探针路径本身见 05。

## 6. 问题与风险

第 0 节的五条在这里按代码落点写清。级别沿用 TL;DR。

| 级别 | 类型 | 结论 | 证据 |
|---|---|---|---|
| 高 | 正确性 | 常驻扣减/回补 SQL 的 WHERE 只有 `id` 和 `version`，没有分片键 `shard_index`。分片算法的列就是 `shard_index`。ShardingSphere 对缺分片键的语句会广播到 `coupon_0..15` × 两个库【推断，框架路由规则】。演示券 1002 在 `coupon_db_0` 写了 shard 0..15 各一行、每行 `remaining_stock=100`（合计 1600）。一次成功的 `deductStock` 会让这 16 行各减 1，也就是 16 件，而不是 1 件。乐观锁用的是聚合对象上的 `version`（取最小 shard 那一行）。扣完后异步「刷新」先走 `getCouponById`，缓存命中就原样写回，version 一直是旧的。下一单 `rows=0`，接口把这当成没库存，直到详情键过期（30–40 分钟）重新加载 | `CouponMapper.xml:142-149`；`coupon-service-sharding.yaml:47-56`；`02-seed-demo.sql:37-45,67-77`；`CouponServiceImpl.java:169,291-315`；`getCouponById` 缓存短路在 `:82-84` |
| 高 | 正确性 | 秒杀落单成功后，order-service 自己消费 `seckill_order_result`，把刚写成 1 的订单改成 2。POJO 把 2 定义成已使用。取消只接受 status=1，所以成功秒杀单在结果消息被消费后取消会得到 false。没有把 3（已过期）写进表的任务 | `SeckillOrderResultListener.java:32-34`；`Order.java:21-23`；`OrderServiceImpl.java:430-431`；`OrderMapper.xml:133` 的过期查询无 Java 调用方（结构地图已记） |
| 高 | 一致性 | Redis `coupon:stock:{id}` 与 DB 不是同一本账。四处已读到的漂移：① 限领等后置失败时，`safeIncreaseSeckillStock` 对库存键 `INCR` 一次，`compensateRedis` 再 `INCR` 一次；Lua 之前只 `DECR` 一次，净效果是 +1，用户可以反复触发。② `hasUserReceivedCoupon` 为真时直接 `markSuccess`，不调用 `compensateRedis`，Lua 已扣的 1 件留在 Redis 外；`seckill:deduct` 300 秒过期后同一用户可以再扣。③ `send().get(500ms)` 超时只说明等待结束，不会取消已经交给生产者的发送【推断：Kafka `Future.get` 超时语义】；消息仍可能被消费并扣 DB，而 Redis 已经加回。④ `getCouponById` 缓存未命中、`preheatCouponStock`、全量预热都用 DB 合计 `SET` 覆盖 Redis，在途预扣被抹掉。DB 条件更新（`seckill_remaining_stock + change >= 0` 且带 shard）仍能挡住 DB 超卖 | `OrderServiceImpl.java:957-960,990-991,1032-1035`；`CouponServiceImpl.java:384-387`；`AsyncSeckillEnterService.java:123-131,171-174`；`enter_seckill.lua:36-39`；`CouponServiceImpl.java:104-114,640-649`；`nacos/.../couponkill-order-service-dev.yaml:70-73` |
| 高 | 正确性 | 停用券挡不住下单和秒杀。`enterSeckillAsync` 只比较时间窗；`createOrder` 只拒绝「找不到」和 type=2。`updateCouponStatus` 会删掉 `coupon:stock`。下一次 `getCouponById` 不看 status，按 DB 剩余库存把键建回来。秒杀 Lua 只认这个键在不在、值是否大于 0 | `OrderServiceImpl.java:903-924,207-215`；`CouponServiceImpl.java:804-835,94-116` |
| 高 | 可用性 | 分片 YAML 一推送就关闭 Spring 仍在使用的数据源。与 README 的零停机切换不是同一套机制。user-service 的副本见 01 | `CouponShardingSphereConfig.java:64,85-93,101`；`OrderShardingSphereConfig.java:90-98`；`README.md:252-257` |
| 高 | 一致性 | Helm 下 order、connector 的 Nacos 地址写法见 05 R13：yml 把 `server-addr` 绑在 `NACOS_SERVER_ADDR`，Chart 注入的是另一组环境变量。本分片只确认消费方的键名 | `couponkill-order-service/.../application.yml:13-20`；`couponkill-connector-service/.../application.yml:12-18`；05 报告 R13 |
| 中 | 安全 | 库存扣减、预热、补偿发券在服务内部不校验调用者。网关用路径片段封住 `/api/v1/coupon/` 下这些字样。能直连 Pod 的调用方可以扣库存。`/order/admin` 同样：网关前缀封禁，控制器不看角色 | `CouponController.java:179-193`；`InternalApiBlockFilter.java:25-44`；`OrderController.java:102-109` |
| 中 | 安全 | 同步令牌缺省是仓库里的固定字符串。Helm 只把 Secret 注入 connector，不注入 coupon，两边会不一致并返回 403 | 第 4 节最后一段的文件 |
| 中 | 正确性 | 预约成功通知在 `markSuccess` 之后调用，不看更新行数，也不和状态更新同一事务。`markSuccess` 没改到行（version 不符）时通知仍会写。进程若在两次调用之间退出，状态已是 SUCCESS，下一轮只扫描 QUEUED，通知不会补发【推断：崩溃窗口】。通知异常被吃掉，注释写明不回滚预约 | `ReservationTriggerService.java:73-79`；`SeckillReservationMapper.xml:87-94`；`NotificationService.java:30-32` |
| 中 | 正确性 | `per_user_limit` 会入库，领取判断用的是常量 15 和 5。`GET /seckill/result` 任意登录用户可按 requestId 读取别人的结果串 | `CouponServiceImpl.java:163`（只拷贝字段）；`OrderServiceImpl.java:182-183,565-571`；`OrderController.java:167-171` |
| 中 | 一致性 | `order_created`、`seckill_compensation` 只有发送、没有 `@KafkaListener`。结果主题的 FAILED 分支没有生产方。`handleSeckillSuccess` 把 SUCCESS 发到 `order_created`，不是结果主题 | `OrderServiceImpl.java:155,731,887`；全仓库 Java 检索 `@KafkaListener` 只有两个类 |
| 中 | 构建 | connector 没有 `src/test`，CI 的 `-pl` 也不包含它（05 R38，本分片在 `.github/workflows/ci.yml` 检索 `connector-service` 为 0） | 目录与 workflow |
| 低 | 死代码 | `grantCoupons` 恒 true；`updateStock(Long,int)` 抛 `UnsupportedOperationException`；`handleExpiredCoupons` 空方法；`compensationAmount`、Guava Bloom、JPA/Druid 依赖无使用点；`payOrder` 无 HTTP 入口 | 对应方法；结构地图第 2.2 节 |
| 低 | 信息泄露 | 未捕获异常的 message 回到客户端。coupon 若干接口把 `e.getMessage()` 放进 `ApiResponse.fail` | `GlobalExceptionHandler.java:19`；`CouponController.java:187` |

Istio 绕过网关后直接打到这些控制器，是 05 R16 和 07a 第 3 节的结论。本分片只确认订单和券控制器信任 `UserContext`，而 `UserContext` 来自请求头。

## 6.5 全模块编译结果

未执行。只读约束禁止跑 Maven。不把各模块 `target/` 里已有的 class 当成这次源码的编译通过证据。

order-service 的 compilerArgs 含 `--enable-preview`，Dockerfile 的 `JAVA_TOOL_OPTIONS` 也带了它（第 1 节）。`StructuredFulfillSupport` 只在 `couponkill.seckill.structured-concurrency=true` 时调用，Nacos 缺省是 false（`couponkill-order-service-dev.yaml:77-78`）。没开这个开关时，热路径不依赖预览 API【已核实：`loadCouponForFulfill` 的分支，`OrderServiceImpl.java:1049-1051`】。

## 7. 最近活跃度

`git log --oneline` 触及三个模块目录的提交共 40 个。分目录：coupon 32、order 34、connector 5（一条提交可以同时改多个目录）。最近一次都在 2026-07-18：

| 提交 | 日期 | 说明 |
|---|---|---|
| `47cd445` | 2026-07-18 | 秒杀时间窗、状态、删除改为按全部分片写 |
| `0e10e22` | 2026-07-18 | 券分片聚合、预约 fireDue、配套 E2E 脚本 |
| `0203a78` | 2026-07-18 | 预约通知；order 目录的最后一次提交 |
| `30deb3b` | 2026-07-18 | connector 多平台价表；connector 目录的最后一次提交 |
| `2dbcdee` | 2026-07-18 | C 端比价 API |
| `e7ee0a2` | 2026-07-18 | 按 requestId 轮询秒杀结果 |
| `9adb042` | 2026-07-18 | 冻结 Go 热路径、统一订单 API |
| `6b3e9b5` | 2026-07-12 | PG/Kafka、异步秒杀、connector 引入 |

再往前是 2025-08 至 09 的压测与初稿（`3649531`、`d60200f`）。2026-07-18 这批提交把热路径从同步扣库存改成了 Lua + Kafka，并补上预约和 connector；第 6 节的分片 SQL、结果状态和 Redis 回补是这套新路径上的行为，不是更早的 MySQL 注释残留。

## 8. 未验证 / 不确定项

- 没有跑 Maven，也没有连 PostgreSQL。广播更新「16 行各减 1」、`update` 返回值是各分片之和，都是按 ShardingSphere 缺分片键时的路由规则推断的。SQL 文本和种子数据是读文件得到的。
- 没有起 Kafka。`Future.get` 超时后消息仍可能送达，依据是客户端 future 超时不等于撤回发送。acks=all 与 500ms 谁先到，取决于 broker，没有压测。
- 没有并发打「限领失败」和「已领用户重入」。两条路径上的 `INCR` / 缺失的 `compensateRedis` 是读代码得到的，净 +1 是这两次 `INCR` 与一次 `DECR` 的算术。
- `ServerHttpRequest` 之外的 Feign 是否绕过网关，按 `FeignClient.name` 走负载均衡推断，没有抓包。
- Helm Secret 的实际值没有读取。令牌不一致的结论只依赖「coupon 模板不注入 `CONNECTOR_INTERNAL_TOKEN`、connector 模板注入」。
- 预约通知丢失需要进程在 `markSuccess` 与 `notifyReservation` 之间退出，没有做故障注入。
- JD OpenAPI 签名客户端没有用真实密钥调用。
- actuator 缺失的运行后果与 05 R30 相同，没有起进程访问 `/actuator/health`。

## 9. 候选 OpenSpec capability（只写现状，不建 spec）

本次只交付探查报告，不在 `openspec/specs` 下落文件。

- `coupon-catalog`：建券、时间窗、上下架、详情聚合。现状是管理写只在网关上要 admin，服务内部不校验；停用只改 `status` 并清缓存，不清掉「还能下单」这条路径。
- `coupon-stock`：常驻 `remaining_stock` 与秒杀 Redis 预扣 + DB 分片。现状是常驻 SQL 缺分片键，Redis 与 DB 会漂移，DB 行级条件更新仍拒绝把单行打成负数。
- `seckill-order`：受理、轮询、落单、取消。现状是受理返回 QUEUED；成功结果把订单打成已使用；取消只接受已创建；`requestId` 不校验归属。
- `seckill-reservation`：预约、到点代抢、站内通知。现状是调度默认开启，成功通知和状态回写分开，QUEUED 无超时。
- `platform-connector`：SKU 绑定、库存同步、比价。现状是 MOCK/JD 可绑定，TB/PDD 为桩；同步只改 Redis；内部令牌在 Helm 上两边注入不对称；无测试、不在 CI。
