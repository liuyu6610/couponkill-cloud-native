import { http } from '../lib/apiClient'
import type { Order } from '../types/api'

// 订单 / 秒杀接口：身份由网关 JWT → X-User-Id 注入，不再传可伪造的 userId
export const orderService = {
  async getUserOrders(pageNum = 1, pageSize = 10): Promise<Order[]> {
    return http.get<Order[]>('/order/user/me', {
      params: { pageNum, pageSize },
    })
  },

  async createOrder(couponId: string): Promise<Order> {
    return http.post<Order>('/order/create', null, {
      params: { couponId },
    })
  },

  async seckill(couponId: string): Promise<unknown> {
    return http.post<unknown>('/order/seckill', null, {
      params: { couponId },
    })
  },

  async cancelOrder(orderId: string): Promise<boolean> {
    return http.post<boolean>('/order/cancel', null, {
      params: { orderId },
    })
  },

  async checkReceived(couponId: string): Promise<boolean> {
    return http.get<boolean>(`/order/check/${couponId}`)
  },
}

export default orderService
