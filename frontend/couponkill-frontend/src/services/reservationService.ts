import { http } from '../lib/apiClient'
import type { SeckillReservation } from '../types/api'

export const reservationService = {
  create(couponId: string): Promise<SeckillReservation> {
    return http.post<SeckillReservation>('/order/reservations', { couponId })
  },

  cancel(id: string): Promise<boolean> {
    return http.del<boolean>(`/order/reservations/${id}`)
  },

  listMine(): Promise<SeckillReservation[]> {
    return http.get<SeckillReservation[]>('/order/reservations/mine')
  },

  getById(id: string): Promise<SeckillReservation> {
    return http.get<SeckillReservation>(`/order/reservations/${id}`)
  },
}

export default reservationService
