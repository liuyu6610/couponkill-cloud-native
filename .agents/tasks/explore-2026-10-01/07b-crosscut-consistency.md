# 07b crosscut-consistency：代码与应用配置层跨模块一致性矩阵

> 分片：crosscut-consistency（只读调查，OpenSpec explore 模式）。路径均相对仓库根目录。
> 口径：「已核实」= 读过对应文件/行；「推断」= 基于框架语义的静态推理，未运行。部署侧（charts / k8s / Jenkins / Dockerfile）结论直接引用 `.agents/tasks/explore-2026-10-01/05-deploy-cicd.md`（下称「05 报告」），不重复推导。敏感值只写 key 名。

## 0. TL;DR

- 这块是什么：把端口、Nacos、Feign（第 1 节）、Kafka、Redis、PostgreSQL、响应体和探针放在一张跨服务对照里。部署侧断裂点引用 05，不重推 Helm。
- 整体健康度：compose + `import-nacos-local.ps1` 之后，Java 的 dataId 和端口能对上第 1.2 节那张图。同一条业务链上，Kafka 有两个主题没有消费者，Redis 库存键被三条路径覆盖，错误体有三套形状，`/actuator/prometheus` 没有依赖。
- 最重要的 5 个发现：
  1. 【高·一致性】第 1.3 节的七条仍然成立：端口多处各写一份、`namespace: couponkill` 与实际的 120/998 不是同一个值、Go 订阅的 dataId 在仓库里不存在、网关路由写了两遍、Go 的 URL 带 8083 而 Service 是 80、connector 的环境变量会被 Nacos 盖掉、Go 本地兜底连 5432。本次重读没有发现和这张表矛盾的端口，所以没有改第 1 节。
  2. 【高·一致性】`seckill_order_result` 的消费者把成功订单写成已使用（02）。`order_created` 和 `seckill_compensation` 只有生产者。Nacos `common.yaml` 把 `enable-auto-commit` 设为 true，消费者代码在该值为 true 时不再设置 `AckMode.RECORD`。
  3. 【高·一致性】`coupon:stock:{id}` 同时被 Lua `DECR`、券详情回源的绝对 `SET`、connector 同步的绝对 `SET` 写入。常驻扣减 SQL 还不带分片键（02）。Redis 闸门和 DB 行不是同一本账。
  4. 【中·契约】成功体是 `ApiResponse` 且 `code=0`、HTTP 仍是 200。网关 401 是空 body，403 是另一段 JSON。coupon 控制器还会返回数字 `500`/`403`，不在 `ErrorCodes` 里。
  5. 【中·可观测性】各服务 yml 写了 `/actuator/health` 的暴露列表，pom 里没有 actuator，也没有 micrometer。05 R30 的抓取路径因此没有进程内端点【推断】。

## 1. 服务标识与端口总表

### 1.1 矩阵

