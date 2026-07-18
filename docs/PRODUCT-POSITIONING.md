# CouponKill 产品定位与可执行路线图

> 读者：老板 / 产品 / 架构 / 开发  
> 性质：产品定位真源（本轮交付物）  
> 仓库现状基线：2026-07-13 只读摸底（connector 绑定、Lua→Kafka→fulfill 热路径、前端 Seckill/ConnectorAdmin）  
> **不做 commit/push**；本文可随迭代修订，修订时请同步「诚实边界」章节。

---

## 0. 一句话定位（金句）

**CouponKill =「本地高并发秒杀券」+「电商 SKU 绑定与库存旁路」的云原生演示/实训平台；  
预约帮抢的核心价值是「到点代发本地秒杀入队」，不是跨平台代下单灰产。**

若不能把「预约 → 到点自动走 `/order/seckill`」做成可靠体验，用户确实没有理由不用京东/淘宝原生秒杀——这一点必须写进产品纪律，而不是用营销话术糊过去。

---

## 1. 核心灵魂拷问（诚实回答）

### 1.1 问题

> 若不能预约/帮抢，用户为什么不直接去电商平台抢？

### 1.2 结论（必须诚实）

| 对比维度 | 直接去京东/淘宝抢 | 用本产品（当前真实能力） | 用本产品（P0 预约帮抢落地后） |
|----------|-------------------|--------------------------|-------------------------------|
| 商品真源 | 平台官方 | **本地秒杀券**；可与平台 SKU **绑定**（库存旁路同步） | 同左；预约任务绑定本地券+可选展示绑定 SKU |
| 下单对象 | 平台订单 | **本系统订单**（`order` 服务落单） | 同左；**不是**代用户在京东下单 |
| 抢购动作 | 用户自己蹲点/App 提醒 | 用户手动点「秒杀」 | **预约后系统到点代发本地入队**（核心差异） |
| 比价 | 平台内/跨 App | **无产品级比价**；管理端仅有单 SKU probe 价 | 秒杀后/成功页展示多平台只读比价（P1） |
| 合规 | 平台 ToS 内 | 演示/自有库存语义清晰 | 仍禁止破解验证码、绕过风控、外挂代下单 |

**差异化价值主张要成立，必须同时满足：**

1. **预约帮抢是主路径**：用户不必盯着开售瞬间；系统在开售窗口代其发起**本地**秒杀请求。  
2. **券 ↔ 平台 SKU 可解释**：用户知道这张秒杀券对应哪个外部商品（绑定信息可见），库存来自同步旁路而非玄学。  
3. **秒杀后同品比价是增益，不是替代**：帮用户判断「抢到的券/价是否划算」，不是伪装成商城导购。

**若砍掉预约帮抢**：本仓库当前更接近「云原生秒杀技术展柜 + Connector 库存旁路」，对 C 端用户相对电商原生秒杀 **没有足够产品理由**——只对技术面试/架构演示有价值。产品路线必须承认这一点。

### 1.3 帮抢边界（硬披露 · 禁止吹牛）

| 说法 | 是否成立 | 说明 |
|------|----------|------|
| 「到点代发本地秒杀入队」 | ✅ 本产品应做的帮抢 | 调度器在开售时刻以**被授权用户**身份调用现有 `POST /order/seckill` → Lua → Kafka → fulfill |
| 「跨平台代你在京东/淘宝下单」 | ❌ 本产品不做 | 触及平台 ToS、账号安全、灰产；仓库禁区 |
| 「绕过验证码/风控脚本」 | ❌ 本产品不做 | 明确禁区 |
| 「Connector 同步 = 热路径抢购」 | ❌ 误解 | Connector 是旁路写 Redis 库存；热路径不经过 Connector Feign |

---

## 2. 完整产品定位

### 2.1 一句话定位

