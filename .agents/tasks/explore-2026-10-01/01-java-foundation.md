# 01 · java-foundation 分片报告（root pom / common / gateway / user-service）

> 范围：根 `pom.xml`、`couponkill-common/`、`couponkill-gateway/`、`couponkill-user-service/`（src/main、src/test、resources、pom）。业务三件套的热路径见 `02-java-business.md`；Helm/Jenkins 见 `05-deploy-cicd.md`；鉴权链已写进 `07a-crosscut-flows.md` 第 3 节的，这里只补网关/用户侧新证据。
> 方式：只读静态阅读 + 只读 git。未运行 Maven / 服务 / 脚本。基线：`main` @ `3969213`（2026-07-18，已核对 `git rev-parse HEAD`）。
> 标注约定：【已核实】= 读代码/配置直接可见；【推断】= 基于框架语义的静态推理，未运行验证。路径相对仓库根。敏感值只写 key 名或长度，不抄正文。

## 0. TL;DR

- 这块是什么：父 POM 把六个 Java 模块钉在 JDK 25 / Boot 4.0.5。`couponkill-common` 是共享契约（响应体、错误码、POJO、Redis/Kafka/Feign、`X-User-Id` 上下文）。网关是 WebFlux 入口，做 JWT、券/Connector 管理写门禁、内部接口封禁。user-service 负责注册登录、BCrypt、按 `id % 2` 分到 `user_db_0/1`，以及给订单服务用的券计数。
- 整体健康度：本地 Nacos 导入成功时，签发端和网关用的是同一段占位密钥，登录契约（`@RequestParam`）和已提交的 Web 前端对得上。用户库的分片方式和登录查询方式互相打架，身份头在几条旁路上仍然可伪造，user-service 的编译器配置和父 POM 不一致。
- 最重要的 5 个发现：
  1. 【高·安全】登录和注册把口令放在 query（`UserController.java:30-42`）。`User.password` 没有 `@JsonIgnore`（`User.java:24`），注册和 `/profile` 会把 BCrypt 哈希带回客户端。Web 前端按这个契约发请求（见 `04-frontend.md`）。
  2. 【高·正确性】`"user".username` 的 UNIQUE 是每个物理库一份（`charts/couponkill/scripts/init-postgres.sql:32,58`），登录 SQL 的 WHERE 只有 `username`，没有分片键 `id`（`UserMapper.xml:28-32`）。ShardingSphere 会广播到两个库。并发注册可以在 `user_db_0` 和 `user_db_1` 各插入一条同名用户【推断：唯一约束跨库不生效】。
  3. 【高·鉴权】网关 `application.yml:26` 把 `jwt.secret` 的缺省值写成 `${JWT_SECRET:secret}`（6 字符，HS256 密钥不够长）。签发端 `@Value` 默认是另一段 64 字符占位串（`JwtUtils.java:19`，`AuthController.java:27`）。Nacos 三份副本与这段占位串相同，导入成功时两边一致【已核实，正文不抄】。Nacos 没导入时网关验签会全部失败【推断】。占位串明文进仓库的问题见 05 报告 R34，这里不重复。
  4. 【高·鉴权】`/api/v1/user/coupon/**` 和 `/api/v1/user/batch/**` 在服务内部不核对调用者（`UserController.java:77-108`，`BatchUserController.java:28-48`）。网关用前缀整段封死（`InternalApiBlockFilter.java:25-31`），浏览器进不来；能直连 Pod 的调用方可以改任意用户的券计数，或一次插入最多 10 万测试用户。`X-Admin-Token` 命中时只补 `X-User-Roles: admin`，不覆盖客户端自带的 `X-User-Id`（`JwtAuthGlobalFilter.java:73-78`）。mock token 签发任意角色的结论已在 07a 第 3 节，不重复推演。
  5. 【高·构建 / 可用性】user-service 把编译器钉在 source/target 21、插件 3.11.0（`couponkill-user-service/pom.xml:120-131`），父 POM 是 `release` 25、插件 3.14.0（`pom.xml:270`）。`PasswordEncoder` 被使用，但没有任何 pom 声明 `spring-security-crypto`。分片配置热更新只换私有字段并关闭旧数据源，和 02 报告第 5 点是同一缺陷（`UserShardingSphereConfig.java:81-100`）。

