import { http, STORAGE_KEYS } from '../lib/apiClient'
import type { LoginResult, UserInfo } from '../types/api'

// 认证 / 用户账户相关接口（对齐 user-service: /api/v1/user/**）
export const authService = {
  async login(username: string, password: string): Promise<LoginResult> {
    const data = await http.post<LoginResult>('/api/v1/user/login', null, {
      params: { username, password },
    })
    return data
  },

  async register(username: string, password: string, phone: string): Promise<UserInfo> {
    return http.post<UserInfo>('/api/v1/user/register', null, {
      params: { username, password, phone },
    })
  },

  // 身份取自网关 X-User-Id，无需再传 userId
  async getProfile(): Promise<UserInfo> {
    return http.get<UserInfo>('/api/v1/user/profile')
  },

  logout(): void {
    localStorage.removeItem(STORAGE_KEYS.token)
    localStorage.removeItem(STORAGE_KEYS.userId)
    localStorage.removeItem(STORAGE_KEYS.username)
    localStorage.removeItem(STORAGE_KEYS.roles)
  },
}

export default authService
