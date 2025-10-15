import apiClient from './authService'

// 优惠券相关API
export const couponService = {
  // 获取优惠券列表
  async getCoupons(params: {
    page?: number
    size?: number
    type?: string
    status?: string
    search?: string
  } = {}) {
    const response = await apiClient.get('/api/v1/coupons', { params })
    return response.data
  },

  // 获取优惠券详情
  async getCouponDetail(couponId: string) {
    const response = await apiClient.get(`/api/v1/coupons/${couponId}`)
    return response.data
  },

  // 获取秒杀优惠券列表
  async getSeckillCoupons() {
    const response = await apiClient.get('/api/v1/coupons/seckill')
    return response.data
  },

  // 参与秒杀
  async participateSeckill(couponId: string) {
    const response = await apiClient.post(`/api/v1/coupons/${couponId}/seckill`)
    return response.data
  },

  // 获取热门优惠券
  async getHotCoupons(limit = 10) {
    const response = await apiClient.get('/api/v1/coupons/hot', {
      params: { limit }
    })
    return response.data
  },

  // 获取推荐优惠券
  async getRecommendedCoupons(limit = 10) {
    const response = await apiClient.get('/api/v1/coupons/recommended', {
      params: { limit }
    })
    return response.data
  },
}

// 导出便捷方法
export const getCoupons = couponService.getCoupons
export const getCouponDetail = couponService.getCouponDetail
export const participateSeckill = couponService.participateSeckill

export default couponService
