# CouponKill 前端项目

这是一个基于 React + TypeScript + Ant Design 的现代化前端应用，为云原生秒杀系统提供用户界面。

## 技术栈

- **React 19** - 现代前端框架
- **TypeScript 5.9** - 类型安全的JavaScript
- **Ant Design 5.x** - 企业级UI组件库
- **Redux Toolkit** - 登录态（token / 角色）
- **TanStack Query 5** - 服务端状态缓存、失效与重试（`src/lib/queryClient.ts` 集中管理 queryKey / staleTime）
- **React Router 7** - 单页面应用路由（页面级 `React.lazy` 懒加载 + 悬停预取，见 `src/lib/routePreload.ts`）
- **Axios** - HTTP客户端（`src/lib/apiClient.ts` 统一注入 Bearer、解包 `{code,message,data}`、非 2xx 转 `ApiError`、401 清会话并跳转登录）
- **Vitest + Testing Library** - 单元/组件测试（`npm test`）
- **Vite 7** - 构建工具（`manualChunks` 拆分 react / antd / icons / query vendor 包）
- **ESLint** - 代码规范

## 项目结构

```
src/
├── components/          # 公共组件
│   ├── Header.tsx      # 头部导航
│   ├── Footer.tsx      # 页脚
│   ├── CouponCard.tsx  # 优惠券卡片
│   └── ProtectedRoute.tsx # 路由守卫
├── pages/              # 页面组件
│   ├── Home.tsx        # 首页
│   ├── Login.tsx       # 登录页
│   └── ...             # 其他页面
├── services/           # API服务
│   ├── authService.ts  # 认证服务
│   ├── couponService.ts # 优惠券服务
│   ├── orderService.ts # 订单服务
│   └── userService.ts  # 用户服务
├── store/              # Redux状态管理
│   ├── index.ts        # store配置
│   └── slices/         # 状态切片
├── types/              # 类型定义
├── utils/              # 工具函数
└── assets/             # 静态资源
```

## 功能特性

### 已实现功能
- ✅ 用户认证（登录/注册）
- ✅ 首页展示（轮播图、统计数据、热门优惠券）
- ✅ 优惠券展示和详情
- ✅ 秒杀专区
- ✅ 响应式设计
- ✅ 现代UI界面

### 待实现功能
- 🔄 订单管理
- 🔄 用户个人中心
- 🔄 购物车功能
- 🔄 支付集成
- 🔄 防刷机制

## 开发指南

### 环境要求
- Node.js 16+
- npm 或 yarn

### 安装依赖
```bash
npm install
```

### 启动开发服务器
```bash
npm run dev
```

访问 http://localhost:5173 查看应用

### 构建生产版本
```bash
npm run build
```

### 代码检查与测试
```bash
npm run lint   # ESLint
npm test       # Vitest（apiClient 错误映射、秒杀轮询、服务层请求契约等）
```

## API配置

所有请求都走**网关**（`:8088`）的 `/api/v1/**`，service 层已写全路径，`VITE_API_BASE_URL` 只决定「发往哪个源」。
前端不直连 user/coupon/order/connector 各服务（下游只信任带网关签名的身份头），`/internal/**` 网关一律 404：


| 场景 | `VITE_API_BASE_URL` | 说明 |
|---|---|---|
| 本地开发 `npm run dev` | 留空 | Vite proxy 把 `/api` 转到 `VITE_PROXY_TARGET`（默认网关 `http://localhost:8088`） |
| 本地预览 `npm run preview` | 留空 | 复用同一份 proxy 配置 |
| 生产，同域部署（推荐） | 留空 | 静态资源与网关同域，Ingress/Nginx 把 `/api/**` 转发到网关，无 CORS |
| 生产，跨域部署 | `https://gw.example.com` | 只填源（协议+主机+端口），**不要**带 `/api/v1`；误带会被自动去除并在控制台告警 |

开发代理在 `vite.config.ts`：只代理 `/api`，目标取 `VITE_PROXY_TARGET`（默认 `http://localhost:8088`，即本地网关）。
示例见 `.env.example`；构建时注入的值会被打进产物，修改后需重新 `npm run build`。

## 项目亮点

1. **现代化技术栈** - 使用最新的React生态技术
2. **类型安全** - 全量TypeScript，提供更好的开发体验
3. **企业级UI** - Ant Design提供专业的界面组
4. **状态管理** - Redux Toolkit简化状态管理逻辑
5. **响应式设计** - 支持桌面端和移动端
6. **性能优化** - Vite提供极快的热更新和构建速度

