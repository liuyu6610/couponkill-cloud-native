import { http } from '../lib/apiClient'
import { toPage } from '../lib/page'
import type { Order, Page, SeckillAccepted, SeckillResult } from '../types/api'

// 统一走 /api/v1/order/**；身份由网关 JWT → X-User-Id（带签名）注入，请求里不再传可伪造的 userId。
// 写接口一律 JSON 请求体（不再用 query 参数）。
const ORDER_BASE = '/api/v1/order'

export const orderService = {
  /** 我的订单（分页，按下单时间倒序）。GET /user/me?page=&size= → Page<Order> */
  async getUserOrders(page = 1, size = 10): Promise<Page<Order>> {
    const raw = await http.get<Page<Order> | Order[]>(`${ORDER_BASE}/user/me`, {
      params: { page, size },
    })
    return toPage(raw, page, size)
  },

  /** 领取常驻券。POST /create {couponId}；秒杀券后端返回 10012（SeckillUseDedicatedAPI） */
  async createOrder(couponId: string): Promise<Order> {
    return http.post<Order>(`${ORDER_BASE}/create`, { couponId })
  },

  /** 秒杀入队：POST /seckill {couponId} → 202 {requestId, status:"QUEUED"}；拒绝走 409/429/503 + 业务码 */
  async seckill(couponId: string): Promise<SeckillAccepted> {
    return http.post<SeckillAccepted>(`${ORDER_BASE}/seckill`, { couponId })
  },

  /**
   * 按 requestId 查秒杀结果：GET /seckill/result?requestId= → {status, orderId?, reason?}；
   * 非属主或不存在返回 404。signal 用于卸载时中止轮询。
   */
  async seckillResult(requestId: string, signal?: AbortSignal): Promise<SeckillResult> {
    return http.get<SeckillResult>(`${ORDER_BASE}/seckill/result`, {
      params: { requestId },
      signal,
    })
  },

  /** 取消订单：POST /cancel {orderId}（仅「已创建」可取消） */
  async cancelOrder(orderId: string): Promise<boolean> {
    return http.post<boolean>(`${ORDER_BASE}/cancel`, { orderId })
  },
}

export default orderService