## 1. 范围与技术栈

| 项 | 版本 / 事实 | 证据 |
|---|---|---|
| 父工程 | `com.aliyun.seckill:couponkill-parent:1.0-SNAPSHOT`，packaging pom，6 个子模块（gateway、common、coupon、order、user、connector） | `pom.xml:7-22` |
| JDK | 属性 `java.version` / source / target = 25；compiler 插件 3.14.0，`<release>25</release>`，Lombok 注解处理 | `pom.xml:55-57,265-282` |
| Boot / Cloud / SCA | 4.0.5 / 2025.1.0 / 2025.1.0.0 | `pom.xml:62-64` |
| 数据与中间件版本（BOM） | PostgreSQL 42.7.4；MyBatis starter 4.0.1；Nacos 客户端属性 3.1.1；jjwt 0.11.5；Sentinel 1.8.9；Guava 33.4.0-jre；fastjson **1.2.83**；pagehelper starter **4.1.0**；Redisson 3.45.1（注释写明业务未用） | `pom.xml:67-88,136-141,215-219` |
| 只出现在 properties、未进 dependencyManagement | `aliyun-sdk-oss`、`fastdfs-client`、`alipay-sdk-java`、`admin-starter-server` | `pom.xml:74-77` |
| common | 库模块，无 `main`。显式依赖 `spring-boot-jackson2`、Nacos discovery/config、Sentinel（含 gateway adapter）、Kafka、Redis+commons-pool2、Feign、jjwt、fastjson、pagehelper、protostuff、Druid、Hikari、PostgreSQL | `couponkill-common/pom.xml` |
| gateway | `spring-cloud-starter-gateway-server-webflux` + WebFlux + Nacos + Sentinel gateway adapter + LoadBalancer + Redis starter + jjwt。不依赖 `couponkill-common`，没有 actuator | `couponkill-gateway/pom.xml:15-97` |
| user-service | 依赖 common、ShardingSphere-JDBC 5.5.2、Web、Nacos、Sentinel、PostgreSQL、Redis、MyBatis、OpenFeign、Kafka、LoadBalancer。没有 actuator，没有 spring-security 依赖。编译器覆盖为 21 | `couponkill-user-service/pom.xml:14-132` |
| 测试 | gateway：路径分类单测 + 一次 JWT 验签冒烟。user：只有 `@SpringBootTest` `contextLoads`。CI 排除 `*ApplicationTests` 的事实见 05 报告，不重复 | 对应 `src/test` |

仓库里已有 `*/target/`。本次没有跑 Maven，不用这些目录证明当前源码能编过。

## 2. 结构地图