| 行 \ 服务 | gateway | user | coupon | order | connector | seckill-go |
|---|---|---|---|---|---|---|
| `spring.application.name`（src/main/resources/application.yml） | couponkill-gateway（couponkill-gateway/src/main/resources/application.yml:5） | couponkill-user-service（couponkill-user-service/src/main/resources/application.yml:6） | couponkill-coupon-service（couponkill-coupon-service/src/main/resources/application.yml:3） | couponkill-order-service（couponkill-order-service/src/main/resources/application.yml:3） | couponkill-connector-service（couponkill-connector-service/src/main/resources/application.yml:3） | 无注册名。Go 只建 Nacos config client，不做服务注册（couponkill-go-service/cmd/server/main.go:38-70）；`nacos/DEFAULT_GROUP/go-service-dev.yaml:6` 写了 `seckill-go-service`，但 `Config` 结构体没有 spring 段（couponkill-go-service/internal/config/config.go:27-90），不读 |
| application.yml 里的 `server.port` | 未设置 | 未设置 | 未设置 | 未设置 | 未设置 | 默认 8090（config.go:362） |
| 导入的 Nacos dataId（group） | common.yaml、gateway-routes.yaml、couponkill-gateway.yaml（DEFAULT_GROUP）（application.yml:10-12） | common.yaml、couponkill-user-service.yaml（:11-12） | common.yaml、couponkill-coupon-service.yaml（:8-9） | 同左（:8-9） | 同左（:8-9） | go-service-dev.yaml（config.go:187,199,306）；监听 middleware-cluster-config.yaml（:220）、service-collaboration.yaml（:264；main.go:72-75） |
| config / discovery namespace | `${NACOS_NAMESPACE:120}`（:17,:23） | 同（:17,:22） | 同（:14,:20） | 同（:14,:20） | 同（:14,:19） | `NACOS_NAMESPACE_ID` 默认 120（config.go:178）；main.go:40 第二个 client 改用配置里的 `nacos.namespace-id` |
| ShardingSphere namespace | - | 998（application.yml:25） | 998（:23） | 998（:23） | 不用分片，直连 `connector_db`（:22-28） | 不用 Nacos 分片配置，DSN 写在 go-service-dev.yaml:22-34 |
| nacos/ 对应文件与 `server.port` | couponkill-gateway-dev.yaml:2 = 8088 | couponkill-user-service-dev.yaml:2 = 8081 | couponkill-coupon-service-dev.yaml:2 = 8080 | couponkill-order-service-dev.yaml:2 = 8082 | couponkill-connector-service-dev.yaml:2 = 8085 | go-service-dev.yaml:2 = 8083 |
| nacos/ 文件里写的 namespace | `spring.cloud.nacos.namespace: couponkill`（couponkill-gateway-dev.yaml:13） | 同（couponkill-user-service-dev.yaml:10） | 同（couponkill-coupon-service-dev.yaml:19） | 同（couponkill-order-service-dev.yaml:10） | 同（couponkill-connector-service-dev.yaml:10） | 同（go-service-dev.yaml:10）；另有 `nacos.namespace-id: "couponkill"`（:42） |
| 本地导入脚本实际发布 | 原名 + 副本 couponkill-gateway.yaml 到 tenant 120（scripts/import-nacos-local.ps1:64,66） | 副本 couponkill-user-service.yaml（:69） | 副本（:68） | 副本（:67） | 副本（:70） | 仅原名 go-service-dev.yaml（:64 循环）；`Localize` 只改主机名，不改 namespace（:29-37） |
| gateway 路由目标 | - | `lb://couponkill-user-service`，Path `/api/v1/user/**,/api/v1/auth/**`（nacos/DEFAULT_GROUP/gateway-routes.yaml:10-12） | `lb://couponkill-coupon-service`（:19-21） | `lb://couponkill-order-service`，含旧前缀 `/order/**`（:28-31） | `lb://couponkill-connector-service`（:38-40） | 已注释，不经网关（:46-50） |
| 服务间调用（Feign） | - | 被 order 调用：`name=couponkill-user-service`（couponkill-order-service/.../feign/UserServiceFeignClient.java:14） | 被 order、connector 调用：`name=couponkill-coupon-service`（CouponServiceFeignClient.java:17；couponkill-connector-service/.../feign/CouponStockFeignClient.java:14） | - | - | 被 order 调用：`url=${couponkill.seckill.go.url:http://seckill-go-svc:8083}`（GoSeckillFeignClient.java:16-17；couponkill-common/.../config/ServiceGoConfig.java:26） |
| couponkill-go-service/config.yaml（Nacos 失败时的本地兜底） | - | - | - | - | - | port 8083（:2）；PG `host=localhost port=5432`（:15-25）；Redis `localhost:6379`（:28）；Nacos `localhost:8848`、namespace-id `couponkill`（:34-35） |
| docker-compose.migration.yml | 不含应用容器，只有中间件：postgres 5433->5432（:20）、redis 6379（:37）、kafka 9092（:58）、nacos 8848/9848（:98-99） | 同左 | 同左 | 同左 | 同左 | 同左 |
| 部署侧对照（引自 05 报告 4.1） | Dockerfile 8080；values 8080；Helm 用 SERVER_PORT 钉死；Service 80->8080 | Dockerfile 8083；values 8081 / prod 8083；SERVER_PORT 钉死 | Dockerfile 8081；values 8080 / prod 8081；不钉端口；没有 Service | Dockerfile 8082；values 8082；不钉端口；Service 80->8082 | Dockerfile 8085；8085；钉死 | Dockerfile 8090；values 8083；注入 `PORT`，代码不读；Service `seckill-go-svc` 80->8083 |

