import { http } from '../lib/apiClient'
import type { EnterSeckillResp, Order } from '../types/api'

// Phase1：统一走 /api/v1/order/**（网关与后端仍兼容旧 /order/**）
// 身份由网关 JWT → X-User-Id 注入，不再传可伪造的 userId
const ORDER_BASE = '/api/v1/order'

export const orderService = {
  async getUserOrders(pageNum = 1, pageSize = 10): Promise<Order[]> {
    return http.get<Order[]>(`${ORDER_BASE}/user/me`, {
      params: { pageNum, pageSize },
    })
  },

  async createOrder(couponId: string): Promise<Order> {
    return http.post<Order>(`${ORDER_BASE}/create`, null, {
      params: { couponId },
    })
  },

  /** 热路径入队；成功 data 含 requestId，供 /seckill/result 轮询 */
  async seckill(couponId: string): Promise<EnterSeckillResp> {
    return http.post<EnterSeckillResp>(`${ORDER_BASE}/seckill`, null, {
      params: { couponId },
    })
  },

  /** 按 requestId 查异步落单结果：PENDING / SUCCESS:orderId / FAIL / UNKNOWN */
  async seckillResult(requestId: string): Promise<string> {
    return http.get<string>(`${ORDER_BASE}/seckill/result`, {
      params: { requestId },
    })
  },

  async cancelOrder(orderId: string): Promise<boolean> {
    return http.post<boolean>(`${ORDER_BASE}/cancel`, null, {
      params: { orderId },
    })
  },

  async checkReceived(couponId: string): Promise<boolean> {
    return http.get<boolean>(`${ORDER_BASE}/check/${couponId}`)
  },
}

export default orderService