```
pom.xml                                 父 BOM、compiler release 25、6 modules
couponkill-common/src/main/java/com/aliyun/seckill/common/
  api/ApiResponse, ErrorCodes           code=0 成功；业务码 10xxx/20xxx/50xxx
  result/Result, enums/ResultCode       历史包装，新代码禁止再引入（ErrorCodes 注释）
  pojo/User, Coupon, Order, ...         跨服务 POJO；User.id 用 JsonFormat STRING
  context/UserContext                   JDK 25 ScopedValue + 请求头回退
  filter/UserContextFilter              仅 SERVLET；读 X-User-Id / X-User-Roles
  interceptor/UserContextInterceptor    空拦截器，只打 debug 日志
  config/RedisConfig                    自建 Lettuce 池 + 带 default typing 的 ObjectMapper
  config/KafkaBaseConfig                JSON 生产者，acks=all，幂等，delivery 10s
  config/FeignConfig                    透传 X-User-Id，不透传 roles
  config/SentinelRuleConfig             fastjson 解析 FlowRule，要有 ds1.nacos.server-addr 才装配
  config/SnowflakeConfig, WebConfig, TpConfig, ServiceGoConfig
  utils/JwtUtils                        签发；getBytes() 无字符集
  exception/GlobalExceptionHandler      BusinessException -> ApiResponse，HTTP 仍是 200
couponkill-gateway/                       (Nacos 端口见 07b；本模块 yml 不写 server.port)
  security/JwtAuthGlobalFilter          order=-100，白名单 / JWT / admin
  security/InternalApiBlockFilter       order=-200，封内部写
  utils/JwtUtil                         验签；与 common 的 JwtUtils 是两份实现
  config/SentinelGatewayConfig          SentinelGatewayFilter @Order(-1)
  config/GatewaySentinelConfig          限流响应体 code=429
  config/SentinelGatewayFilterFactory   路由上的 Sentinel 过滤器工厂
  FallbackController                    /fallback/{user,coupon,order,connector}
couponkill-user-service/
  controller/UserController             /api/v1/user/login|register|profile|coupon/**
  controller/AuthController             POST /api/v1/auth/token/mock
  controller/BatchUserController        POST /api/v1/user/batch/generate
  service/Impl/UserServiceImpl          BCrypt、Redis 号段、登录写 Redis
  config/UserShardingSphereConfig       Nacos ns=998，dataId=user-service-sharding.yaml
  config/SecurityConfig                 只暴露 BCryptPasswordEncoder
  mapper/UserMapper.xml                 逻辑表 "user"、user_coupon_count
  task/UserTask                         @Scheduled 每周日；应用类没有 @EnableScheduling
```

user-service 启动类扫描 `com.aliyun.seckill.couponkilluserservice` 和 `com.aliyun.seckill.common`（`CouponkillUserServiceApplication.java:14`）。网关不扫描 common，所以用不到 `UserContextFilter`（它也声明了只在 Servlet 应用生效）。

### 2.1 用户模型（按代码和 DDL）

- 表 `"user"`：每个库 `user_db_0`、`user_db_1` 各一张，`username VARCHAR(50) NOT NULL UNIQUE`，`password VARCHAR(100)`，`status` 默认 1。主键 `id`，`GENERATED BY DEFAULT AS IDENTITY`，应用插入时自带 id（`init-postgres.sql:30-41`）。
- 分片：逻辑表 `user` 的分片列是 `id`，算法 `user-db-$->{id % 2}`；`user_coupon_count` 的分片列是 `user_id`，同样 `% 2`（`nacos/shard/DEFAULT_GROUP/user-service-sharding.yaml:47-67`）。没有分表。
- 券计数：`total/seckill/normal/expired` 加 `version`。真正被登录后路径调用的更新 SQL 不读 `version`，直接 `count = count + #{count}`（`UserCouponCountMapper.java:43-51`）。带乐观锁的 `update(...)` 没有调用方。
- 身份：登录成功写入 JWT claims `userId` + `roles`。`connector.admin.user-ids` 默认 `10000` 的用户得到 `["admin","user"]`，其他人是 `["user"]`（`UserServiceImpl.java:157-161`）。

## 3. 核心流程

### 3.1 注册 / 登录 / 当前用户

```
POST /api/v1/user/register?username&password&phone     白名单，不校验 JWT
  -> 长度/手机号正则
  -> selectByUsername（无分片键，广播）
  -> Redis INCR user:id:sequence，一次取 100 个号；Redis 不可用则雪花
  -> BCrypt 后 insertUser + insert user_coupon_count
  -> ApiResponse 里的 User 含 password 哈希

POST /api/v1/user/login?username&password              白名单
  -> selectByUsername + passwordEncoder.matches
  -> 不看 status
  -> jwtUtils.generateToken(id, roles)
  -> SET user:login:{id} = token EX 24h     （全仓库没有读取这个 key 的代码）
  -> {token, userId(string), username, roles}   不含 password

GET /api/v1/user/profile                               要 JWT
  -> UserContext.requireCurrentUserId()
  -> selectById，原样返回 User（含哈希）
```