面向「想准时抢本地秒杀券、并看清对应电商同品价」的用户，提供 **预约 → 到点自动入队 → 结果通知 → 同品比价参考** 的秒杀助手；技术底座是已验证的高并发本地秒杀链路与 SKU 绑定旁路。

### 2.2 目标用户

| 角色 | 诉求 | 本产品如何服务 |
|------|------|----------------|
| C 端抢券用户 | 不想蹲点；开售瞬间忙不过来 | 预约帮抢（P0） |
| 价格敏感用户 | 抢完想确认是否划算 | 多平台同品只读比价（P1） |
| 运营/管理员 | 把本地秒杀券绑到真实/Mock SKU，校准库存 | ConnectorAdmin + `platform_sku_binding`（已有雏形） |
| 技术面试/实训观众 | 看清云原生秒杀架构 | README + 热路径演示（已有） |

### 2.3 非目标用户

- 想「全自动黑产脚本刷平台官方秒杀」的人 → **明确拒绝**  
- 想把本系统当完整电商商城（购物车、支付、物流、售后）的人 → **不做**  
- 想用本系统替代京东/淘宝账号体系与支付的人 → **不做**  
- 需要 TB/PDD 正式开放平台生产能力的人 → **二期以后**，当前 stub

### 2.4 Jobs-to-be-Done（JTBD）

| Job | 用户原话级表述 | 成功标准 |
|-----|----------------|----------|
| JTBD-1 预约帮抢 | 「开售时我可能不在手机前，系统帮我抢本地券」 | 预约成功 → 到点自动入队 → 成功/失败可感知通知 |
| JTBD-2 券+SKU 可解释 | 「我要知道这张秒杀券对应哪个平台商品」 | 秒杀详情展示 binding（platform + externalSkuId + 标题快照） |
| JTBD-3 同品比价 | 「抢完我想看看别的平台同款多少钱」 | 成功页/详情页展示 ≥2 平台只读价 + 可信度说明 |
| JTBD-4 运营绑品 | 「我要把券绑到 JD/Mock SKU 并同步库存」 | ConnectorAdmin 创建绑定 + sync（已有） |

### 2.5 核心场景（用户旅程）

```
运营：创建秒杀券(type=2) → Connector 绑定 platform+SKU → 同步库存到 Redis
用户：浏览秒杀场次 → 一键预约（选券）→ 等待开售
系统：到点调度 → 以用户身份 enter_seckill → QUEUED → fulfill → 通知
用户：查看结果 → （可选）同品多平台比价 → 去外部平台自行决策是否下单
```

### 2.6 价值主张画布

| 板块 | 内容 |
|------|------|
| **我们提供** | 预约窗口、到点代发本地秒杀、异步结果、券-SKU 绑定可见性、（P1）同品比价参考 |
| **用户付出** | 注册登录、提前预约、信任系统在开售窗口代其发起请求、接受「本地券 ≠ 平台订单」 |
| **为何比自己蹲点更好** | 时间解放（不必盯秒）、失败可解释（冷却/无库存/未预热）、可复盘；**不是**保证 100% 抢到 |
| **为何比电商原生更好（有限）** | 仅在「本地活动券 + 预约帮抢 + 跨平台比价参考」组合下成立；单点拼「平台官方秒杀体验」拼不过 |
| **我们不承诺** | 跨平台代下单、绕过风控、比官方更快的灰产通道、完整商城履约 |

### 2.7 竞品对照与合规边界

| 对照物 | 他们强在哪 | 我们差异 | 合规边界 |
|--------|------------|----------|----------|
| 电商原生秒杀（京东/淘宝） | 真商品、真支付、真物流、官方提醒 | 本地券活动 + 预约代发本地入队 + 跨平台比价参考 | 不爬敏感接口、不模拟登录刷单 |
| 比价 App | 全网价库、历史价 | 绑定 SKU 的**轻量只读比价**，场景绑在「秒杀后」 | 只用官方开放 API / MOCK；标注数据时效与可信度 |
| 抢购脚本灰产 | 「更快」的假象 | **明确不做**；产品文案禁止暗示 | 禁止验证码破解、设备指纹伪造、共享账号代下单教程 |

