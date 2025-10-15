# CouponKill 前端项目

这是一个基于 React + TypeScript + Ant Design 的现代化前端应用，为云原生秒杀系统提供用户界面。

## 技术栈

- **React 18** - 现代前端框架
- **TypeScript** - 类型安全的JavaScript
- **Ant Design 5.x** - 企业级UI组件库
- **Redux Toolkit** - 状态管理
- **React Router 6** - 单页面应用路由
- **Axios** - HTTP客户端
- **Vite** - 快速构建工具
- **ESLint + Prettier** - 代码规范

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

### 代码检查
```bash
npm run lint
```

## API配置

前端应用需要连接后端API服务。请在环境变量或配置文件中设置：

```typescript
// .env 文件
VITE_API_BASE_URL=https://your-backend-api.com
```

## 项目亮点

1. **现代化技术栈** - 使用最新的React生态技术
2. **类型安全** - 全量TypeScript，提供更好的开发体验
3. **企业级UI** - Ant Design提供专业的界面组
4. **状态管理** - Redux Toolkit简化状态管理逻辑
5. **响应式设计** - 支持桌面端和移动端
6. **性能优化** - Vite提供极快的热更新和构建速度

## 与后端集成

前端应用已配置好与云原生秒杀系统的后端服务集成：

- 用户服务：`/api/v1/auth/*`
- 优惠券服务：`/api/v1/coupons/*`
- 订单服务：`/api/v1/orders/*`
- 用户中心：`/api/v1/user/*`

请确保后端服务已正确部署并可访问。

## 贡献指南

欢迎提交Issue和Pull Request！请遵循以下步骤：

1. Fork项目
2. 创建功能分支：`git checkout -b feature/amazing-feature`
3. 提交更改：`git commit -m 'Add amazing feature'`
4. 推送分支：`git push origin feature/amazing-feature`
5. 提交Pull Request

## 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。
