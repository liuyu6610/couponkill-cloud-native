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

/** 会话过期后跳转登录页时留给登录页的一次性提示标记（sessionStorage） */
export const SESSION_EXPIRED_FLAG = 'ck_session_expired'

/**
 * 后端业务错误码（对齐 internal/platform/apperr，码值与 Java 版一致，只增不改）。
 * HTTP 状态码随错误码走：400 校验、401 未登录、403 无权限、404 不存在、409 冲突/售罄/重复、
 * 429 限流/冷却、503 系统繁忙、其余 500。
 */
export const ErrorCode = {
  CoolingDown: 10001,
  OutOfStock: 10002,
  RateLimited: 10003,
  InvalidRequest: 10004,
  NotPreheated: 10005,
  NotStarted: 10006,
  ActivityEnded: 10007,
  CouponNotFound: 10008,
  CouponLimitExceeded: 10009,
  RepeatSeckill: 10010,
  OrderNotFound: 10011,
  SeckillUseDedicatedAPI: 10012,
  CouponServiceUnavailable: 10013,
  CouponDisabled: 10014,
  AuthFail: 20001,
  TokenInvalid: 20002,
  UserNotFound: 20003,
  UserExist: 20004,
  PasswordError: 20005,
  Forbidden: 20006,
  SysError: 50000,
  SystemBusy: 50003,
} as const

/** 没有收到任何 HTTP 响应（断网、超时、被取消以外的传输层错误）时 ApiError.code 的取值 */
export const NETWORK_ERROR_CODE = -1

/**
 * 错误分类，决定界面如何呈现：
 * - network / timeout：没有响应，提示检查网络；
 * - session_expired：401（令牌失效/被吊销），已清会话并跳转登录；
 * - unauthorized：401 但属于凭证错误（如登录口令错误 20005），不当作会话过期；
 * - forbidden / not_found / invalid：403 / 404 / 400（及其它 4xx）；
 * - conflict：409，售罄 / 重复 / 未开始 / 已结束 / 券停用等业务拒绝；
 * - rate_limited：429，冷却或限流，可稍后重试；
 * - busy：503，系统繁忙，可稍后重试；
 * - server：其它 5xx 或无法解析的响应，属于意外错误，界面应提供请求号以便排障。
 */
export type ApiErrorKind =
  | 'network'
  | 'timeout'
  | 'session_expired'
  | 'unauthorized'
  | 'forbidden'
  | 'not_found'
  | 'invalid'
  | 'conflict'
  | 'rate_limited'
  | 'busy'
  | 'server'

/** 按 HTTP 状态码与业务码给错误分类；status 为 0/undefined 表示没有响应 */
export function classifyApiError(status: number | undefined, code?: number): ApiErrorKind {
  if (!status) return 'network'
  if (status === 401) return code === ErrorCode.PasswordError ? 'unauthorized' : 'session_expired'
  if (status === 403) return 'forbidden'
  if (status === 404) return 'not_found'
  if (status === 409) return 'conflict'
  if (status === 429) return 'rate_limited'
  if (status === 503) return 'busy'
  if (status >= 500) return 'server'
  return 'invalid'
}

export interface ApiErrorInit {
  /** HTTP 状态码；0 = 没有响应 */
  status?: number
  /** 响应体 requestId，或响应头 X-Request-Id */
  requestId?: string
  /** 429/503 响应的 Retry-After（秒） */
  retryAfterSec?: number
  kind?: ApiErrorKind
}

/** 业务/传输错误：携带后端 code / message / HTTP 状态 / requestId，供上层区分处理 */
export class ApiError extends Error {
  /** 业务码（ErrorCode）；没有响应体时为 -1 */
  code: number
  /** HTTP 状态码；0 = 没有响应 */
  status: number
  kind: ApiErrorKind
  requestId?: string
  retryAfterSec?: number

  constructor(code: number, message: string, init: ApiErrorInit = {}) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = init.status ?? 0
    this.kind = init.kind ?? classifyApiError(this.status, code)
    this.requestId = init.requestId
    this.retryAfterSec = init.retryAfterSec
  }
}