**合法/合规红线（产品与工程共同遵守）：**

1. 不实现、不文档化「绕过电商风控 / 验证码破解」。  
2. Connector 仅使用开放能力或 MOCK；JD 无精确库存时 **SKIP**，禁止默认虚增（已有安全合并语义）。  
3. 帮抢 = 本地热路径代发；对外文案禁止写成「帮你抢京东」。  
4. 用户授权：预约即授权「到点以我的账号发起本地秒杀」；可取消预约。

### 2.8 MVP vs 不做清单

#### MVP（必须做成，否则定位破产）

1. **活动时间窗**：券或活动维度的 `seckill_start_at` / `seckill_end_at`（当前后端无此字段，前端已诚实提示）。  
2. **预约**：用户对某秒杀券预约；开售前可取消；一人一券预约幂等。  
3. **到点帮抢调度**：开售触发 → 代发 `POST /order/seckill`（兼容现有 Lua 返回码，**禁止改 Lua 语义**）。  
4. **结果通知**：站内结果（轮询/结果页）+ 可选站内消息；失败原因映射现有 ErrorCodes。  
5. **绑定可见**：秒杀详情展示已绑定的 platform + SKU（只读，来自 connector）。  

#### 明确不做（砍掉，避免半吊子商城）

- 购物车、支付、物流、售后、评价  
- 跨平台代下单 / 账号托管代购  
- 全网爬虫比价引擎、历史价大数据（P2 再评估，非 MVP）  
- TB/PDD 正式生产对接（枚举已 stub，二期）  
- 改 `enter_seckill.lua` 返回码或绕过 Lua 直接扣 DB（type=2 已拒绝普通下单）  
- 「社交帮抢」（帮好友抢、分享助力）——易与灰产混淆，P2+ 且需单独合规评审  

---

## 3. 现状摸底（事实 · 2026-07-13）

### 3.1 已有能力

| 域 | 现状 | 关键锚点 |
|----|------|----------|
| **SKU 绑定** | `platform_sku_binding`：一券一绑、平台+SKU 唯一；平台 MOCK/JD 可用，TB/PDD stub | `charts/couponkill/scripts/03-init-connector.sql`；`BindingService`；`ConnectorController` `/api/v1/connector/*` |
| **库存旁路** | 定时 `StockSyncJob` → 拉平台库存 → Feign `sync-stock` → Redis `coupon:stock:{couponId}`；安全合并不抬高库存 | `StockSyncJob`；`CouponServiceImpl.syncRedisStock` |
| **秒杀热路径** | `POST /order/seckill` → Lua 预扣 → Kafka `seckill_order_create` → fulfill → `seckill_order_result` | `enter_seckill.lua`；`AsyncSeckillEnterService`；`SeckillOrderCreateListener` |
| **前端秒杀** | 列表 → 登录 → 秒杀 → 轮询 `GET /order/check/{couponId}` | `Seckill.tsx`；`useSeckill.ts` |
| **管理端** | ConnectorAdmin：绑定、同步、probe（含单平台 price） | `/admin/connector` |
| **倒计时诚实** | 无 endTime 时不展示假倒计时 | `SeckillCountdown.tsx` |

### 3.2 Lua 返回码（禁止改动语义）

| 返回值 | 含义 | Java 侧 |
|--------|------|---------|
| `1` | 新扣成功 → Kafka 入队 | `QUEUED` |
| `0` | 无库存 | `OUT_OF_STOCK` |
| `-2` | 冷却中 | `COOLING_DOWN` |
| `-3` | 已扣过（幂等） | `QUEUED`（复用 requestId） |
| `-4` | 库存未预热 | 尝试预热后重试 / `NOT_PREHEATED` |

### 3.3 缺口（产品定位所需）

