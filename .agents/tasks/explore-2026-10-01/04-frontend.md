# 04 · frontend 分片报告（admin UI / miniprogram）

> 范围：已提交的 `frontend/couponkill-frontend/`（Vite + React，含 C 端和管理台），以及工作区里未跟踪的 `frontend/couponkill-miniprogram/`。
> 方式：只读。未跑 `npm`、未开 Vite、未开微信开发者工具。基线：`main` @ `3969213`。`git status` 显示 `frontend/couponkill-miniprogram/` 为 untracked；`git ls-tree HEAD` 在该路径下是 0 个文件。
> 标注：【已核实】= 读源码或 git 输出；【推断】= 未在浏览器或开发者工具里跑。
> 后端热路径缺陷（停用券仍可下单、秒杀单被写成 status=2）以 02 为准，这里只写前端怎么展示、怎么调用。

## 0. TL;DR

- 这块是什么：HEAD 里只有一套 Web 应用 `frontend/couponkill-frontend`。它同时是 C 端（登录、券列表、秒杀、订单、预约、通知）和管理台（`/admin/coupons`、`/admin/connector`）。小程序目录是 `afddf63`（2026-07-18）从 git 删掉之后，工作区里又出现的未跟踪树，文件不齐，不能当一个微信工程打开。
- 整体健康度：已提交的 Web 端和当前网关/订单契约是对齐的（`/api/v1/**`、`code=0`、`@RequestParam`、秒杀用 `requestId` 轮询）。管理台的按钮能打到网关的 admin 路径。小程序是半截页面加一份 API 封装，和 Web 端不是同一完成度。仓库没有前端 CI，也没有前端镜像。
- 最重要的 5 个发现：
  1. 【高·完成度】`frontend/couponkill-miniprogram/` 不在 HEAD（删除提交 `afddf63`，`git ls-tree` 计数为 0），当前 23 个文件全部 untracked。没有 `app.js`、`app.json`、`project.config.json`。8 个页面里只有 `pages/coupons` 和 `pages/seckill` 有 `.js`。微信开发者工具缺少入口配置文件，工程打不开【推断：平台要求根上有 `app.json`】。
  2. 【高·安全】Web 登录把口令放进 query：`authService.login` 使用 `http.post(url, null, { params })`（`src/services/authService.ts:7-9`），对应 user-service 的 `@RequestParam`（01 报告）。管理台是否显示，只看 localStorage 里的 `ck_roles`（`authSlice.ts:137-138`，`ProtectedRoute.tsx:31`）。改本地角色就能看到管理页面；真正的写接口仍要网关认的 admin JWT。mock token 能签发这种 JWT 的事实在 07a，不重复。
  3. 【高·交付】`.env.production` 的 `VITE_API_BASE_URL` 为空。生产构建会请求页面自己的源，而不是一个写死的网关地址（`src/lib/apiClient.ts:28`）。`.github/workflows/ci.yml` 没有 npm/vite；五个 Java Dockerfile 加 Go、operator 共七个镜像，没有前端镜像（镜像清单见 05）。README 写 React 18 和 React Router 6（`frontend/couponkill-frontend/README.md:7-11`），`package.json` 是 React 19.1.1 和 react-router-dom 7.9.4。
  4. 【中·契约】Web 秒杀走 `POST /api/v1/order/seckill` 拿 `requestId`，再轮询 `/seckill/result`（`src/hooks/useSeckill.ts:32-51`）。未跟踪小程序的 `seckillUntilReceived` 只调 `seckill` 然后轮询 `checkReceived`（`utils/api.js:254-261`），没有 `requestId`。小程序把订单 status `2` 显示成「已使用」（`utils/api.js:148-149`）。02 已核实秒杀成功后订单会被写成 2，因此这条文案会把刚抢到的券显示成已使用。
  5. 【中·管理台】券管理四个写操作（create、seckill-window、status、delete）用 query 参数，和 `CouponController` 的 `@RequestParam` 以及网关 `isCouponAdminPath` 一致【已核实，见第 4 节】。管理台上的「停用」调用的是 status 接口。status=0 挡不住下单是 02 的第 4 点；管理台会让操作者以为开关已经生效。

## 1. 范围与技术栈

### 1.1 跟踪状态

| 路径 | git | 说明 |
|---|---|---|
| `frontend/couponkill-frontend/**` | 已提交，HEAD 上 12 个触及该目录的提交，最新 `3969213` | 含 `CouponAdmin.tsx`、`ConnectorAdmin.tsx`、`ProtectedRoute` 的 `requireAdmin` |
| `frontend/couponkill-miniprogram/**` | HEAD 中 0 文件；工作区 untracked | `afddf63` 的提交说明是 remove miniprogram。删除已用 `git log --diff-filter=D` 核对到该路径 |