### 1.2 本地链路（compose + 导入脚本）下各服务的实际端口（推断）

```
browser --> vite proxy / miniprogram (默认 http://localhost:8088 / http://127.0.0.1:8088)
             frontend/couponkill-frontend/vite.config.ts:7, frontend/couponkill-miniprogram/utils/api.js:28
   |
   v
gateway :8088 (nacos couponkill-gateway.yaml) --lb://--> user :8081 | coupon :8080 | order :8082 | connector :8085
                                                            ^
order --Feign url--> seckill-go-svc:8083 (Go 读到 go-service-dev.yaml 时监听 8083，否则 8090)
```

只要 Nacos 在 120 命名空间里有 `couponkill-<svc>.yaml`，端口由 Nacos 决定；取不到就退回 Spring 默认 8080。5 个 application.yml 都没写 `server.port`，所以 Nacos 缺配置时 gateway、user、coupon、order、connector 会全部落到 8080（推断，Spring Boot 默认值语义）。

### 1.3 不一致清单（本节）

1. 同一服务的端口在 4 到 5 处各写一份。user 在 Nacos 里是 8081，Dockerfile 和 values-prod 是 8083；coupon 在 Nacos 里是 8080，Dockerfile 和 values-prod 是 8081；Go 默认值 8090（config.go:362）与 Nacos 和 config.yaml 的 8083 不同。已核实。
2. 所有 `*-dev.yaml` 都写了 `spring.cloud.nacos.namespace: couponkill`，而实际使用的命名空间是 120 和 998，导入脚本也不会改写这个值（import-nacos-local.ps1:29-37）。Java 侧的连接用的是更具体的 `discovery/config.namespace`，所以不受影响；但 `nacos/DEFAULT_GROUP/common.yaml:45` 的 Sentinel 数据源写的是 `namespace: ${spring.cloud.nacos.namespace}`，会解析成 `couponkill`，从而读不到规则（推断）。Go 侧 main.go:40 用配置里的 `namespace-id: couponkill` 去取 `service-collaboration.yaml`，也会落到一个不存在的命名空间（代码路径已核实，运行结果属推断）。
3. Go 订阅的两个 dataId 在仓库里都不存在同名文件。`middleware-cluster-config.yaml`（config.go:220）对应的仓库文件叫 `middleware-cluster.yaml`；`service-collaboration.yaml`（config.go:264、main.go:73）对应的仓库文件是 `nacos/DEFAULT_GROUP/service-collaboration`，没有扩展名，导入脚本按 `-Filter *.yaml` 过滤（:64），所以永远不会发布；它的顶层键是 `service协同`（见 05 报告 4.3），而 Go 解析的是 `collaboration`（config.go:283-297）。
4. 网关路由定义了两遍：`gateway-routes.yaml:8-45` 和 `couponkill-gateway-dev.yaml:43-81`。两份内容目前相同。后导入的 `${spring.application.name}.yaml` 会整体覆盖列表（推断，Spring Boot 对 import 顺序和 list 属性的语义），以后只改其中一份就会失效。
5. order 调 Go 用的是 `http://seckill-go-svc:8083`（GoSeckillFeignClient.java:16-17），而 Helm 里的 Service 暴露的是 80->8083（05 报告 4.1），k8s-istio 的 VS/DR 也按 80 写（k8s-istio/istio_VirtualService.yaml:35-38）。在集群里按 8083 访问会失败（推断）。Go 路径目前默认冻结（couponkill-order-service-dev.yaml:81-82 `go.enabled: false`），所以影响有限。
6. connector 的 application.yml 用 `${REDIS_HOST:127.0.0.1}`（:31-32）和 `${CONNECTOR_DB_HOST:127.0.0.1}`（:23）接收环境变量。但 Nacos 的 common.yaml:21 写死 `spring.data.redis.host: redis-master`，couponkill-connector-service-dev.yaml:13 写死 `spring.datasource.url`。只要这两份 Nacos 配置被加载，就会覆盖 application.yml，这两个环境变量随之失效（推断：`spring.config.import` 导入的文档优先级高于导入它的文件）。
7. 本地兜底 `couponkill-go-service/config.yaml:15-25` 连 `localhost:5432`，而 compose 把 PG 映射到宿主机 5433（docker-compose.migration.yml:20）。只有经 Nacos 下发的配置才会被 `Localize` 改写成 5433（import-nacos-local.ps1:30）。已核实。