| 能力 | 结论 | 证据 |
|------|------|------|
| 预约 | **无** | 后端/主前端无 reserve 表与 API；Coupon 无活动时间窗 |
| 帮抢 | **无** | 无委托/调度任务；热路径 `userId` 仅来自 JWT |
| 比价 | **无产品能力** | 仅 probe 单 SKU `price`；无比价聚合 API/页 |
| 秒杀↔绑定联动 | **无** | Seckill 页不读 connector |
| 解绑 | **无** | 无 DELETE binding API |

### 3.4 架构关系（帮抢必须遵守）

```
[预约调度器] --到点--> POST /order/seckill (JWT=预约用户)
                         │
                         ▼
                 enter_seckill.lua  (不改返回码)
                         │ 成功(1/-3)
                         ▼
              Kafka seckill_order_create
                         │
                         ▼
                   fulfillSeckillOrder
                         │
                         ▼
              Kafka seckill_order_result → 通知预约任务状态

[Connector] ----旁路----> Redis coupon:stock:{id}   （不在 /order/seckill 请求链上）
```

---

## 4. 功能规格（可开发粒度 · P0 / P1 / P2）

### 4.1 P0 — 定位成立的最小闭环

#### P0-1 活动时间窗（coupon 或 activity）

**目标：** 让预约与倒计时有真源。

| 项 | 规格 |
|----|------|
| 模型建议 | `coupon` 表增加 `seckill_start_at` / `seckill_end_at`（TIMESTAMPTZ）；或独立 `seckill_activity` 表挂 `coupon_id`（更易一场多券，P0 可先券字段） |
| API | 管理端创建/更新秒杀券时必填时间窗；C 端列表/详情返回 |
| 前端 | `SeckillCountdown` 接真实 `endTime`；开售前展示开抢倒计时 |
| 校验 | `start < end`；开售后不可改 start（或仅运营强制改并记审计） |
| 模块 | `coupon-service` + Nacos 无强制；`frontend`；DB：`init-postgres.sql` |

#### P0-2 券 ↔ 平台 SKU 绑定对齐扩展（用户可见）

**已有：** `platform_sku_binding`（`platform`, `external_sku_id`, `coupon_id`, sync 元数据）。

| 项 | 规格 |
|----|------|
| 扩展（建议） | 可选缓存字段：`title_snapshot`, `price_snapshot`, `snapshot_at`（减少 C 端打 probe）；**不破坏** uk 约束 |
| C 端 API | `GET /api/v1/connector/bindings/by-coupon/{couponId}`（只读，可匿名或登录）；或 coupon 详情聚合返回 `binding` |
| 前端 | 秒杀卡片/详情展示「关联：JD · SKU xxx · 标题」；无绑定则展示「未绑定外部商品」 |
| 不做 | 一券多平台绑定（当前 `uk_binding_coupon` 一券一绑；多平台比价用「比价组」见 P1，不拆一券多绑） |
| 模块 | `connector-service` + 可选 `coupon-service` 聚合；`frontend` |

#### P0-3 预约抢购

**表草案：`seckill_reservation`（建议落 `order_db` 或独立，与订单同域便于帮抢）**

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | BIGSERIAL | PK |
| `user_id` | BIGINT | 预约人 = 帮抢受益人 |
| `coupon_id` | BIGINT | 秒杀券 |
| `status` | VARCHAR(32) | `PENDING` / `CANCELLED` / `TRIGGERED` / `QUEUED` / `SUCCESS` / `FAILED` / `EXPIRED` |
| `reserve_at` | TIMESTAMPTZ | 预约时间 |
| `trigger_at` | TIMESTAMPTZ | 计划触发（通常= seckill_start_at） |
| `triggered_at` | TIMESTAMPTZ | 实际触发 |
| `request_id` | VARCHAR(64) | 热路径 requestId（成功入队后） |
| `order_id` | BIGINT | 履约成功后 |
| `fail_code` | INT | 映射 ErrorCodes |
| `fail_reason` | VARCHAR(256) | 可读原因 |
| `retry_count` | INT | 调度重试次数 |
| `version` | INT | 乐观锁 |
| 唯一约束 | `(user_id, coupon_id)` WHERE status IN 活跃态 | 防重复预约 |

