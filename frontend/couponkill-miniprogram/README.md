# 云原生秒杀系统微信小程序

基于云原生技术的秒杀系统微信小程序版本，提供完整的优惠券购买、秒杀、订单管理等功能。

## 📋 功能特性

### 🏠 首页
- 轮播图展示热门活动
- 平台统计数据展示
- 热门优惠券推荐
- 秒杀专区快速入口

### 🎫 优惠券中心
- 优惠券列表展示（支持分页）
- 多维度筛选（类型、状态）
- 搜索功能
- 优惠券详情查看

### ⚡ 秒杀专区
- 实时秒杀活动展示
- 倒计时显示
- 秒杀价格展示
- 一键秒杀功能

### 📋 订单管理
- 订单列表展示
- 订单状态追踪
- 订单详情查看

### 👤 用户中心
- 用户信息展示
- 我的优惠券管理
- 订单历史记录

## 🚀 技术栈

- **微信小程序原生框架**
- **TypeScript**（可选，后续扩展）
- **SCSS** 样式预处理器
- **云原生后端API**

## 📁 项目结构

```
couponkill-miniprogram/
├── app.js                 # 小程序入口文件
├── app.json               # 小程序配置
├── app.wxss               # 小程序全局样式
├── project.config.json    # 项目配置文件
├── package.json           # 项目信息
├── utils/                 # 工具库
│   └── util.js           # 通用工具函数
├── pages/                 # 页面文件
│   ├── index/            # 首页
│   │   ├── index.js
│   │   ├── index.wxml
│   │   └── index.wxss
│   ├── coupons/          # 优惠券列表
│   │   ├── index.js
│   │   ├── index.wxml
│   │   └── index.wxss
│   ├── seckill/          # 秒杀专区
│   │   ├── index.js
│   │   ├── index.wxml
│   │   └── index.wxss
│   ├── orders/           # 订单列表
│   ├── order-detail/     # 订单详情
│   ├── user/             # 用户中心
│   ├── login/            # 登录页
│   └── register/         # 注册页
└── assets/               # 静态资源
    └── icons/           # 图标文件
```

## 🔧 开发指南

### 环境要求

- 微信开发者工具 1.06.2307260 或以上版本
- 微信小程序开发者账号

### 快速开始

1. **克隆项目**
   ```bash
   git clone <repository-url>
   cd couponkill-miniprogram
   ```

2. **打开微信开发者工具**
   - 选择"导入项目"
   - 选择项目根目录
   - 填写你的 AppID（可在微信公众平台申请）

3. **配置后端接口**
   编辑 `app.js` 中的 `globalData.apiBase`：
   ```javascript
   globalData: {
     apiBase: 'https://your-api-domain.com/api/v1'
   }
   ```

4. **编译运行**
   - 点击微信开发者工具的"编译"按钮
   - 使用"真机调试"或"模拟器"查看效果

### 开发规范

#### 文件命名规范
- 页面文件名：小写英文，多个单词用连字符连接（如：coupon-detail）
- 组件文件名：首字母大写驼峰命名（如：CouponCard）
- 工具文件名：小写英文（如：util.js）

#### 代码规范
- 使用 ES6+ 语法
- 变量命名采用驼峰式
- 常量使用大写下划线命名
- 注释要清晰明了

#### 样式规范
- 使用 rpx 作为单位
- 公共样式放在 app.wxss
- 页面样式放在对应目录的 wxss 文件

## 📱 页面说明

### 首页 (pages/index)
- 展示轮播图、统计数据、热门优惠券、秒杀活动
- 支持下拉刷新
- 点击优惠券跳转到详情页

### 优惠券列表 (pages/coupons)
- 支持搜索和筛选
- 分页加载
- 展示优惠券详细信息

### 秒杀专区 (pages/seckill)
- 实时倒计时显示
- 秒杀状态判断
- 一键秒杀功能

### 订单管理 (pages/orders)
- 订单状态筛选
- 订单详情查看
- 支持取消订单

### 用户中心 (pages/user)
- 个人信息展示
- 我的优惠券管理
- 订单历史记录

## 🔗 API 接口

### 基础配置
- 基础URL：`https://your-api-domain.com/api/v1`
- 请求头：`Authorization: Bearer {token}`

### 主要接口
- `GET /banners` - 获取轮播图
- `GET /statistics` - 获取统计数据
- `GET /coupons` - 获取优惠券列表
- `GET /coupons/hot` - 获取热门优惠券
- `GET /coupons/seckill` - 获取秒杀优惠券
- `GET /coupons/{id}` - 获取优惠券详情
- `POST /orders` - 创建订单
- `POST /orders/seckill` - 秒杀下单
- `GET /orders` - 获取订单列表
- `GET /orders/{id}` - 获取订单详情
- `POST /auth/wechat-login` - 微信登录

## 🔐 登录授权

小程序使用微信授权登录，无需单独注册账号：

1. 用户点击登录按钮
2. 调用微信登录接口获取 code
3. 后端通过 code 获取用户信息
4. 返回 token 和用户信息
5. 本地存储 token，进行后续请求

## 💰 支付功能

集成微信支付：

```javascript
// 发起支付
wx.requestPayment({
  timeStamp: '',
  nonceStr: '',
  package: '',
  signType: 'MD5',
  paySign: '',
  success: (res) => {
    console.log('支付成功', res)
  },
  fail: (err) => {
    console.log('支付失败', err)
  }
})
```

## 🚀 部署上线

1. **申请小程序账号**
   - 注册微信小程序账号
   - 完善小程序信息

2. **代码审核**
   - 提交代码审核
   - 等待微信团队审核

3. **发布上线**
   - 审核通过后发布
   - 设置版本号和描述

## 📊 性能优化

- **图片懒加载**：使用图片懒加载组件
- **列表虚拟化**：大量数据时使用虚拟列表
- **缓存策略**：合理使用本地存储缓存
- **网络优化**：请求拦截、错误重试

## 🛠️ 常见问题

### 1. 真机调试白屏
- 检查 AppID 是否正确配置
- 检查网络请求域名是否在合法域名列表中

### 2. 图片不显示
- 检查图片地址是否正确
- 确认图片已上传到合法域名

### 3. 支付失败
- 检查支付签名是否正确
- 确认支付金额格式正确

## 📞 联系我们

如有问题或建议，请联系开发团队。

---

**享受秒杀乐趣，发现更多优惠！** 🎉
