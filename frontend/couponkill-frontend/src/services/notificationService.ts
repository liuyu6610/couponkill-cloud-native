import { http } from '../lib/apiClient'
import { toList } from '../lib/page'
import type { UserNotification } from '../types/api'

const BASE = '/api/v1/order/notifications'

export const notificationService = {
  /** GET /mine?limit= → 通知数组（或分页信封），按时间倒序 */
  async listMine(limit = 20): Promise<UserNotification[]> {
    return toList(
      await http.get<UserNotification[] | { items: UserNotification[] }>(`${BASE}/mine`, {
        params: { limit },
      })
    )
  },

  /** GET /unread-count → {count} */
  unreadCount(): Promise<number> {
    return http.get<{ count: number }>(`${BASE}/unread-count`).then((r) => r?.count ?? 0)
  },

  /** POST /{id}/read */
  markRead(id: string): Promise<unknown> {
    return http.post<unknown>(`${BASE}/${encodeURIComponent(id)}/read`)
  },

  /** POST /read-all */
  markAllRead(): Promise<unknown> {
    return http.post<unknown>(`${BASE}/read-all`)
  },
}

export default notificationService