号段生成在单 JVM 内 `synchronized`（`UserServiceImpl.java:71-86`）。多副本靠 Redis `INCRBY` 分号【推断：Redis 可用时不撞号】。Redis 返回空时改走雪花，和号段不是同一序列；之后 Redis 恢复会继续发小号，和已经发出的雪花 id 空间不同，撞号概率低，但分片注释里写的「保证落到两个库」对雪花只是 `% 2` 仍然成立。

`UserTask` 注释写「20 天未活跃」，实现是 `minusMonths(6)`，而且 `CouponkillUserServiceApplication` 没有 `@EnableScheduling`，这个 `@Scheduled` 不会跑【已核实：两个文件都读过；Spring 不启用调度是框架语义，标推断】。

### 3.2 网关鉴权（本分片拥有的分支）

过滤器顺序【已核实】：`InternalApiBlockFilter` -200，然后 `JwtAuthGlobalFilter` -100，然后 Sentinel 网关过滤器 -1。

白名单、mock token、白名单不清洗 `X-User-*`、下游无条件信任 `X-User-Id`，07a 第 3 节已经写完。本分片补三条：

1. 管理写在验 JWT 之前有一条短路：`connector.admin.token` 非空且等于 `X-Admin-Token` 时，直接放行，并 `header("X-User-Roles","admin")`。不设置、也不清除 `X-User-Id`（`JwtAuthGlobalFilter.java:71-79`）。Nacos 里该 token 默认是空环境变量（`nacos/DEFAULT_GROUP/couponkill-gateway-dev.yaml:97`），所以默认不启用【已核实】。一旦配上，持有该 token 的人可以自带任意 `X-User-Id`【推断：与 07a 已核实的 UserContextFilter 组合】。
2. 券管理写门禁只覆盖四条：`/api/v1/coupon/create`、路径含 `/seckill-window`、`/api/v1/coupon/{id}/status`、`DELETE /api/v1/coupon/{id}`（`JwtAuthGlobalFilter.java:146-159`）。单测只断言这组布尔值（`JwtAuthCouponAdminPathTest.java`）。库存扣减/预热走 InternalApiBlock，不走 admin 门禁。
3. 普通 JWT 成功路径会覆写 `X-User-Id`、`X-Authenticated`、`X-User-Roles`（`JwtAuthGlobalFilter.java:113-118`）。`mutate().header` 是否覆盖同名头，按 Spring 的 Builder 约定是覆盖【推断，未跑网关】。

`/fallback/**` 在白名单里。回退控制器固定 HTTP 429 + `code:429`（`FallbackController.java:12-38`）。

### 3.3 分片数据源

启动时用独立的 Nacos `ConfigService`（namespace 来自 `shardingsphere.namespace`，user 的 yml 写死 `998`）拉取 `user-service-sharding.yaml`，空内容直接抛 `IllegalStateException`，服务起不来（`UserShardingSphereConfig.java:48-53`）。监听器收到变更后 `createDataSource`，关掉旧的 `ShardingSphereDataSource`，把新实例赋给字段 `dataSource`。`@Bean` 方法早已返回旧对象，Spring 注入的仍是旧引用。这和 02 报告里 coupon/order 的写法相同，后果也相同：一次分片配置推送会让 user-service 的 DB 调用打到已关闭的数据源【推断：关闭语义来自 `ShardingSphereDataSource.close()`，未在运行中触发】。

旁边还监听 `middleware-cluster-config.yaml`，回调只打日志（`UserShardingSphereConfig.java:125-130`）。仓库里对应文件名是 `middleware-cluster.yaml`，对不齐的事实见 07b 第 1.3 节第 3 条。

分片 YAML 里的 JDBC 主机是 `postgres:5432`，用户名口令是本地演示值。本地导入脚本会改主机名，细节不在本分片重复。

## 4. 对外契约