## 2. MQ 矩阵

生产者配置在 common：`acks=all`，`delivery-timeout-ms=10000`，`enable-auto-commit: true`，`auto-offset-reset: latest`（`nacos/DEFAULT_GROUP/common.yaml:7-16`）。order-service 的 `KafkaConsumerConfig` 默认把 auto-commit 读成 false、offset reset 读成 `earliest`、ack 模式 `RECORD`（`KafkaConsumerConfig.java:38-50`）。Nacos 导入成功时，common.yaml 的值会盖过 Java 缺省【推断：`spring.config.import` 覆盖同名属性】。`enableAutoCommit==true` 时 `applyAckMode` 直接返回，不设置 RECORD（`:65-68`）。

| topic | 谁发送 | 谁消费 | 消费组 | 载荷 | 备注 |
|---|---|---|---|---|---|
| `seckill_order_create` | `AsyncSeckillEnterService.java:125`，key=requestId | `SeckillOrderCreateListener.java:22-27` | `seckill-order-create-group` | `SeckillOrderCommand` | 热路径。`Future.get` 默认 500ms（order-dev.yaml:73） |
| `seckill_order_result` | `OrderServiceImpl.java:1023`，仅 `status=SUCCESS` | `SeckillOrderResultListener.java:23-36` | `seckill-order-result-group` | `OrderMessage` | SUCCESS 分支把订单改成 2。FAILED 分支没有生产方 |
| `order_created` | `OrderServiceImpl.java:155,394` | 无 `@KafkaListener` | — | `OrderMessage` | compose 会建这个 topic（05 第 4.4 节）。Chart 的 topic 列表里没有它（05 R33） |
| `seckill_compensation` | `OrderServiceImpl.java:731` | 无监听器 | — | `OrderMessage` | 只在 `handleSeckillFailure` 里发送，而该方法要等结果主题出现 FAILED |

主题名的其它副本（`kafka-config.yaml`、`messaging-connection.yaml`、values-prod）与代码默认名一致，见 05 第 4.4 节。KEDA 用的点号主题名 `seckill.order.create` 对不上，见 05 R27。

消费端反序列化关闭了类型头，只信任 `com.aliyun.seckill.common.pojo`（`KafkaConsumerConfig.java:79-81,99-101`）。落单监听器对毒消息不重试（`:114-120`）。并发缺省 8 和 4（order-dev.yaml:75-76），执行器是虚拟线程。

Go 热路径默认不生产这些主题（07a 第 2b 节）。03 报告若描述 Go 自己的 Kafka 客户端，以那份为准；本分片只确认 Java 侧没有第二个消费者组。

## 3. Redis 矩阵