### 1.2 Web 依赖（`frontend/couponkill-frontend/package.json`）

| 项 | package.json | README 写法 |
|---|---|---|
| React / React DOM | `^19.1.1` | React 18 |
| react-router-dom | `^7.9.4` | React Router 6 |
| antd | `^5.27.5` | Ant Design 5（一致） |
| Vite | `^7.1.7` | Vite（无版本） |
| 状态 | Redux Toolkit `^2.9.0`、react-redux `^9.2.0`、TanStack Query `^5.101.2` | README 只写了 Redux Toolkit |
| HTTP | axios `^1.12.2` | axios |
| TypeScript | `~5.9.3` | 有 |

脚本：`dev` = vite，`build` = `tsc -b && vite build`，`lint` = eslint。没有 test 脚本【已核实】。

开发代理：`VITE_PROXY_TARGET` 默认 `http://localhost:8088`，把 `/api` 和 `/order` 转到该地址（`vite.config.ts:7-21`）。`.env.development` 把 `VITE_API_BASE_URL` 留空、代理目标写成 `http://localhost:8088`，与 07b 里网关本地端口 8088 一致。

### 1.3 小程序

无 `package.json`。运行时是微信 `wx.request`。默认基址 `http://127.0.0.1:8088`（`utils/api.js:25-28`）。注释写明开发者工具要勾选不校验合法域名。

## 2. 结构地图

```
frontend/couponkill-frontend/          已提交
  vite.config.ts                       /api、/order -> :8088
  .env.development / .env.production / .env.example
  src/main.tsx, App.tsx                路由见下
  src/lib/apiClient.ts                 Bearer、解包 code 0 或 200、401 清本地会话
  src/lib/routePreload.ts              路由级懒加载
  src/store/slices/authSlice.ts        ck_token / ck_userId / ck_username / ck_roles
  src/services/                        auth, user, coupon, order, reservation, notification, connector
  src/pages/                           C 端 + CouponAdmin + ConnectorAdmin
  src/components/ProtectedRoute.tsx    requireAdmin 只看 Redux roles
  src/components/Header.tsx            管理入口、通知铃铛

frontend/couponkill-miniprogram/       未跟踪，23 个文件
  utils/api.js                         唯一完整的逻辑模块
  pages/coupons/                       js+wxml+wxss+json
  pages/seckill/                       js+wxml+wxss+json（无下单按钮逻辑，见 3.3）
  pages/index, orders, user            有 wxml，无 js
  pages/login                          只有 json+wxss
  pages/register, coupon-detail, order-detail
                                       只有 json
  （无 app.js / app.json / sitemap / project.config.json）
```

Web 路由（`src/App.tsx:63-116`）：

| 路径 | 守卫 | 页面 |
|---|---|---|
| `/login` `/register` | 无 | 登录、注册 |
| `/` `/coupons` `/coupons/:id` `/seckill` | 无 | 首页、列表、详情、秒杀 |
| `/reservations` `/orders` `/orders/:id` `/user/*` | 要登录 | 预约、订单、个人中心 |
| `/admin/connector` `/admin/coupons` | 要登录且 roles 含 admin | Connector 管理、券管理 |

`Login.tsx:68` 把种子数据里的演示账号预填进表单。种子脚本书面写了同一账号（`charts/couponkill/scripts/02-seed-demo.sql:8`）。口令不在本报告重复。

## 3. 核心流程

### 3.1 Web 登录和管理台门禁

```
Login 表单
  -> POST /api/v1/user/login?username&password     axios params，口令在 query
  -> 解包 code 0/200
  -> localStorage: ck_token, ck_userId, ck_username, ck_roles(JSON)
  -> Authorization: Bearer
401
  -> 清四件套并跳 /login（apiClient.ts:46-53）
logout
  -> 只清本地（authService.ts:24-28），没有服务端作废（01：Redis 键只写不读）

ProtectedRoute requireAdmin
  -> roles 来自登录响应写入的 localStorage
  -> 不是 admin：页面上 403 文案，不发请求
```

管理台因此是展示层门禁。接口层门禁在网关 JWT admin（01 第 3.2 节）。两边要同时为真，写操作才成功。

### 3.2 Web 秒杀与券管理

