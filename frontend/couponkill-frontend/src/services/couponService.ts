import { http } from '../lib/apiClient'
import { toList } from '../lib/page'
import { CouponType } from '../types/api'
import type { Coupon } from '../types/api'

/** 创建优惠券的请求体（JSON，POST /api/v1/coupon/create，admin） */
export interface CreateCouponParams {
  name: string
  description?: string
  type: number
  /** 金额，最多两位小数 */
  faceValue: number
  minSpend: number
  validDays: number
  perUserLimit: number
  /** 发行总量；秒杀券即秒杀总库存 */
  totalStock: number
  /** ISO-8601（带时区），如 2026-11-06T10:00:00.000Z */
  seckillStartAt?: string
  seckillEndAt?: string
}

// 优惠券接口（对齐 coupon-service: /api/v1/coupon/**）；列表兼容裸数组与分页信封
export const couponService = {
  // 可用优惠券列表（公开）
  async getAvailableCoupons(): Promise<Coupon[]> {
    return toList(await http.get<Coupon[] | { items: Coupon[] }>('/api/v1/coupon/available'))
  },

  // 全部优惠券（管理台用）
  async getAllCoupons(): Promise<Coupon[]> {
    return toList(await http.get<Coupon[] | { items: Coupon[] }>('/api/v1/coupon/list'))
  },

  // 优惠券详情
  async getCouponById(id: string): Promise<Coupon> {
    return http.get<Coupon>(`/api/v1/coupon/${encodeURIComponent(id)}`)
  },

  // 秒杀优惠券：后端无专用列表接口，从可用列表按 type 过滤
  async getSeckillCoupons(): Promise<Coupon[]> {
    const all = await couponService.getAvailableCoupons()
    return all.filter((c) => c.type === CouponType.SECKILL)
  },

  /** 管理写：创建（admin）。POST /create {…} → Coupon */
  async createCoupon(params: CreateCouponParams): Promise<Coupon> {
    return http.post<Coupon>('/api/v1/coupon/create', params)
  },

  /** 管理写：更新秒杀时间窗。POST /{id}/seckill-window {seckillStartAt, seckillEndAt}（ISO-8601） */
  async updateSeckillWindow(id: string, seckillStartAt: string, seckillEndAt: string): Promise<Coupon> {
    return http.post<Coupon>(`/api/v1/coupon/${encodeURIComponent(id)}/seckill-window`, {
      seckillStartAt,
      seckillEndAt,
    })
  },

  /** 管理写：更新状态 0/1。POST /{id}/status {status} */
  async updateCouponStatus(id: string, status: number): Promise<number> {
    return http.post<number>(`/api/v1/coupon/${encodeURIComponent(id)}/status`, { status })
  },

  /** 管理写：删除。DELETE /{id} */
  async deleteCoupon(id: string): Promise<number> {
    return http.del<number>(`/api/v1/coupon/${encodeURIComponent(id)}`)
  },
}

export default couponService