**API 草案：**

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/order/reservations` | body: `{ couponId }`；校验登录、type=2、未开售、库存预热策略 |
| DELETE | `/order/reservations/{id}` | 取消；仅 `PENDING` |
| GET | `/order/reservations/mine` | 我的预约列表 |
| GET | `/order/reservations/{id}` | 详情含 status / requestId |

**预约窗口语义：**

- 可预约：`now < seckill_start_at` 且活动未取消  
- 开售后：不可新预约；已有 `PENDING` 由调度器接管  
- 过期：`seckill_end_at` 仍 `PENDING` → `EXPIRED`

#### P0-4 帮抢调度（到点代发本地入队）

**定义（再次强调）：** 调度器持有用户委托，在开售窗口调用**现有**秒杀热路径，**不**改 Lua 返回码。

| 项 | 规格 |
|----|------|
| 触发 | 到达 `seckill_start_at`（允许 jitter 0–200ms 防惊群） |
| 执行身份 | **预约用户自己的 userId**（服务账号内部调用需带用户身份上下文，禁止管理员身份扣用户库存） |
| 接热路径 | 复用 `AsyncSeckillEnterService.enter` 或等价内部方法；最终仍走 Lua→Kafka→fulfill |
| 成功 | Lua `1`/`-3` → 写 `request_id`，status=`QUEUED`；监听 result → `SUCCESS`/`FAILED` |
| 失败映射 | `0`→FAILED 无库存；`-2`→可短退避重试（见下）；`-4`→预热后有限重试 |
| 重试/退避 | 仅对瞬态：预热失败、冷却（若 prod cooldown>0）、Kafka 发送超时补偿后；**上限**如 3 次，指数退避 100ms/400ms/1s；耗尽 → `FAILED` |
| 库存回补 | **不新造语义**：失败走现有 compensateRedis / fulfill 失败路径 |
| 并发与风控 | 同一 `(user,coupon)` 仅一个活跃预约；调度分片按 `coupon_id`；全局限流沿用 Sentinel；禁止为帮抢放大 Lua 并发破坏公平 |
| 通知 | 最少：结果页可查；P0 可选站内铃铛；推送/短信 P1 |
| 模块 | **新建调度**优先挂 `order-service`（贴热路径）或独立 `reservation-job`；**禁止**在 connector 里发秒杀 |

**内部调用注意：**

- 现网 `userId` 来自网关 `X-User-Id`。帮抢需：  
  - **方案 A（推荐）**：order 内 `ReservationTriggerService` 直接调 `enterSeckillAsync(userId, couponId)`，不经公网网关；或  
  - **方案 B**：内部 token + 显式 `X-User-Id`，网关白名单仅集群内。  
- **禁止**无授权切换 userId。

#### P0-5 前端预约体验

- 秒杀页：开售前主 CTA 为「预约帮抢」，开售后为「立即秒杀」  
- 我的预约：状态机展示（待开抢 / 抢购中 / 成功 / 失败原因）  
- 成功后引导「查看同品比价」（P1 入口可先灰显或占位）

---

### 4.2 P1 — 同品比价 + 体验增强

#### P1-1 秒杀后多平台同品比价

| 项 | 规格 |
|----|------|
| 数据模型 | `price_compare_group`：`id`, `local_coupon_id`（或 binding_id）, `created_at`；子表 `price_compare_item`：`platform`, `external_sku_id`, `title`, `price`, `currency`, `fetched_at`, `source`(`PROBE`/`MANUAL`), `confidence` |
| 数据源 | Connector `probeProduct`（已有 price）；运营可手工补 TB/PDD SKU 映射（即使平台 stub，可手工价） |
| 展示时点 | ① 秒杀成功页 ② 订单详情 ③ 券详情「同品参考价」；**不在**热路径阻塞 |
| 可信度 | UI 必须展示：`fetched_at`、来源、`confidence`（HIGH=官方 API / MEDIUM=手工 / LOW=过期>24h） |
| API | `GET /api/v1/connector/price-compare?couponId=`（**已落地 MVP**：绑定 + 实时 probe，暂无 `price_compare_*` 表；多平台手工映射为后续增强） |
| 不做 | 保证最低价下单；历史价曲线（P2） |
| 模块 | `connector-service` + `frontend`（券详情「同品参考价」） |

#### P1-2 体验与运维

- ~~前端改用 `GET /api/v1/order/seckill/result?requestId=`~~（已落地：`useSeckill` 按 requestId 轮询）  

- 预约成功/失败站内通知  
- Connector 解绑 API  
- 绑定与创建券向导（Admin）

---

### 4.3 P2 — 扩展（有精力再做）

- TB/PDD 正式 Connector 实现  
- 一场活动多券、预约排队优先级、公平抽签模式  
- 历史价、降价提醒  
- 多端推送（企微/邮件）  
- （C 端仅 Web `frontend/couponkill-frontend`；微信小程序已移除，非目标）

---

## 5. 技术落点

### 5.1 模块职责

| 模块 | 改什么 | 不改什么 |
|------|--------|----------|
| **coupon-service** | 活动时间窗字段；详情返回 binding 聚合（可选） | 不承担帮抢调度 |
| **connector-service** | 只读 by-coupon；比价组；可选 snapshot 字段；解绑（P1） | **不**调用 `/order/seckill` |
| **order-service** | 预约表/API；触发器调 `enterSeckillAsync`；结果回写预约状态 | **不**改 `enter_seckill.lua` 返回码；不绕过 Lua |
| **gateway** | 如需内部触发头校验；路由 `/order/reservations` | 不把 admin 身份当用户秒杀 |
| **frontend** | 预约 CTA、倒计时、绑定展示、比价页（P1） | 不伪造倒计时 |
| **nacos** | 调度开关、触发提前量、重试次数、比价缓存 TTL | — |
| **charts/scripts** | PG DDL：时间窗、预约表、比价表 | — |

### 5.2 关键 API 草案汇总

```
# 预约（order）
POST   /order/reservations
DELETE /order/reservations/{id}
GET    /order/reservations/mine
GET    /order/reservations/{id}