| 入口 | 方法与参数 | 鉴权（经网关时） | 成功体 |
|---|---|---|---|
| `/api/v1/user/register` | POST，`username`/`password`/`phone` 全是 `@RequestParam` | 白名单 | `ApiResponse<User>`，含 password 哈希 |
| `/api/v1/user/login` | POST，`username`/`password` 为 `@RequestParam` | 白名单 | `code=0`，data 为 token、字符串 userId、username、roles |
| `/api/v1/user/profile` | GET，无参，身份取 `X-User-Id` | JWT | `ApiResponse<User>`，含哈希 |
| `/api/v1/auth/token/mock` | POST JSON `{userId, roles}` | 白名单；行为见 07a | Bearer token |
| `/api/v1/user/coupon/count` | GET `userId` | 网关前缀封禁；服务本身不鉴权 | 计数或 null |
| `/api/v1/user/coupon/seckill/update`、`/normal/update` | POST `userId` + `count`（可为负） | 同上 | `ApiResponse<Void>` |
| `/api/v1/user/batch/generate` | POST `startId` + `count`（1..100000） | 网关前缀封禁 | 文本成功消息 |

错误码真源是 `ErrorCodes`：用户侧 `20001` 鉴权失败、`20002` token 无效、`20003` 用户不存在、`20004` 已存在、`20005` 密码错误（`ErrorCodes.java:26-30`）。`GlobalExceptionHandler` 把 `BusinessException` 收成 `ApiResponse.fail(code, message)`，不改 HTTP 状态（`GlobalExceptionHandler.java:11-15`）。未捕获异常的 message 会拼进 `"系统异常: " + e.getMessage()` 返回给客户端（`:17-20`）。

网关自己的拒绝：缺 token / 验签失败是 HTTP 401 且空 body（`JwtAuthGlobalFilter.java:85-86`）；非管理员打管理写是 HTTP 403 空 body（`:109-110`）；内部接口是 HTTP 403 + `{"code":403,"message":"forbidden: internal api"}`（`InternalApiBlockFilter.java:52-55`）。三套失败形状不一样。

JWT 两份实现必须共享 `jwt.secret`。网关验签用 UTF-8（`JwtUtil.java:25`）。common 的 `getSigningKey()` 用 `secret.getBytes()`，不指定字符集（`JwtUtils.java:32`）；mock 签发用 UTF-8（`AuthController.java:54`）。占位串是 ASCII，两种编码结果相同【已核实】。非 ASCII 密钥会签发出网关认不出的 token【推断】。

登录把 token 存进 `user:login:{id}`，注释写「黑名单」。没有任何过滤器读取它，也没有登出接口。登出只存在于前端清 localStorage（见 04）。旧 token 在 `jwt.expiration`（Nacos 写 86400000 ms）到期前一直有效【已核实：无读取点】。

Feign 拦截器只在有 `UserContext` userId 时写入 `X-User-Id` 和 `X-Authenticated`，不写 `X-User-Roles`（`FeignConfig.java:44-51`）。订单服务直连 user-service 的计数接口因此不经过网关封禁【推断：Feign name 直连，见 07b 端口表】。

## 5. 依赖关系

```
浏览器 / 前端
  -> couponkill-gateway  (自有 JwtUtil，不依赖 common)
       -> lb://couponkill-user-service     路由在 Nacos gateway-routes.yaml（07b）
couponkill-user-service
  -> couponkill-common（组件扫描）
  -> Nacos config ns 120（common.yaml + couponkill-user-service.yaml）
  -> Nacos config ns 998（user-service-sharding.yaml）  失败则进程起不来
  -> PostgreSQL user_db_0 / user_db_1
  -> Redis（号段、登录 token、Lettuce 池）
couponkill-order-service --Feign--> user-service 的 /coupon/count 与 update
  （订单侧调用点属 02，这里只标被调契约）
```

common 被 coupon/order/connector 同样扫描。那些服务因此带上：带 default typing 的 `ObjectMapper` Bean、自建 `RedisConnectionFactory`、Kafka 生产者、Feign 的 `@Primary` ObjectMapper、全局异常处理。网关不在这条扫描里。

