import { http } from '../lib/apiClient'
import type { SeckillReservation } from '../types/api'

// Phase1：统一走 /api/v1/order/reservations/**（旧 /order/reservations 仍兼容）
const RESERVATION_BASE = '/api/v1/order/reservations'

export const reservationService = {
  create(couponId: string): Promise<SeckillReservation> {
    return http.post<SeckillReservation>(RESERVATION_BASE, { couponId })
  },

  cancel(id: string): Promise<boolean> {
    return http.del<boolean>(`${RESERVATION_BASE}/${id}`)
  },

  listMine(): Promise<SeckillReservation[]> {
    return http.get<SeckillReservation[]>(`${RESERVATION_BASE}/mine`)
  },

  getById(id: string): Promise<SeckillReservation> {
    return http.get<SeckillReservation>(`${RESERVATION_BASE}/${id}`)
  },
}

export default reservationService
