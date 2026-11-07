import { http } from '../lib/apiClient'
import { authService } from './authService'
import type { UserInfo } from '../types/api'

export const userService = {
  async getProfile(): Promise<UserInfo> {
    return authService.getProfile()
  },

  /**
   * 我的优惠券数量：GET /api/v1/user/coupon/count（网关路由到 order 服务，身份取自网关头）。
   * data 兼容裸数字与 `{count}`。
   */
  async getCouponCount(): Promise<number> {
    const raw = await http.get<number | { count: number } | null>('/api/v1/user/coupon/count')
    if (typeof raw === 'number') return raw
    return typeof raw?.count === 'number' ? raw.count : 0
  },
}

export default userService