/** 信封里的成功码：HTTP 2xx 对应 code 200（秒杀受理 202） */
function isSuccessCode(code: number): boolean {
  return code >= 200 && code < 300
}

/**
 * 规范化 API 基址。
 * - 留空 -> ''：同源相对路径（开发走 Vite proxy；生产由网关/Ingress 同域托管静态资源并转发 /api/**）。
 * - 只填网关源（协议+主机+端口），不要带 /api/v1：各 service 已写全 `/api/v1/...`，
 *   误带会拼成 `/api/v1/api/v1/...`。这里兜底剥掉尾部斜杠和误填的 /api/v1，并告警。
 */
export function normalizeApiBaseUrl(raw: string | undefined | null): string {
  let base = (raw ?? '').trim().replace(/\/+$/, '')
  if (/\/api\/v1$/i.test(base)) {
    console.warn('[apiClient] VITE_API_BASE_URL 不应包含 /api/v1，已自动去除：', raw)
    base = base.replace(/\/api\/v1$/i, '')
  }
  return base
}

/** 响应体没有可用 message（网关/代理的空 body、HTML 错误页）时，按 HTTP 状态给出中文兜底文案 */
export function httpStatusMessage(status: number | undefined): string | undefined {
  if (status == null) return undefined
  if (status === 400) return '请求参数有误，请检查后重试'
  if (status === 401) return '登录已过期，请重新登录'
  if (status === 403) return '无权限执行该操作'
  if (status === 404) return '请求的资源不存在'
  if (status === 409) return '操作冲突，请刷新后重试'
  if (status === 429) return '请求过于频繁，请稍后再试'
  if (status === 503) return '系统繁忙，请稍后再试'
  if (status >= 500) return '服务暂时不可用，请稍后重试'
  return undefined
}

/** 清理本地会话（令牌、用户信息、角色） */
export function clearStoredSession(): void {
  localStorage.removeItem(STORAGE_KEYS.token)
  localStorage.removeItem(STORAGE_KEYS.userId)
  localStorage.removeItem(STORAGE_KEYS.username)
  localStorage.removeItem(STORAGE_KEYS.roles)
}

/** 默认的会话过期处理：留一次性提示标记，然后整页跳转 /login（整页刷新同时清空 Redux 与查询缓存） */
function redirectToLogin(): void {
  try {
    sessionStorage.setItem(SESSION_EXPIRED_FLAG, '1')
  } catch {
    // 隐私模式等场景 sessionStorage 不可用，只是少一条提示
  }
  if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
    window.location.assign('/login')
  }
}

let sessionExpiredHandler: () => void = redirectToLogin
let sessionExpiryHandled = false

/** 替换会话过期时的跳转行为（测试用）；返回恢复默认行为的函数 */
export function setSessionExpiredHandler(handler: () => void): () => void {
  const previous = sessionExpiredHandler
  sessionExpiredHandler = handler
  return () => {
    sessionExpiredHandler = previous
  }
}

/** 登录成功后调用：重新武装 401 处理（每个会话只处理一次过期） */
export function resetSessionExpiryGuard(): void {
  sessionExpiryHandled = false
}

/** 取出请求当时携带的 Authorization（兼容 AxiosHeaders 与普通对象） */
function sentAuthorization(config: AxiosRequestConfig | undefined): string | null {
  const headers = config?.headers as
    | { get?: (name: string) => unknown; Authorization?: unknown }
    | undefined
  const raw = typeof headers?.get === 'function' ? headers.get('Authorization') : headers?.Authorization
  return typeof raw === 'string' && raw ? raw : null
}

/**
 * 401 处理：清会话并跳转登录页，且整个会话只执行一次。
 * 并发请求同时 401 时只有第一个生效；用旧令牌发出的迟到 401 不会把刚重新登录的会话踢掉
 * （请求携带的令牌与当前本地令牌不一致就忽略）。
 */
