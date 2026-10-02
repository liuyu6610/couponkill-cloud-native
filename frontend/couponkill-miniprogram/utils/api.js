/**
 * 现网契约 API（对齐 Gateway + Web 前端）
 * - 成功码：ApiResponse code=0（过渡兼容 200）
 * - 路径：/api/v1/user|coupon|order/**
 * - ID：后端 Long 以字符串序列化，本地一律 String()
 */

const SUCCESS_CODES = { 0: true, 200: true }

const STORAGE = {
  token: 'ck_token',
  userId: 'ck_userId',
  username: 'ck_username',
  roles: 'ck_roles',
}

function getAppSafe() {
  try {
    return getApp()
  } catch (e) {
    return null
  }
}

function apiBase() {
  const app = getAppSafe()
  // 默认直连本地网关；开发者工具需勾选「不校验合法域名」
  return (app && app.globalData && app.globalData.apiBase) || 'http://127.0.0.1:8088'
}

function getToken() {
  return wx.getStorageSync(STORAGE.token) || ''
}

function clearAuth() {
  wx.removeStorageSync(STORAGE.token)
  wx.removeStorageSync(STORAGE.userId)
  wx.removeStorageSync(STORAGE.username)
  wx.removeStorageSync(STORAGE.roles)
  const app = getAppSafe()
  if (app) {
    app.globalData.userInfo = null
  }
}

function saveLogin(result) {
  const userId = String(result.userId)
  const roles = (result.roles || []).map(String)
  wx.setStorageSync(STORAGE.token, result.token)
  wx.setStorageSync(STORAGE.userId, userId)
  wx.setStorageSync(STORAGE.username, String(result.username || ''))
  wx.setStorageSync(STORAGE.roles, JSON.stringify(roles))
  const app = getAppSafe()
  if (app) {
    app.globalData.userInfo = {
      id: userId,
      username: String(result.username || ''),
      roles,
    }
  }
  return { ...result, userId, roles }
}

function request(path, options = {}) {
  const method = (options.method || 'GET').toUpperCase()
  const token = getToken()
  const header = Object.assign(
    {
      'Content-Type': options.contentType || 'application/json',
    },
    options.header || {}
  )
  if (token) {
    header.Authorization = 'Bearer ' + token
  }

  let url = apiBase() + path
  let data = options.data

  // Spring @RequestParam：GET 用 query；POST form 也走 data + x-www-form-urlencoded
  if (options.query && typeof options.query === 'object') {
    const qs = Object.keys(options.query)
      .filter((k) => options.query[k] !== undefined && options.query[k] !== null)
      .map((k) => encodeURIComponent(k) + '=' + encodeURIComponent(String(options.query[k])))
      .join('&')
    if (qs) {
      url += (url.indexOf('?') >= 0 ? '&' : '?') + qs
    }
  }

  return new Promise((resolve, reject) => {
    wx.request({
      url,
      method,
      data: data === undefined ? undefined : data,
      header,
      timeout: options.timeout || 15000,
      success(res) {
        if (res.statusCode === 401) {
          clearAuth()
          reject(new Error('未登录或登录已过期'))
          return
        }
        if (res.statusCode !== 200) {
          reject(new Error('HTTP ' + res.statusCode))
          return
        }
        const body = res.data
        if (body && typeof body === 'object' && 'code' in body) {
          if (SUCCESS_CODES[body.code]) {
            resolve(body.data)
            return
          }
          reject(new Error(body.message || '请求失败'))
          return
        }
        resolve(body)
      },
      fail(err) {
        reject(err || new Error('网络异常'))
      },
    })
  })
}

/** application/x-www-form-urlencoded POST（登录/注册/下单） */
function postForm(path, form) {
  return request(path, {
    method: 'POST',
    contentType: 'application/x-www-form-urlencoded',
    data: form,
  })
}

const CouponType = { NORMAL: 1, SECKILL: 2 }

const OrderStatus = {
  CREATED: 1,
  USED: 2,
  EXPIRED: 3,
  CANCELLED: 4,
}

function orderStatusText(status) {
  switch (Number(status)) {
    case OrderStatus.CREATED:
      return '已创建'
    case OrderStatus.USED:
      return '已使用'
    case OrderStatus.EXPIRED:
      return '已过期'
    case OrderStatus.CANCELLED:
      return '已取消'
    default:
      return '未知'
  }
}

const api = {
  STORAGE,
  CouponType,
  OrderStatus,
  orderStatusText,
  clearAuth,
  getToken,
  isLoggedIn() {
    return !!getToken()
  },

  login(username, password) {
    return postForm('/api/v1/user/login', { username, password }).then(saveLogin)
  },

  register(username, password, phone) {
    return postForm('/api/v1/user/register', { username, password, phone })
  },

  getProfile() {
    return request('/api/v1/user/profile').then((u) => {
      if (!u) return u
      const normalized = {
        ...u,
        id: String(u.id),
      }
      const app = getAppSafe()
      if (app) {
        app.globalData.userInfo = {
          id: normalized.id,
          username: normalized.username,
          roles: JSON.parse(wx.getStorageSync(STORAGE.roles) || '[]'),
        }
      }
      return normalized
    })
  },

  listAvailableCoupons() {
    return request('/api/v1/coupon/available').then((list) =>
      (list || []).map((c) => ({ ...c, id: String(c.id) }))
    )
  },

  listAllCoupons() {
    return request('/api/v1/coupon/list').then((list) =>
      (list || []).map((c) => ({ ...c, id: String(c.id) }))
    )
  },

  getCoupon(id) {
    return request('/api/v1/coupon/' + encodeURIComponent(String(id))).then((c) =>
      c ? { ...c, id: String(c.id) } : c
    )
  },

  listMyOrders(pageNum = 1, pageSize = 10) {
    return request('/api/v1/order/user/me', {
      query: { pageNum, pageSize },
    }).then((list) =>
      (list || []).map((o) => ({
        ...o,
        id: String(o.id),
        userId: String(o.userId),
        couponId: String(o.couponId),
      }))
    )
  },

  createOrder(couponId) {
    return postForm('/api/v1/order/create', { couponId: String(couponId) }).then((o) =>
      o
        ? {
            ...o,
            id: String(o.id),
            userId: String(o.userId),
            couponId: String(o.couponId),
          }
        : o
    )
  },

  seckill(couponId) {
    return postForm('/api/v1/order/seckill', { couponId: String(couponId) })
  },

  checkReceived(couponId) {
    return request('/api/v1/order/check/' + encodeURIComponent(String(couponId)))
  },

  cancelOrder(orderId) {
    return postForm('/api/v1/order/cancel', { orderId: String(orderId) })
  },

  /** 热路径 QUEUED 后轮询是否已落单 */
  async seckillUntilReceived(couponId, maxAttempts = 10) {
    await this.seckill(couponId)
    for (let i = 0; i < maxAttempts; i++) {
      const ok = await this.checkReceived(couponId).catch(() => false)
      if (ok) return true
      await new Promise((r) => setTimeout(r, Math.min(200 * 2 ** i, 2000)))
    }
    throw new Error('秒杀受理中，请稍后在「订单」查看')
  },
}

module.exports = api