coupon 与 order 扫描 common，共用 `RedisConfig` 的连接工厂。键用 `StringRedisSerializer`（`RedisConfig.java:179`）。值有两套：`RedisTemplate` 的 JSON 带 default typing（`:177-180`，风险说明在 01），以及 `StringRedisTemplate` / 裸 `stringCommands`。库存键走字符串命令，Lua 的 `GET`/`DECR` 才能 `tonumber`。

| 键 | 写入 | 读取 | TTL | 形态 |
|---|---|---|---|---|
| `coupon:stock:{couponId}` | 券：回源和预热绝对 SET（`CouponServiceImpl.java:110-114,614-616,646-648`）；常驻扣减 `decrement`（`:304`）；秒杀回补 `increment`（`:387`）。订单 Lua `DECR`（`enter_seckill.lua:36`），补偿 `INCR`（`AsyncSeckillEnterService.java:173`）。connector 经 sync-stock 再绝对 SET（`CouponServiceImpl.java:706-727`） | Lua `GET`/`EXISTS`；loadtest `GET` | 回源/预热 30–40 分钟；补偿不改 TTL | Redis 字符串整数 |
| `coupon:detail:{id}` | `getCouponById`、预热 | 同方法先读 | 30–40 分钟；空值 5 分钟 | JSON 对象 |
| `coupon:shards:{id}` | 分片扣减后刷新 | `getCachedCouponShards` | 5 分钟 | JSON 列表 |
| `coupon:last_shard:{id}` | 轮询选分片 | 同上 | 60 秒 | JSON 数字（走 RedisTemplate） |
| `coupon:out_of_stock:{id}` | 分片都没库存时 | 未发现读取方 | 1 分钟 | JSON 布尔 |
| `coupon:available` | 只在建券/改状态时 `delete` | 未发现读取方 | — | — |
| `seckill:deduct:{user}:{coupon}` | Lua `SETEX`，值为 requestId | Lua `GET`；补偿 `DEL` | 默认 300 秒（order-dev.yaml:70） | 字符串 |
| `seckill:cooldown:{user}:{coupon}` | Lua；`setUserCooldown` 再用 RedisTemplate 写一次 | Lua `EXISTS` | 默认 2 秒 | 字符串 `"1"` |
| `seckill:req:{requestId}` | `PENDING` / `SUCCESS:{orderId}` / `FAIL` | 结果查询；预约同步 | 10 分钟 | 字符串 |
| `seckill:consume:{requestId}` | `SETNX` | 仅作锁 | 30 分钟 | 字符串 |
| `user:received:{user}:{coupon}` | 常驻 Lua 占位；落单 `set true` | `hasUserReceivedCoupon` | 占位 1 小时；查询缓存 5 或 30 分钟 | 常驻路径是字符串 `"1"`，落单路径是 JSON 布尔【已核实：两套模板】 |
| `user:received:bloom` | `SETBIT` | `GETBIT` | 无 TTL | bitmap。不是 `BloomFilterConfig` 那个 Bean |
| `user:coupon:count:{user}:total` / `:seckill` | 下单后 `INCR` | 限领 | 回源写入 300 秒；`INCR` 不刷新 TTL | 见 02 `checkCouponCountLimit` |
| `reservation:fire:{id}` | `SETNX` | 预约调度 | 30 秒 | 字符串 |
| `connector:sync:all:lock` | `SETNX`，值为 UUID | 解锁时比对 | 默认 55 秒 | 字符串。connector 不扫描 common，用 Boot 自配置的字符串模板 |
| `user:id:sequence`、`user:login:{id}` | user-service | 登录键没有读取方 | 见 01 | 不在本分片重写 |

Go 在沙箱开启时用同一前缀：`coupon:stock:` 与 `user:received:`（`couponkill-go-service/cmd/server/main.go:98`，`config.go:373-374`）。默认不进热路径，所以不会和 Lua 并发写【已核实开关；并发后果未运行】。

