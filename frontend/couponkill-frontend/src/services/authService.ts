import axios from 'axios'
import { clearStoredSession, http, normalizeApiBaseUrl, STORAGE_KEYS } from '../lib/apiClient'
import type { LoginResult, UserInfo } from '../types/api'

// 认证 / 用户账户相关接口（对齐 user-service: /api/v1/user/**）
export const authService = {
  /**
   * 登录：POST /api/v1/user/login，凭证放在 JSON 请求体中提交。
   * 不能用 axios params（URL 查询串）——查询串会进入网关访问日志和浏览器历史。
   * 返回 {token, userId, username, roles}；口令错误是 401 + 20005（不会被当作会话过期）。
   */
  async login(username: string, password: string): Promise<LoginResult> {
    return http.post<LoginResult>('/api/v1/user/login', { username, password })
  },

  /** 注册：同样以 JSON 请求体提交，口令不出现在 URL 中 */
  async register(username: string, password: string, phone: string): Promise<UserInfo> {
    return http.post<UserInfo>('/api/v1/user/register', { username, password, phone })
  },

  // 身份取自网关 X-User-Id，无需再传 userId
  async getProfile(): Promise<UserInfo> {
    return http.get<UserInfo>('/api/v1/user/profile')
  },

  /**
   * 退出登录：先尽力通知服务端吊销当前令牌（POST /api/v1/user/logout，按 jti 吊销，之后该令牌立即失效），
   * 再清理本地会话。服务端调用是 fire-and-forget：不阻塞 UI，失败只打日志；
   * 用裸 axios 绕开 apiClient 的 401 拦截器，避免令牌已过期时把用户重定向到登录页。
   */
  logout(): void {
    const token = localStorage.getItem(STORAGE_KEYS.token)
    if (token) {
      const baseURL = normalizeApiBaseUrl(import.meta.env.VITE_API_BASE_URL)
      axios
        .post(`${baseURL}/api/v1/user/logout`, null, {
          headers: { Authorization: `Bearer ${token}` },
          timeout: 5000,
        })
        .catch((err: unknown) => {
          console.warn('[authService] 服务端退出登录失败，仅清理本地会话', err)
        })
    }
    clearStoredSession()
  },
}

export default authService
