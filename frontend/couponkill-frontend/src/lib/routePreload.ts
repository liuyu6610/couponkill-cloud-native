/** 路由级懒加载工厂 + intent preload（hover 时预取 chunk） */

export const loadLogin = () => import('../pages/Login')
export const loadRegister = () => import('../pages/Register')
export const loadHome = () => import('../pages/Home')
export const loadCouponList = () => import('../pages/CouponList')
export const loadCouponDetail = () => import('../pages/CouponDetail')
export const loadSeckill = () => import('../pages/Seckill')
export const loadOrderList = () => import('../pages/OrderList')
export const loadOrderDetail = () => import('../pages/OrderDetail')
export const loadUserCenter = () => import('../pages/UserCenter')
export const loadConnectorAdmin = () => import('../pages/ConnectorAdmin')
export const loadCouponAdmin = () => import('../pages/CouponAdmin')
export const loadMyReservations = () => import('../pages/MyReservations')

const preloaders: Record<string, () => Promise<unknown>> = {
  '/': loadHome,
  '/coupons': loadCouponList,
  '/seckill': loadSeckill,
  '/orders': loadOrderList,
  '/reservations': loadMyReservations,
  '/user': loadUserCenter,
  '/login': loadLogin,
  '/register': loadRegister,
  '/admin/connector': loadConnectorAdmin,
  '/admin/coupons': loadCouponAdmin,
}

/** 带参数的详情路由：需先于前缀表匹配，否则 /coupons/1 会误命中 /coupons 预取成列表页 chunk */
const detailPreloaders: Array<[RegExp, () => Promise<unknown>]> = [
  [/^\/coupons\/[^/]+/, loadCouponDetail],
  [/^\/orders\/[^/]+/, loadOrderDetail],
]

/** 按路径意图预加载；已加载过的模块会被浏览器/ bundler 缓存，可重复调用 */
export function preloadRoute(path: string): void {
  const detail = detailPreloaders.find(([re]) => re.test(path))
  if (detail) {
    void detail[1]().catch((err) => console.warn('[preload] 详情页 chunk 预取失败', path, err))
    return
  }
  const key = Object.keys(preloaders).find((p) => path === p || path.startsWith(p + '/'))
  const loader = key ? preloaders[key] : undefined
  if (loader) {
    // 预取失败（弱网/发版后旧 chunk 404）不应产生未处理的 Promise 拒绝；真正导航时 lazy 会再加载一次
    void loader().catch((err) => console.warn('[preload] chunk 预取失败', path, err))
  }
}