`user:received` 的两种序列化是实打实的分叉：常驻占位用一段内联 Lua 执行 `SET key 1`（`OrderServiceImpl.java:263-276`，脚本跑在 `RedisTemplate` 上，但 Lua 写的是裸字符串）。落单成功用 `redisTemplate.opsForValue().set(..., true)`（`:1010`），值会带 Jackson 类型。`hasUserReceivedCoupon` 把读到的值转成 `Boolean`（`:492`）。JSON 布尔能转成功；裸字符串 `"1"` 不能【推断：Jackson 反序列化不会把 `"1"` 变成 `Boolean`，异常被 `:520-523` 吃掉并当成未领取】。常驻防重主要靠 Lua 的 `EXISTS`，这条读取分叉影响的是「缓存未命中前的布尔判断」。

## 4. DB 矩阵

逻辑库和表以 `charts/couponkill/scripts/init-postgres.sql` 与 `03-init-connector.sql` 为准，执行入口见 05 第 4.5 节。

| 库 | 表 | 分片键 | 写入方 | 对照 |
|---|---|---|---|---|
| `user_db_0/1` | `"user"`、`user_coupon_count` | `id` / `user_id` 的 `% 2` | user-service | 唯一约束和登录广播见 01。订单只通过 Feign 改计数 |
| `coupon_db_0/1` | `coupon_0..15` | `shard_index`：库 `(shard/16)%2`，表 `shard%16`（`coupon-service-sharding.yaml:48-75`） | coupon-service | 常驻 `updateStock` 不带 `shard_index`，会广播（02 第 6 节）。种子 1001/1002 只插入 `coupon_db_0` 的 0..15（`02-seed-demo.sql:35-45`），`coupon_db_1` 没有这两张演示券的行 |
| `coupon_db_0/1` | `stock_log_0..15` | 同上 | 无 | 只有 POJO `StockLog.java`，没有 Mapper |
| `order_db_0/1` | `order_0..15` | `user_id % 2` 库，`user_id % 16` 表（`order-service-sharding.yaml:51-68`） | order-service | `UNIQUE(user_id, coupon_id)` 在每个物理表上。`selectById` / `updateStatus` 的 WHERE 只有订单 id（`OrderMapper.xml:35-36,106-109`），取消和「改成已使用」都会广播【推断：缺分片键】。雪花 id 全局唯一时仍然只命中一行 |
| `order_db_0` | `seckill_reservation`、`user_notification` | SINGLE，不按 user 分库（`order-service-sharding.yaml:45-48`） | order-service | `order_db_1` 没有这两张表。预约 SQL 不需要带 user_id 才能路由 |
| `connector_db` | `platform_sku_binding`、`coupon_price_map` | 无分片，Hikari | connector-service | 只有 compose 的 03 脚本建库。Helm db-init 不跑 03（05 R32） |

连接池：分片 YAML 的 `maximumPoolSize` 是 32（coupon 与 order 的 sharding yaml `:12`）。order-dev 注释写「=300」，旁边的 Hikari 200 不被 ShardingSphere 使用（`couponkill-order-service-dev.yaml:17-20`）。05 R50 已记这条注释，本分片确认业务数据源类是 `*ShardingSphereConfig`，不是 yml 里的 `spring.datasource`。

coupon 的 `selectById` 带 `LIMIT 1` 且没有分片键（`CouponMapper.xml:65-69`）。同步库存用它判断券是否存在（`CouponServiceImpl.java:680`）。广播加 `LIMIT 1` 时每个物理表各自截断，客户端可能拿到多行【推断：ShardingSphere 不会把各分片的 LIMIT 收成全局 1 行】。存在性判断仍能工作；若 MyBatis 被配成单行映射，多行会抛 `TooManyResultsException`【推断，未跑】。热路径读券用的是按 `shard_index` 循环的 `selectByCouponIdAndShardIndex`，不走这条。

## 5. 返回体与错误码