秒杀（`useSeckill.ts`）：`orderService.seckill` -> 有 `requestId` 就轮询 `seckillResult`，把 `SUCCESS*` 当成功、`FAIL*` 当失败、其余当进行中，最多约 10 次，间隔 200ms 起、封顶 2s。没有 `requestId` 时退回 `checkReceived`。这和订单控制器的 query 契约一致：`POST /seckill` 的 `couponId`、`GET /seckill/result` 的 `requestId` 都是 `@RequestParam`（`OrderController.java:113-120,166-168`）。

券管理（`CouponAdmin.tsx` + `couponService.ts`）：列表用 `GET /api/v1/coupon/list`；创建、改时间窗、改状态用 POST query；删除用 `DELETE /api/v1/coupon/{id}`。时间格式 `YYYY-MM-DD HH:mm:ss`，与 `CouponController` 的 `@DateTimeFormat` 相同（`CouponController.java:53-56`）。创建接口在服务端把 status 写成 1（`CouponController.java:91`），管理台不能在创建时指定停用。

Connector 管理页走 `/api/v1/connector/**` 的绑定、同步、探针、比价映射（`connectorService.ts`）。比价和按券查绑定在网关白名单里（07a），管理写不在。本分片没有再对每一条 Connector 路径做权限矩阵。

通知铃铛每 30s 调 `/api/v1/order/notifications/unread-count`（`Header.tsx:48-53`）。

### 3.3 小程序（未跟踪）

`utils/api.js` 把登录/注册/下单/秒杀/取消做成 `application/x-www-form-urlencoded` 的 body（`postForm`，`:126-132`）。这和 `@RequestParam` 兼容，也避免了 Web 那种 query 口令【已核实：wx.request 的 data + 该 content-type】。成功码同样接受 0 和 200。

`seckillUntilReceived` 不读 `requestId`。`pages/seckill/index.js` 只拉 type=2 的券并 `navigateTo` 详情页；详情页没有 js。列表页同样跳到没有脚本的详情页。没有 `app.json` 的 `pages` 数组，这些 Page 注册不会被加载【推断】。

## 4. 对外契约

Web 与小程序封装相对网关的调用（只列本分片读过的封装，不重复 02 的服务端语义）：

| 客户端方法 | HTTP | 与控制器 |
|---|---|---|
| `authService.login/register` | POST query | `UserController` `@RequestParam`【已核实】 |
| `authService.getProfile` | GET `/api/v1/user/profile` | 身份来自网关头【已核实】 |
| `couponService.getAvailableCoupons/getAllCoupons/getCouponById` | GET | 读接口，网关不要求 admin |
| `couponService.create/updateSeckillWindow/updateCouponStatus/delete` | POST query 或 DELETE | 与 `CouponController.java:41-130` 及网关 admin 路径一致 |
| `orderService.create/seckill/cancel` | POST query | `OrderController` `@RequestParam` |
| `orderService.seckillResult` | GET query `requestId` | Web 在用；小程序封装里没有这个方法 |
| `orderService.checkReceived` | GET `/api/v1/order/check/{couponId}` | 小程序把它当秒杀完成条件 |
| `reservationService` | `/api/v1/order/reservations` | 仅 Web |
| `notificationService` | `/api/v1/order/notifications` | 仅 Web |
| `connectorService` | `/api/v1/connector/**` | 仅 Web 管理台和详情比价 |

`apiClient` 把 HTTP 200 且 body.code 为 0 或 200 当成功（`apiClient.ts:24,62-68`）。user-service 的业务失败也是 HTTP 200 + 非 0 code（01 的 `GlobalExceptionHandler`），前端会抛 `ApiError`。网关 401 走错误拦截器。网关 403 空 body（管理写被拒）时，前端只能拿到 HTTP 状态，message 退回 axios 的默认文案【推断：`apiClient.ts:55-57` 在没有 `data.message` 时用 `error.message`】。

ID 在 Web 类型里一律 string（`src/types/api.ts:1-3`）。`User.id` 的后端 `@JsonFormat(STRING)` 在 01。秒杀 couponId 仍通过 query 传，Spring 把数字字符串转成 Long。超过 `Number.MAX_SAFE_INTEGER` 的 id 只要全程保持字符串就安全；若某次被 `Number()` 吃掉就会偏【推断】。小程序 `api.js` 对列表结果做了 `String(id)`，但 `pages/seckill` 没有下单，这条路径目前走不到。

## 5. 依赖关系