`RedisConfig.objectMapper()` 打开 `DefaultTyping.NON_FINAL` + `LaissezFaireSubTypeValidator`，并且 `redisTemplate()` 用这个 mapper 做 JSON value（`RedisConfig.java:83-86,174-178`）。Redis 里的值带类型信息【已核实】。同一 Bean 会不会成为 HTTP 的 ObjectMapper：`FeignConfig.feignObjectMapper()` 标了 `@Primary` 且没有 default typing（`FeignConfig.java:34-39`）。Boot 在存在多个 ObjectMapper 时注入 `@Primary`【推断】。HTTP 面更可能用无 default typing 的那只；Redis 面确定用有的那只。

user-service 的 pom 引入了 Kafka 和 `@EnableFeignClients`，本模块没有 `@FeignClient`，也没有 `KafkaTemplate` 发送点【已核实：在 user-service 源码内检索】。网关 pom 引入 Redis starter，gateway 源码没有 Redis 调用【已核实】。

pagehelper 打进 common，Java 源码没有 `PageHelper` 调用【已核实】。starter 版本号 `4.1.0` 和常见的 starter 2.x 线对不上，是否能解析未验证【推断】。fastjson 1.2.83 的唯一调用是 `SentinelRuleConfig` 把 Nacos 文本解析成 `List<FlowRule>`（`SentinelRuleConfig.java:31-34`），且要配置项 `spring.cloud.sentinel.datasource.ds1.nacos.server-addr` 才创建这个 Bean。

ShardingSphere 热更新缺陷的运行后果以 02 第 5 点为准，user-service 是同一实现的第三个副本。

## 6. 问题与风险

| 级别 | 类型 | 结论 | 证据 |
|---|---|---|---|
| 高 | 安全 | 口令出现在 URL query，会进访问日志和代理日志。注册/资料接口返回密码哈希 | `UserController.java:30-42`；`User.java:24`；`UserServiceImpl.java:109` 返回的就是这个对象 |
| 高 | 正确性 | 用户名唯一性是分片库本地的。广播查询在两库都命中时，MyBatis 的单行查询会抛 `TooManyResultsException`【推断】。并发双注册可以制造这种状态【推断】 | `init-postgres.sql:32`；`UserMapper.xml:28-32`；分片列是 `id`（sharding yaml:51-52） |
| 高 | 鉴权 | 网关缺省密钥是 `secret`，对不上签发端缺省占位串；`Keys.hmacShaKeyFor` 对短密钥抛弱密钥异常，`verifyToken` 吞掉后返回 false，全部鉴权请求 401【推断】 | `couponkill-gateway/src/main/resources/application.yml:26`；`JwtUtil.java:36-45`；`JwtUtils.java:19` |
| 高 | 鉴权 | 计数更新和批量造用户没有调用者校验。`count` 可以是负数，SQL 是 `seckill_count + #{count}`，没有下限。网关封的是前缀，不是身份 | `UserCouponCountMapper.java:43-46`；`BatchUserController.java:37`；`InternalApiBlockFilter.java:26-27` |
| 高 | 鉴权 | 配了 `CONNECTOR_ADMIN_TOKEN` 之后，管理令牌等于一张不绑定用户的管理员票，用户 id 由客户端头决定 | `JwtAuthGlobalFilter.java:73-78`；07a 已核实下游信任该头 |
| 高 | 可用性 | 分片 YAML 一推送就关闭 Spring 仍在使用的数据源 | `UserShardingSphereConfig.java:84-99`；机制同 02 第 5 点 |
| 中 | 构建 | 子模块 compiler 21 / 插件 3.11.0 与父 POM release 25 / 插件 3.14.0 叠在一起。有效 POM 是合并还是子配置盖住父配置，没跑 Maven【推断】。风险是 user-service 产出 21 字节码，或 release 与 source 同时存在导致编译失败 | 两个 pom 的插件块 |
| 中 | 构建 | `BCryptPasswordEncoder` 在源码里，pom 没有 security 依赖。干净编译是否靠传递依赖带上 `spring-security-crypto`，未验证 | `SecurityConfig.java:6-14`；user 与 common 的 pom |
| 中 | 安全 | Redis JSON 使用 default typing。谁能写这些 key，谁就可能在反序列化时指定类型【推断，未构造】 | `RedisConfig.java:83-86` |
| 中 | 正确性 | 登录不看 `status`。停用用户只要哈希还在就能拿 token | `UserServiceImpl.java:127-142` 无 status 判断 |
| 中 | 正确性 | `user:login:{id}` 只写不读，不能作废 token。注释与行为不符 | `UserServiceImpl.java:143-146`；全仓库无第二处 `USER_LOGIN_KEY` / `user:login:` |
| 低 | 死代码 | `UserContextInterceptor` 空实现；`UserMapper.xml` 里的 `insert`/`selectAll`/`updatePassword` 不在 `UserMapper.java` 上；`ShardingUtils` 无引用；网关 Redis、user 的 Kafka/Feign 客户端无使用点 | 对应文件与检索 |
| 低 | 信息泄露 | 未捕获异常的 message 返回给调用方 | `GlobalExceptionHandler.java:19` |
| 低 | 依赖 | fastjson 1.2.83 仍在 BOM 和 common 里，调用点只有 Sentinel 规则解析 | `pom.xml:88`；`SentinelRuleConfig.java:7-8` |

