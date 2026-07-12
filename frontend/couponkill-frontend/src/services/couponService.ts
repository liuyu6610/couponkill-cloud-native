import { http } from '../lib/apiClient'
import { CouponType } from '../types/api'
import type { Coupon } from '../types/api'

// 优惠券接口（对齐 coupon-service: /api/v1/coupon/**）
export const couponService = {
  // 可用优惠券列表
  async getAvailableCoupons(): Promise<Coupon[]> {
    return http.get<Coupon[]>('/api/v1/coupon/available')
  },

  // 全部优惠券
  async getAllCoupons(): Promise<Coupon[]> {
    return http.get<Coupon[]>('/api/v1/coupon/list')
  },

  // 优惠券详情
  async getCouponById(id: string): Promise<Coupon> {
    return http.get<Coupon>(`/api/v1/coupon/${id}`)
  },

  // 秒杀优惠券：后端无专用列表接口，从可用列表按 type 过滤
  async getSeckillCoupons(): Promise<Coupon[]> {
    const all = await couponService.getAvailableCoupons()
    return all.filter((c) => c.type === CouponType.SECKILL)
  },
}

export default couponService
