import axios from 'axios'
import type { AxiosRequestConfig, AxiosResponse } from 'axios'
import type { ApiEnvelope } from '../types/api'

// 本地存储键（单一真源）
export const STORAGE_KEYS = {
  token: 'ck_token',
  userId: 'ck_userId',
  username: 'ck_username',
  roles: 'ck_roles',
} as const

// 业务错误：携带后端 code / message，供上层区分处理
export class ApiError extends Error {
  code: number
  constructor(code: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.code = code
  }
}

// Phase2：order 已统一 ApiResponse(code=0)；过渡期仍兼容历史 Result(code=200)
const SUCCESS_CODES = new Set<number>([0, 200])

const instance = axios.create({
  // baseURL 留空 -> 相对路径（开发走 Vite proxy，生产走同源网关或 VITE_API_BASE_URL）
  baseURL: import.meta.env.VITE_API_BASE_URL || '',
  timeout: 15000,
})

// 请求拦截：注入 Bearer token
instance.interceptors.request.use((config) => {
  const token = localStorage.getItem(STORAGE_KEYS.token)
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// 响应拦截：统一解包 + 401 处理
instance.interceptors.response.use(
  (response: AxiosResponse<ApiEnvelope<unknown>>) => response,
  (error) => {
    const status = error?.response?.status
    if (status === 401) {
      localStorage.removeItem(STORAGE_KEYS.token)
      localStorage.removeItem(STORAGE_KEYS.userId)
      localStorage.removeItem(STORAGE_KEYS.username)
      localStorage.removeItem(STORAGE_KEYS.roles)
      if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
        window.location.href = '/login'
      }
    }
    const message =
      error?.response?.data?.message || error?.message || '网络异常，请稍后重试'
    return Promise.reject(new ApiError(status ?? -1, message))
  }
)

// 解包统一响应体：成功返回 data，失败抛 ApiError
function unwrap<T>(response: AxiosResponse<ApiEnvelope<T>>): T {
  const body = response.data
  if (body && typeof body === 'object' && 'code' in body) {
    if (SUCCESS_CODES.has(body.code)) {
      return body.data
    }
    throw new ApiError(body.code, body.message || '请求失败')
  }
  // 非标准包装体，直接返回原始数据
  return body as unknown as T
}

export const http = {
  async get<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(await instance.get<ApiEnvelope<T>>(url, config))
  },
  async post<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(await instance.post<ApiEnvelope<T>>(url, data, config))
  },
  async put<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(await instance.put<ApiEnvelope<T>>(url, data, config))
  },
  async del<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(await instance.delete<ApiEnvelope<T>>(url, config))
  },
}

export default instance