function handleSessionExpired(config: AxiosRequestConfig | undefined, url: string | undefined): void {
  if (sessionExpiryHandled) return
  const current = localStorage.getItem(STORAGE_KEYS.token)
  const sent = sentAuthorization(config)
  if ((sent ?? null) !== (current ? `Bearer ${current}` : null)) return
  sessionExpiryHandled = true
  console.warn('[apiClient] 401：登录态失效，清理本地会话', url)
  clearStoredSession()
  sessionExpiredHandler()
}

/** 读取响应头（axios 把头名统一成小写；兼容 AxiosHeaders 与普通对象） */
function readHeader(headers: unknown, name: string): string | undefined {
  if (!headers || typeof headers !== 'object') return undefined
  const h = headers as { get?: (n: string) => unknown } & Record<string, unknown>
  const v = typeof h.get === 'function' ? h.get(name) : h[name]
  return typeof v === 'string' && v ? v : undefined
}

/** 把 axios 抛出的错误规整为 ApiError：优先用响应体 {code,message,requestId}，其次 HTTP 状态兜底文案 */
export function toApiError(error: unknown): ApiError {
  if (error instanceof ApiError) return error
  const e = error as {
    code?: string
    message?: string
    response?: { status?: number; data?: unknown; headers?: unknown }
  }
  const response = e?.response
  const status = response?.status
  const data = response?.data
  const body = data && typeof data === 'object' ? (data as Partial<ApiEnvelope<unknown>>) : undefined
  const bodyCode = typeof body?.code === 'number' ? body.code : undefined
  const bodyMessage = typeof body?.message === 'string' && body.message ? body.message : undefined
  const requestId =
    (typeof body?.requestId === 'string' && body.requestId ? body.requestId : undefined) ??
    readHeader(response?.headers, 'x-request-id')
  const retryAfter = Number(readHeader(response?.headers, 'retry-after'))
  const retryAfterSec = Number.isFinite(retryAfter) && retryAfter > 0 ? retryAfter : undefined

  if (!status) {
    const timedOut = e?.code === 'ECONNABORTED' || e?.code === 'ETIMEDOUT'
    return new ApiError(NETWORK_ERROR_CODE, timedOut ? '请求超时，请稍后重试' : '网络异常，请检查网络后重试', {
      status: 0,
      kind: timedOut ? 'timeout' : 'network',
    })
  }
  const message = bodyMessage || httpStatusMessage(status) || e?.message || '请求失败，请稍后重试'
  return new ApiError(bodyCode ?? status, message, { status, requestId, retryAfterSec })
}

const instance = axios.create({
  baseURL: normalizeApiBaseUrl(import.meta.env.VITE_API_BASE_URL),
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

// 响应拦截：非 2xx 一律转成 ApiError（带服务端 code/message/requestId），401 清会话并跳转登录
instance.interceptors.response.use(
  (response: AxiosResponse) => response,
  (error: unknown) => {
    // 主动取消（组件卸载中止轮询等）原样抛出，不当作网络错误、也不触发登出
    if (axios.isCancel(error)) {
      return Promise.reject(error)
    }
    const apiError = toApiError(error)
    const config = (error as { config?: AxiosRequestConfig })?.config
    if (apiError.kind === 'session_expired') {
      handleSessionExpired(config, config?.url)
    } else {
      console.warn('[apiClient] 请求失败', {
        method: config?.method,
        url: config?.url,
        status: apiError.status,
        code: apiError.code,
        requestId: apiError.requestId,
        message: apiError.message,
      })
    }
    return Promise.reject(apiError)
  }
)

// 解包统一响应体：成功返回 data，失败抛 ApiError
function unwrap<T>(response: AxiosResponse<ApiEnvelope<T>>): T {
  const body = response.data
  if (body && typeof body === 'object' && 'code' in body) {
    if (isSuccessCode(body.code)) {
      return body.data
    }
    // HTTP 2xx 但信封是业务失败码（经代理改写等异常链路）：同样按业务错误处理
    throw new ApiError(body.code, body.message || '请求失败', {
      status: response.status,
      requestId: body.requestId ?? readHeader(response.headers, 'x-request-id'),
      kind: 'invalid',
    })
  }
  // 非标准包装体（如 204 空响应），直接返回原始数据
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
