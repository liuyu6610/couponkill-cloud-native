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

/** 按路径意图预加载；已加载过的模块会被浏览器/ bundler 缓存，可重复调用 */
export function preloadRoute(path: string): void {
  const key = Object.keys(preloaders).find((p) => path === p || path.startsWith(p + '/'))
  const loader = key ? preloaders[key] : undefined
  if (loader) {
    void loader()
  }
}