Istio 绕过网关、以及下游把 `X-User-Id` 当已登录用户，是 05 报告 R16 和 07a 第 3 节的结论，本分片不另开一条。

mock 接口没有 profile 开关这件事同样以 07a 为准。本分片只确认实现位置在 `AuthController.java:36-62`，并且网关白名单包含该路径（`JwtAuthGlobalFilter.java:35`）。

## 6.5 全模块编译结果

未执行。只读约束禁止跑 Maven。不把 `couponkill-*/target/` 里已有的 class 当成这次源码的编译通过证据。

user-service 与父 POM 的编译器冲突、以及 security 依赖是否传递可见，都留在第 8 节。

## 7. 最近活跃度

`git log` 触及本分片路径的提交共 61 个，最近一次是基线本身：

| 提交 | 日期 | 说明 |
|---|---|---|
| `3969213` | 2026-07-18 | 网关 JWT admin 放行券管理写（与前端管理台同一提交） |
| `2dbcdee` | 2026-07-18 | Connector 比价路径进白名单 |
| `3209388` | 2026-07-18 | Long 以字符串序列化（`User.id` 的 `@JsonFormat`） |
| `747677d` | 2026-07-18 | JWT HTTP 冒烟测试 |
| `9adb042` | 2026-07-18 | 冻结 Go 热路径、统一订单 API |

## 8. 未验证 / 不确定项

- 没有跑 `mvn`，不知道 user-service 在 JDK 25 上的有效编译参数，也不知道 `spring-security-crypto` 能否从 ShardingSphere 或其他传递依赖进来。
- 没有起 Nacos。`jwt.secret` 的结论是把 application.yml、三份 `nacos/**` 和 `@Value` 默认值对过文本：导入成功时网关 yaml 与 user yaml 的 secret 字段相同，且等于代码里的占位默认；导入失败时网关落到 `secret`。
- 没有对 ShardingSphere 做双库并发注册实验。广播与「每库一个 UNIQUE」是读配置得到的，双写成功是推断。
- `ServerHttpRequest.Builder.header` 覆盖同名头、`hmacShaKeyFor` 拒绝短密钥、`@EnableScheduling` 缺失导致定时任务不跑，都是框架文档语义，未在进程里打点。
- 网关与 user 的 pom 都没有 `spring-boot-starter-actuator`。05 报告里的探针路径是 `/actuator/health`。缺依赖则该路径不会由 Boot 暴露【推断】。探针清单本身不在这里重写。

## 9. 候选 OpenSpec capability（只写现状，不建 spec）

本次只交付探查报告，不在 `openspec/specs` 下落文件。

- `user-account`：注册、登录、资料。现状是 query 传口令、返回哈希、不校验 status、token 不能作废。
- `gateway-auth`：JWT 验签、券/Connector 管理写、内部前缀封禁。现状是白名单与 mock 的缺口在 07a，管理令牌短路在本报告第 3.2 节。
- `user-coupon-count`：按 userId 加减计数，供订单限领。现状是无调用者校验、无下限、乐观锁方法无人调用，网关侧整段不可达。
