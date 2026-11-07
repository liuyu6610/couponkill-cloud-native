import { http } from '../lib/apiClient'
import { toList } from '../lib/page'
import type { SeckillReservation } from '../types/api'

// 统一走 /api/v1/order/reservations/**
const RESERVATION_BASE = '/api/v1/order/reservations'

export const reservationService = {
  /** POST / {couponId} → 预约；同一用户同一券只能有一个进行中的预约（重复返回 409） */
  create(couponId: string): Promise<SeckillReservation> {
    return http.post<SeckillReservation>(RESERVATION_BASE, { couponId })
  },

  /** DELETE /{id}：仅 PENDING 可取消 */
  cancel(id: string): Promise<unknown> {
    return http.del<unknown>(`${RESERVATION_BASE}/${encodeURIComponent(id)}`)
  },

  /** GET /mine → 我的预约（按创建时间倒序） */
  async listMine(): Promise<SeckillReservation[]> {
    return toList(
      await http.get<SeckillReservation[] | { items: SeckillReservation[] }>(`${RESERVATION_BASE}/mine`)
    )
  },

  /** GET /{id} */
  getById(id: string): Promise<SeckillReservation> {
    return http.get<SeckillReservation>(`${RESERVATION_BASE}/${encodeURIComponent(id)}`)
  },
}

export default reservationService
