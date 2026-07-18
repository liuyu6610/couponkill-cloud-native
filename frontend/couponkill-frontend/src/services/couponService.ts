import { http } from '../lib/apiClient'
import { CouponType } from '../types/api'
import type { Coupon } from '../types/api'

export interface CreateCouponParams {
  id?: string
  name: string
  description?: string
  type: number
  faceValue: number
  minSpend: number
  validDays: number
  perUserLimit: number
  totalStock: number
  seckillTotalStock?: number
  /** yyyy-MM-dd HH:mm:ss */
  seckillStartAt?: string
  seckillEndAt?: string
}

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

  /** 管理写：创建（网关 JWT+admin） */
  async createCoupon(params: CreateCouponParams): Promise<Coupon> {
    return http.post<Coupon>('/api/v1/coupon/create', null, { params })
  },

  /** 管理写：更新秒杀时间窗（全分片） */
  async updateSeckillWindow(
    id: string,
    seckillStartAt: string,
    seckillEndAt: string
  ): Promise<Coupon> {
    return http.post<Coupon>(`/api/v1/coupon/${id}/seckill-window`, null, {
      params: { seckillStartAt, seckillEndAt },
    })
  },

  /** 管理写：更新状态 0/1（全分片） */
  async updateCouponStatus(id: string, status: number): Promise<number> {
    return http.post<number>(`/api/v1/coupon/${id}/status`, null, {
      params: { status },
    })
  },

  /** 管理写：删除（全分片） */
  async deleteCoupon(id: string): Promise<number> {
    return http.del<number>(`/api/v1/coupon/${id}`)
  },
}

export default couponService