## 与后端集成

前端对接 Go 重写后的后端（契约见 `docs/GO-REWRITE-DESIGN.md` §4/§10）：

- 用户服务：`/api/v1/user/**`（登录、注册、登出、profile）；`GET /api/v1/user/coupon/count` 由网关路由到 order 服务
- 优惠券服务：`/api/v1/coupon/**`（列表、详情；管理写接口需 admin 角色）
- 订单服务：`/api/v1/order/**`（领取、秒杀、取消、预约、站内通知）
- Connector：`/api/v1/connector/**`（外部 SKU 绑定、同品比价）

### 数据约定

- **ID 一律是字符串**（雪花 ID 超过 2^53 会在 JS 数字里丢精度），前端全程按 `string` 处理，不做 `Number()` 转换。
- **时间**是 ISO-8601（RFC3339，带时区）字符串，字段名 `createdAt/updatedAt/getTime/expireTime/cancelTime/seckillStartAt/seckillEndAt…`；
  展示统一走 `formatBackendDateTime`，提交给后端的时间用 `toISOString()`。
- **金额**（`faceValue/minSpend/price`）是两位小数的 JSON 数字（兼容字符串），展示统一用 `formatMoney`（固定两位小数）。
- **写接口使用 JSON 请求体**（`{couponId}`、`{orderId}` 等），不再用 query 参数；列表接口兼容裸数组与分页信封 `{items,total,page,size}`。
- 角色取自登录响应的 `roles`（缺省时读 JWT 载荷里的 `roles`），管理台路由要求 `admin`。

### 错误处理（`src/lib/apiClient.ts`、`errorMessage.ts`、`notifyError.tsx`）

后端用真实 HTTP 状态码，错误体是信封 `{code,message,data:null,requestId}`。非 2xx 统一转成 `ApiError`
（`code` 业务码、`status` HTTP 状态、`kind` 分类、`requestId`），界面文案优先用服务端 `message`：

- **401**（20001/20002 等，非口令错误）：清本地会话并跳转 `/login`，整个会话只跳一次；用旧令牌发出的迟到 401 不会踢掉新会话；
  登录页会给一次性「登录已过期」提示。登录口令错误（401 + 20005）只是普通错误提示。
- **403** 无权限、**409** 业务拒绝（售罄 10002 / 重复 10010 / 未开始 10006 / 已结束 10007 / 券停用 10014）、
  **429** 冷却/限流（10001 / 10003）、**503** 系统繁忙（50003）：用 warning 展示，不当作故障；429 的 `Retry-After` 会被秒杀轮询采纳。
- **其它 5xx / 无法解析的响应**：error 提示并带「复制请求ID」按钮，控制台同时输出请求号，方便向后端报障。
- 统一入口 `notifyError(message, err, fallback)`；会话过期不弹窗（已在跳转）。

### 秒杀流程（`src/hooks/useSeckill.ts`、`src/lib/seckill.ts`）

`POST /api/v1/order/seckill {couponId}` 受理后返回 **202** `{requestId, status:"QUEUED"}`，
前端按 `requestId` 有界退避轮询 `GET /api/v1/order/seckill/result?requestId=`（200ms 起指数退避、上限 2s，最多 10 次）。
响应是 JSON 对象 `{status, orderId?, reason?}`，`status` 为 `QUEUED | PROCESSING | SUCCESS | FAILED`：

- `SUCCESS`：带 `orderId`，可直达订单详情；`FAILED`：按 `reason` 映射中文
  （`OUT_OF_STOCK / REPEAT / TIMEOUT / COUPON_DISABLED / COUPON_NOT_FOUND / LIMIT_EXCEEDED / ENQUEUE_FAILED / SYSTEM_ERROR`，见 `SECKILL_REASON_MESSAGES`）；
- 暂时性错误（网络、超时、429、503、5xx）按进行中继续重试；401/403/404/400 立即停止；超过上限提示去「我的订单」确认；
- 组件卸载或再次发起时中止轮询（含在途请求与退避定时器）。

请确保网关与后端服务已正确部署并可访问。

## 贡献指南

欢迎提交Issue和Pull Request！请遵循以下步骤：

1. Fork项目
2. 创建功能分支：`git checkout -b feature/amazing-feature`
3. 提交更改：`git commit -m 'Add amazing feature'`
4. 推送分支：`git push origin feature/amazing-feature`
5. 提交Pull Request

## 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。