Java 业务成功体只有一种：`{"code":0,"message":"success","data":...}`（`ApiResponse.java:8-26`）。`GlobalExceptionHandler` 不改 HTTP 状态（`GlobalExceptionHandler.java:11-15`），所以参数错误和业务拒绝多数仍是 HTTP 200。未捕获异常是 `code=500` 加上 `"系统异常: "` 和异常 message（`:17-20`）。

| 场景 | HTTP | body | 出处 |
|---|---|---|---|
| 业务成功 | 200 | `code=0` | `ApiResponse.success` |
| 秒杀拒绝（冷却、售罄、未开始、已结束、未预热、限流） | 200 | `ErrorCodes` 的 `10001`–`10007` 或 `10003` | `OrderController.java:150-157`；常量在 `ErrorCodes.java:11-22` |
| 登录失败等 | 200 | `20001`–`20005` | 01 第 4 节 |
| coupon 扣库存抛异常 | 200 | `code=500`，message 含异常文本 | `CouponController.java:187`。`500` 不是 `ErrorCodes.SYS_ERROR`（那是 `50000`） |
| 内部令牌不对 | 200 | `code=403`，`forbidden: invalid internal token` | `CouponController.java:237` |
| connector 参数 / 未实现平台 | 200 | `code=400` | `ConnectorExceptionHandler.java:15-18`。connector 不扫描 common 的处理器 |
| 网关无 token 或验签失败 | 401 | 空 | 01 第 4 节 |
| 网关内部路径 | 403 | `{"code":403,"message":"forbidden: internal api"}` | `InternalApiBlockFilter.java:52-55` |
| 网关非 admin 打管理写 | 403 | 空 | 01 |
| Sentinel 网关限流 / fallback | 429 | `code:429` | 01 第 3.2 节 |
| 秒杀结果查询 | 200 | data 是纯字符串，不是订单对象 | `OrderController.java:167-171` |

06 的通知冒烟把 `code==200` 也算通过（06 第 6 节第 6 行）。上表里没有成功路径会返回 `200` 这个业务码。

Go 的 HTTP 体不在这里展开，见 03。订单在沙箱双开时把 Go 的 `Result.code` 原样放进 `ApiResponse.fail`（`OrderController.java:139-141`），两套历史包装会在这一跳叠在一起。默认不走这条。

## 6. 可观测性贯通

| 手段 | 代码里有什么 | 断点 |
|---|---|---|
| Actuator | coupon、order 的 yml 写了 `refresh,health,info`（order `application.yml:37-41`，coupon `:27-31`）；connector 写了 `health,info`（`:44-48`） | 全仓库 `pom.xml` 检索 `actuator` / `micrometer` 为 0。Boot 不会因为这段 yml 就挂上端点【推断】。Chart 探针和 ServiceMonitor 仍打 `/actuator/health` 与 `/actuator/prometheus`（05 R30） |
| 日志 | slf4j。秒杀热路径用 `debug`，失败用 `error` 并带 requestId | 没有 traceId / span 字段。网关、订单、券的日志对不齐同一次请求 |
| 指标 | `OrderController` 的 `currentRequestCount` 只存在进程内（`:48,122`） | 没有注册到 Micrometer。Sentinel 资源名 `couponSeckill`（`OrderController.java:114-117`）依赖 dashboard 和 Nacos 规则 |
| Sentinel | `common.yaml:36-49` 指向 `sentinel-dashboard:8080`，dataId `couponkill-gateway-sentinel`，namespace 用 `${spring.cloud.nacos.namespace}` | 该占位符解析成 `couponkill` 而不是 120（07b 第 1.3 节第 2 条）。规则覆盖面见 05 R50 |
| Kafka | 毒消息打 warn，带 topic 和 offset | 没有消费滞后指标。KEDA 的 group/topic 与代码不一致（05 R27） |
| 健康检查脚本 | 06 的冒烟看业务 `code==0`，不看 actuator | 与 Chart 探针不是同一条 URL |

gateway 与 user 的 pom 同样没有 actuator（01 第 8 节）。五个 Java 服务在这一点上一致。