# 既有热路径（复用，帮抢内部调用）
POST   /order/seckill
GET    /order/seckill/result?requestId=
GET    /order/check/{couponId}

# 绑定可见 / 比价（connector）
GET    /api/v1/connector/bindings/by-coupon/{couponId}
GET    /api/v1/connector/price-compare?couponId=
GET    /api/v1/connector/probe/{platform}/product?skuId=   # 已有
```

### 5.3 与现有热路径兼容性检查清单（开发门禁）

- [ ] 帮抢最终仍执行 Lua，返回码集合不变  
- [ ] 幂等键仍为 `seckill:deduct:{userId}:{couponId}`  
- [ ] type=2 仍禁止走普通 `createOrder`  
- [ ] Kafka topic 仍为 `seckill_order_create` / `seckill_order_result`  
- [ ] Connector 同步失败不影响已入队请求的 fulfill 语义  
- [ ] 预约取消与已 TRIGGERED 的竞态有明确胜者（建议：TRIGGERED 不可取消）

### 5.4 风险与合规

| 风险 | 等级 | 缓解 |
|------|------|------|
| 文案暗示「帮抢京东」 | 高 | 产品文案审查；UI 写「帮抢本站秒杀券」 |
| 内部接口伪造 X-User-Id | 高 | 仅集群内网 + internal token；审计日志 |
| 开售惊群打垮 order | 中 | 分片调度、jitter、沿用 Sentinel；KEDA 已有扩缩能力 |
| 比价数据过期误导 | 中 | 展示 fetched_at + confidence；过期标灰 |
| 平台 ToS（JD 开放能力） | 中 | 只用合规密钥与公开/授权 API；无库存则 SKIP |
| 账号安全 | 高 | 不托管电商密码；不做跨平台登录态共享 |
| 面试演示 vs 真实 C 端期望 | 中 | README/本定位文档双处声明边界 |

### 5.5 极小缺口说明（本轮未改代码，仅指引）

若下一迭代动手，**最小可合并顺序**建议：

1. DDL：`seckill_start_at`/`seckill_end_at` + `seckill_reservation`  
2. coupon API 返回时间窗 → 前端倒计时变真  
3. reservation API + 内部 trigger → 接 `enterSeckillAsync`  
4. 秒杀详情拉 binding  
5. （P1）price-compare  

本轮**未改**业务代码与 Lua，避免在定位未对齐时污染热路径。

---

## 6. 路线图（建议节奏）

| 阶段 | 周期建议 | 交付 | 退出标准 |
|------|----------|------|----------|
| **定位冻结** | 本轮 | 本文档 | 老板确认金句与不做清单 |
| **P0 开发** | 1–2 周 | 时间窗+预约+调度帮抢+绑定可见 | 用户可预约并在开售自动入队；失败可解释 |
| **P0 验收** | 2–3 天 | 压测开售触发 + 兼容 Lua 回归 | 热路径回归用例全绿 |
| **P1** | +1 周 | 比价+requestId 轮询+解绑 | 成功页展示 ≥2 源比价与可信度 |
| **P2** | 视资源 | TB/PDD、历史价、推送 | 单独评审合规 |

---

## 7. 给老板的决策摘要

### 定位金句

> **预约后系统到点帮你抢的是「本站秒杀券」；电商绑定负责库存对齐与同品解释，比价负责决策参考——不是灰产代下单。**

### 与「直接去电商抢」对比（决策用）

| | 电商原生 | CouponKill（P0 后） |
|--|----------|---------------------|
| 抢什么 | 平台商品订单 | 本地秒杀券（可绑定展示外部 SKU） |
| 谁动手 | 你自己 | 预约后系统代发本地入队 |
| 抢完呢 | 平台履约 | 本站订单；可选同品比价后**你自己**去平台决策 |
| 合不合法 | 官方 | 合法路径；禁止脚本破解 |

### P0 清单（开发优先级）

1. 活动时间窗（coupon/activity）  
2. 预约 API + 表 + 前端 CTA  
3. 开售调度 → 复用 `enterSeckillAsync` / Lua→Kafka→fulfill  
4. 结果回写预约状态 + 可查询失败原因  
5. 秒杀详情展示 `platform_sku_binding`  

### P1 清单

1. 多平台同品只读比价（probe + 手工映射）+ 可信度  
2. 前端 `requestId` 结果轮询  
3. 绑定解绑  

### 不做清单（再强调）

跨平台代下单、验证码/风控破解、半吊子商城、改 Lua 返回码、社交灰产帮抢。

---

## 8. 附录：关键代码锚点

- 绑定 DDL：`charts/couponkill/scripts/03-init-connector.sql`  
- 绑定服务：`couponkill-connector-service/.../BindingService.java`  
- Lua：`couponkill-order-service/src/main/resources/lua/enter_seckill.lua`  
- 热路径入口：`OrderController` `POST /order/seckill`  
- 前端诚实倒计时：`frontend/couponkill-frontend/src/components/SeckillCountdown.tsx`  
- 管理端：`frontend/couponkill-frontend/src/pages/ConnectorAdmin.tsx`  
- 迁移/中间件真源：`docs/MIGRATION-PostgreSQL-Kafka.md`  

---

*文档结束。修订时请保持「帮抢 = 本地入队」边界不被营销话术稀释。*
