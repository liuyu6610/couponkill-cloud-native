import { http } from '../lib/apiClient'
import type { UserNotification } from '../types/api'

const BASE = '/api/v1/order/notifications'

export const notificationService = {
  listMine(limit = 20): Promise<UserNotification[]> {
    return http.get<UserNotification[]>(`${BASE}/mine`, { params: { limit } })
  },

  unreadCount(): Promise<number> {
    return http.get<{ count: number }>(`${BASE}/unread-count`).then((r) => r?.count ?? 0)
  },

  markRead(id: string): Promise<boolean> {
    return http.post<boolean>(`${BASE}/${id}/read`)
  },

  markAllRead(): Promise<number> {
    return http.post<number>(`${BASE}/read-all`)
  },
}

export default notificationService