## 7. 系统级风险

| 级别 | 风险 | 会怎样 |
|---|---|---|
| 高 | 结果主题把成功单写成已使用；补偿主题和 `order_created` 没人消费 | 取消失败没有第二条消息把状态改回去。补偿发送是写进空主题。见第 2 节与 02 |
| 高 | 库存键被绝对 SET 覆盖，常驻 SQL 广播 | 受理人数和 DB 行数对不上。见第 3、4 节与 02 |
| 高 | 第 1.3 节的 Nacos / 端口 / dataId | 集群路径上服务发现、分片数据源、Go 协作配置对不齐。Helm 侧见 05 R10–R13 |
| 中 | auto-commit 与 RECORD 两套缺省 | 导入 common.yaml 后，监听器抛错前 offset 可能已经按时间提交【推断：Kafka 客户端 auto-commit 语义】。至少一次会退化成「失败也提交」 |
| 中 | `user:received` 两种编码 | 落单写入的 JSON 布尔和常驻 Lua 的 `"1"` 不是同一种值。读取异常时当成未领取，限领之后的 DB 唯一约束仍在 |
| 中 | 三套错误体 | 前端若只判断 HTTP 200，会把业务失败当成成功；若只判断 body 里的 `code==200`，会把真成功判失败。06 的通知冒烟属于后者的放宽 |
| 中 | 探针与指标 | 有 NetworkPolicy 或编排按 `/actuator/health` 判活时，进程会一直不健康【推断，与 05 R30 相同】。本分片没有起进程 |
| 低 | `stock_log`、`coupon:available`、`coupon:out_of_stock` | 表和键存在，没有形成对账链 |

身份伪造、口令在 query、Istio 绕过网关不在这张表里重复，见 07a 第 3、4 节和 01。

## 8. 未验证 / 不确定项

- 第 1 节的端口表本次又对过 coupon / order / connector 的 `application.yml`、分片 yaml 和 Feign URL，没有改数字。user、gateway、Go 的格子沿用原表，没有发现互相矛盾的新文件。
- `spring.config.import` 是否整表覆盖 `spring.kafka.consumer.*`，没有起 Nacos 后打印 Environment。common.yaml 里这些键是存在的。
- ShardingSphere 对无分片键 `UPDATE`/`SELECT` 的实际路由和「各分片 LIMIT 1 合并后的行数」没有连数据库。
- `RedisTemplate.opsForValue().set(key, true)` 的 JSON 能否被下一次 `get` 转成 `Boolean`，按 Jackson 默认类型推断，没有往 Redis 写样本。
- Lua `DECR` 与 `RedisTemplate.decrement` 是否打在同一个字符串值上：两边都是 Redis 字符串命令【已核实 API】；并发下的最终数字没有压测。
- actuator 缺失后访问 `/actuator/health` 的 HTTP 状态没有请求过。结论与 05 一样，是「pom 没有依赖」。
- Go 进程在默认端口 8090 与 Nacos 8083 之间的选择见第 1 节，本分片没有启动 Go。

## 9. 候选 OpenSpec capability

只写跨模块契约的现状，不建 spec。

- `kafka-topic-contract`：四个主题名、两个消费组、`SeckillOrderCommand` 与 `OrderMessage` 分开。现状是结果主题会改订单状态，另外两个主题没有消费者，auto-commit 与 ack 模式的缺省互相打架。
- `redis-stock-key`：`coupon:stock:{id}` 是秒杀闸门，字符串整数。现状是预热、回源和 connector 同步会用 DB 或平台数覆盖它。
- `api-response-envelope`：业务体 `code/message/data`，成功为 0。现状是网关 401/403 和控制器里的 `500`/`403`/`400` 不在同一套常量里，HTTP 状态也不统一。
- `process-health`：编排想要的是 `/actuator/health` 与 `/actuator/prometheus`。现状是 yml 写了 exposure，依赖没引进来。