```
开发：浏览器 -> Vite :5173 -> proxy /api,/order -> gateway :8088
生产构建：静态文件的源 + VITE_API_BASE_URL（当前为空 -> 相对路径）
未跟踪小程序：wx.request -> http://127.0.0.1:8088

gateway JWT
  -> user / coupon / order / connector
管理写额外要求 roles 含 admin，或 X-Admin-Token（01）
```

前端不参加 `.github/workflows/ci.yml`（检索无 frontend/npm/vite）【已核实】。Chart 和 Jenkins 的镜像列表里没有这套静态资源（05 的服务表）。生产环境变量空着，意味着要么把构建产物挂到网关同源，要么在构建时另行注入 `VITE_API_BASE_URL`。仓库里没有第二种做法的脚本【已核实：`.env.production` 为空，无 Dockerfile】。

`useSeckill` 依赖订单服务返回 `requestId` 和 `/seckill/result` 的字符串协议（`SUCCESS` / `FAIL` / `PENDING`）。该协议的服务端实现属于 02，本分片只确认客户端按这个形状解析（`useSeckill.ts:20-24`）。

## 6. 问题与风险

| 级别 | 类型 | 结论 | 证据 |
|---|---|---|---|
| 高 | 完成度 | 小程序未跟踪且缺少小程序入口文件，多数页面没有脚本 | 第 1.1 节 git；目录 23 文件清单 |
| 高 | 安全 | Web 口令在 query。管理页面角色可被本地存储伪造，写操作仍会被网关拒绝 | `authService.ts:7-9`；`ProtectedRoute.tsx:31-37`；01 的 admin 门禁 |
| 高 | 交付 | 生产 API 基址为空；无 CI、无镜像；README 版本与 package.json 不符 | `.env.production`；`package.json`；`README.md:7-11`；ci.yml 无匹配 |
| 中 | 契约 | 小程序秒杀完成条件是 `checkReceived`，不是 Web 已使用的 `requestId` 结果 | `utils/api.js:254-261` 对比 `useSeckill.ts:32-51` |
| 中 | 展示 | 小程序把 status 2 显示为已使用。与 02 第 2 点（秒杀结果监听把订单写成 2）叠在一起，成功单会显示成已使用 | `utils/api.js:137-149`；02 的 `SeckillOrderResultListener` |
| 中 | 产品 | 券管理「停用」只打 status 接口。02 已核实 status=0 挡不住下单和秒杀 | `couponService.ts:61-63`；02 第 4 点 |
| 低 | 体验 | 登录页预填演示账号。这是本地种子，不是额外的密钥渠道 | `Login.tsx:68`；`02-seed-demo.sql:8` |
| 低 | 死页面 | 小程序 `goDetail` 指向没有 js/wxml 的 `pages/coupon-detail` | `pages/seckill/index.js:30-33`；该目录只有 `index.json` |

Web 端订单状态展示没有在本分片逐行对照 `OrderStatus` 文案。类型常量把 2 标成「已使用」（`src/types/api.ts:27-33`），和后端枚举注释一致。若 Web 列表按这个常量渲染，会有和小程序相同的「刚抢到即已使用」问题【推断：未逐页读 OrderList 的列渲染】。根因仍是 02，不在前端单独开修复项。

## 7. 最近活跃度

已提交前端 12 个提交，最近一次就是基线 `3969213`（2026-07-18，`feat(admin): ... add admin UI`）。同一天还有预约通知、比价 UI、`seckill/result` 轮询（`e7ee0a2`）。

小程序曾被 `3209388`（2026-07-18，`align miniprogram API`）改过，随后被 `afddf63` 从树里删除。现在工作区里的 23 个文件没有提交。

## 8. 未验证 / 不确定项

- 没有 `npm install` / `npm run build` / 浏览器点击。路由、懒加载、antd 管理表单是否能提交成功，都是读源码。
- 没有打开微信开发者工具。缺少 `app.json` 则无法作为小程序编译，这是平台规则推断。
- 403 空 body 时 UI 文案、雪花 id 在 query 里的精度，未用真实响应验证。
- `.env.production` 为空是文件事实。实际发布时是否有人在 CI 之外注入环境变量，仓库里看不到。

## 9. 候选 OpenSpec capability（只写现状，不建 spec）

- `web-storefront`：已提交。登录、券、秒杀轮询、订单、预约、通知与当前 `/api/v1` 契约对齐；口令在 query。
- `admin-console`：已提交。券与 Connector 管理页在前端按角色隐藏，写权限在网关。券状态开关不代表 02 里的下单拦截。
- `miniprogram`：不在 HEAD。工作区副本没有工程入口，秒杀完成条件与 Web 不一致。
