import { AxiosError } from 'axios'
import type { InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Mock } from 'vitest'
import instance, {
  ApiError,
  ErrorCode,
  STORAGE_KEYS,
  classifyApiError,
  http,
  httpStatusMessage,
  normalizeApiBaseUrl,
  resetSessionExpiryGuard,
  setSessionExpiredHandler,
} from './apiClient'

describe('normalizeApiBaseUrl', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('留空/空白时返回空串，走同源相对路径', () => {
    expect(normalizeApiBaseUrl(undefined)).toBe('')
    expect(normalizeApiBaseUrl(null)).toBe('')
    expect(normalizeApiBaseUrl('')).toBe('')
    expect(normalizeApiBaseUrl('   ')).toBe('')
  })

  it('只去掉尾部斜杠，保留网关源', () => {
    expect(normalizeApiBaseUrl('https://gw.example.com/')).toBe('https://gw.example.com')
    expect(normalizeApiBaseUrl(' http://localhost:8088// ')).toBe('http://localhost:8088')
  })

  it('误带 /api/v1（含大小写、尾斜杠）时剥掉并告警，避免拼成 /api/v1/api/v1', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    expect(normalizeApiBaseUrl('https://gw.example.com/api/v1')).toBe('https://gw.example.com')
    expect(normalizeApiBaseUrl('https://gw.example.com/API/V1/')).toBe('https://gw.example.com')
    expect(normalizeApiBaseUrl('/api/v1')).toBe('')
    expect(warn).toHaveBeenCalledTimes(3)
  })

  it('其它路径前缀不动（如网关挂在子路径下）', () => {
    expect(normalizeApiBaseUrl('https://example.com/gateway')).toBe('https://example.com/gateway')
    expect(normalizeApiBaseUrl('https://example.com/api/v1x')).toBe('https://example.com/api/v1x')
  })
})

describe('httpStatusMessage', () => {
  it('无状态码（网络错误）返回 undefined，交给上层兜底', () => {
    expect(httpStatusMessage(undefined)).toBeUndefined()
  })

  it('响应体没有 message 时，常见状态给出中文文案', () => {
    expect(httpStatusMessage(400)).toBe('请求参数有误，请检查后重试')
    expect(httpStatusMessage(401)).toBe('登录已过期，请重新登录')
    expect(httpStatusMessage(403)).toBe('无权限执行该操作')
    expect(httpStatusMessage(404)).toBe('请求的资源不存在')
    expect(httpStatusMessage(409)).toBe('操作冲突，请刷新后重试')
    expect(httpStatusMessage(429)).toBe('请求过于频繁，请稍后再试')
    expect(httpStatusMessage(503)).toBe('系统繁忙，请稍后再试')
  })

  it('其它 5xx 归为服务暂时不可用', () => {
    for (const s of [500, 502, 504, 599]) {
      expect(httpStatusMessage(s)).toBe('服务暂时不可用，请稍后重试')
    }
  })

  it('2xx 与未覆盖的 4xx 不给文案', () => {
    expect(httpStatusMessage(200)).toBeUndefined()
    expect(httpStatusMessage(418)).toBeUndefined()
  })
})

describe('classifyApiError', () => {
  it('按 HTTP 状态分类；401 的口令错误(20005)不算会话过期', () => {
    expect(classifyApiError(undefined)).toBe('network')
    expect(classifyApiError(0)).toBe('network')
    expect(classifyApiError(400)).toBe('invalid')
    expect(classifyApiError(401, ErrorCode.AuthFail)).toBe('session_expired')
    expect(classifyApiError(401, ErrorCode.TokenInvalid)).toBe('session_expired')
    expect(classifyApiError(401)).toBe('session_expired')
    expect(classifyApiError(401, ErrorCode.PasswordError)).toBe('unauthorized')
    expect(classifyApiError(403)).toBe('forbidden')
    expect(classifyApiError(404)).toBe('not_found')
    expect(classifyApiError(409)).toBe('conflict')
    expect(classifyApiError(429)).toBe('rate_limited')
    expect(classifyApiError(503)).toBe('busy')
    expect(classifyApiError(500)).toBe('server')
    expect(classifyApiError(502)).toBe('server')
  })
})

/** 用假适配器模拟服务端：2xx 正常返回，其它状态抛 AxiosError（与真实 axios 的 settle 行为一致） */
function stubServer(status: number, body: unknown, headers: Record<string, string> = {}) {
  instance.defaults.adapter = async (config: InternalAxiosRequestConfig) => {
    const response = { data: body, status, statusText: '', headers, config, request: {} }
    if (status >= 200 && status < 300) return response
    throw new AxiosError(`Request failed with status code ${status}`, 'ERR_BAD_REQUEST', config, {}, response)
  }
}

function envelope(code: number, message: string, requestId?: string) {
  return { code, message, data: null, ...(requestId ? { requestId } : {}) }
}

/** 捕获 http 调用抛出的 ApiError */
async function catchApiError(call: () => Promise<unknown>): Promise<ApiError> {
  try {
    await call()
  } catch (err) {
    expect(err).toBeInstanceOf(ApiError)
    return err as ApiError
  }
  throw new Error('expected the call to reject')
}

describe('http 错误映射（真实 HTTP 状态 + 信封）', () => {
  const originalAdapter = instance.defaults.adapter
  let onExpired: Mock<() => void>
  let restoreHandler: () => void

  beforeEach(() => {
    localStorage.clear()
    resetSessionExpiryGuard()
    onExpired = vi.fn<() => void>()
    restoreHandler = setSessionExpiredHandler(onExpired)
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    instance.defaults.adapter = originalAdapter
    restoreHandler()
    vi.restoreAllMocks()
    localStorage.clear()
  })

  it('成功：HTTP 200/202 的信封解包出 data（秒杀受理 202）', async () => {
    stubServer(202, { code: 200, message: 'ok', data: { requestId: 'r1', status: 'QUEUED' } })
    await expect(http.post('/api/v1/order/seckill', { couponId: '1' })).resolves.toEqual({
      requestId: 'r1',
      status: 'QUEUED',
    })
    stubServer(200, { code: 202, message: 'accepted', data: 7 })
    await expect(http.get('/x')).resolves.toBe(7)
  })

  it('成功：非信封响应体（如 204 空体）原样返回', async () => {
    stubServer(204, '')
    await expect(http.del('/x')).resolves.toBe('')
  })

  it('HTTP 2xx 但信封是业务失败码时抛 ApiError', async () => {
    stubServer(200, envelope(ErrorCode.OutOfStock, '库存不足'))
    const err = await catchApiError(() => http.get('/x'))
    expect(err.code).toBe(ErrorCode.OutOfStock)
    expect(err.message).toBe('库存不足')
  })

  // 每个状态：服务端 message / code 原样进入 ApiError，分类正确
  const cases: Array<[number, number, string, ApiError['kind']]> = [
    [400, ErrorCode.InvalidRequest, 'couponId 不能为空', 'invalid'],
    [403, ErrorCode.Forbidden, '无权限访问', 'forbidden'],
    [404, ErrorCode.CouponNotFound, '优惠券不存在', 'not_found'],
    [409, ErrorCode.OutOfStock, '库存不足', 'conflict'],
    [409, ErrorCode.RepeatSeckill, '请勿重复秒杀', 'conflict'],
    [409, ErrorCode.NotStarted, '活动尚未开始', 'conflict'],
    [409, ErrorCode.ActivityEnded, '活动已结束', 'conflict'],
    [409, ErrorCode.CouponDisabled, '优惠券已停用', 'conflict'],
    [429, ErrorCode.CoolingDown, '操作过于频繁，请稍后再试', 'rate_limited'],
    [429, ErrorCode.RateLimited, '请求过于频繁', 'rate_limited'],
    [503, ErrorCode.SystemBusy, '系统繁忙，请稍后再试', 'busy'],
    [500, ErrorCode.SysError, '系统错误', 'server'],
    [401, ErrorCode.PasswordError, '用户名或密码错误', 'unauthorized'],
  ]
  it.each(cases)('HTTP %i 业务码 %i：使用服务端 message，kind=%s', async (status, code, message, kind) => {
    stubServer(status, envelope(code, message, 'req-123'))
    const err = await catchApiError(() => http.post('/x', {}))
    expect(err.status).toBe(status)
    expect(err.code).toBe(code)
    expect(err.message).toBe(message)
    expect(err.kind).toBe(kind)
    expect(err.requestId).toBe('req-123')
  })

  it('requestId：信封缺省时取响应头 X-Request-Id；Retry-After 解析为秒', async () => {
    stubServer(429, envelope(ErrorCode.RateLimited, '请求过于频繁'), {
      'x-request-id': 'hdr-9',
      'retry-after': '3',
    })
    const err = await catchApiError(() => http.get('/x'))
    expect(err.requestId).toBe('hdr-9')
    expect(err.retryAfterSec).toBe(3)
  })

  it('响应体不是信封（网关空 body / HTML 错误页）时按 HTTP 状态给中文兜底，code 取 HTTP 状态', async () => {
    stubServer(502, '<html>Bad Gateway</html>')
    const bad = await catchApiError(() => http.get('/x'))
    expect(bad.message).toBe('服务暂时不可用，请稍后重试')
    expect(bad.code).toBe(502)
    expect(bad.kind).toBe('server')

    stubServer(403, '')
    expect((await catchApiError(() => http.get('/x'))).message).toBe('无权限执行该操作')
  })

  it('网络错误与超时：没有响应，kind 区分 network / timeout，code=-1', async () => {
    instance.defaults.adapter = async (config) => {
      throw new AxiosError('Network Error', 'ERR_NETWORK', config)
    }
    const net = await catchApiError(() => http.get('/x'))
    expect(net.kind).toBe('network')
    expect(net.code).toBe(-1)
    expect(net.status).toBe(0)

    instance.defaults.adapter = async (config) => {
      throw new AxiosError('timeout of 15000ms exceeded', 'ECONNABORTED', config)
    }
    const timeout = await catchApiError(() => http.get('/x'))
    expect(timeout.kind).toBe('timeout')
    expect(timeout.message).toContain('超时')
  })

  it('主动取消原样抛出，不当作网络错误也不触发登出', async () => {
    localStorage.setItem(STORAGE_KEYS.token, 'tok')
    const controller = new AbortController()
    instance.defaults.adapter = async (config) => {
      throw new AxiosError('canceled', AxiosError.ERR_CANCELED, config)
    }
    controller.abort()
    await expect(http.get('/x', { signal: controller.signal })).rejects.not.toBeInstanceOf(ApiError)
    expect(onExpired).not.toHaveBeenCalled()
    expect(localStorage.getItem(STORAGE_KEYS.token)).toBe('tok')
  })

  it('请求自动带 Bearer 令牌', async () => {
    localStorage.setItem(STORAGE_KEYS.token, 'tok-1')
    let sent: string | undefined
    instance.defaults.adapter = async (config) => {
      sent = String(config.headers.get('Authorization'))
      return { data: { code: 200, message: 'ok', data: 1 }, status: 200, statusText: '', headers: {}, config, request: {} }
    }
    await http.get('/x')
    expect(sent).toBe('Bearer tok-1')
  })

  describe('401 会话过期', () => {
    function loginAs(token: string) {
      localStorage.setItem(STORAGE_KEYS.token, token)
      localStorage.setItem(STORAGE_KEYS.userId, '42')
      localStorage.setItem(STORAGE_KEYS.username, 'demo')
      localStorage.setItem(STORAGE_KEYS.roles, '["user"]')
    }

    it('20001/20002：清会话并跳转登录页', async () => {
      for (const code of [ErrorCode.AuthFail, ErrorCode.TokenInvalid]) {
        resetSessionExpiryGuard()
        onExpired.mockClear()
        loginAs('tok')
        stubServer(401, envelope(code, '未登录或登录已过期'))
        const err = await catchApiError(() => http.get('/api/v1/order/user/me'))
        expect(err.kind).toBe('session_expired')
        expect(onExpired).toHaveBeenCalledTimes(1)
        expect(localStorage.getItem(STORAGE_KEYS.token)).toBeNull()
        expect(localStorage.getItem(STORAGE_KEYS.userId)).toBeNull()
        expect(localStorage.getItem(STORAGE_KEYS.username)).toBeNull()
        expect(localStorage.getItem(STORAGE_KEYS.roles)).toBeNull()
      }
    })

    it('并发请求同时 401 只跳转一次', async () => {
      loginAs('tok')
      stubServer(401, envelope(ErrorCode.TokenInvalid, '令牌无效'))
      const results = await Promise.allSettled([http.get('/a'), http.get('/b'), http.post('/c', {})])
      expect(results.every((r) => r.status === 'rejected')).toBe(true)
      expect(onExpired).toHaveBeenCalledTimes(1)
    })

    it('本地已无令牌的并发 401（如未登录访问受保护接口）同样只跳转一次，登录后重新武装', async () => {
      stubServer(401, envelope(ErrorCode.AuthFail, '未登录或登录已过期'))
      await Promise.allSettled([http.get('/a'), http.get('/b'), http.get('/c')])
      expect(onExpired).toHaveBeenCalledTimes(1)

      resetSessionExpiryGuard() // 登录成功时由 authSlice 调用
      loginAs('tok-2')
      await catchApiError(() => http.get('/d'))
      expect(onExpired).toHaveBeenCalledTimes(2)
    })

    it('用旧令牌发出的迟到 401 不会踢掉刚重新登录的新会话', async () => {
      loginAs('old-token')
      let release!: () => void
      let markStarted!: () => void
      const gate = new Promise<void>((r) => {
        release = r
      })
      const started = new Promise<void>((r) => {
        markStarted = r
      })
      instance.defaults.adapter = async (config) => {
        markStarted() // 请求已带着旧令牌发出
        await gate
        const response = { data: envelope(ErrorCode.TokenInvalid, '令牌无效'), status: 401, statusText: '', headers: {}, config, request: {} }
        throw new AxiosError('401', 'ERR_BAD_REQUEST', config, {}, response)
      }
      const inFlight = catchApiError(() => http.get('/slow'))
      await started
      loginAs('new-token') // 请求在途时用户重新登录
      release()
      await inFlight
      expect(onExpired).not.toHaveBeenCalled()
      expect(localStorage.getItem(STORAGE_KEYS.token)).toBe('new-token')
    })

    it('登录口令错误（401 + 20005）不是会话过期：不清会话、不跳转', async () => {
      loginAs('tok')
      stubServer(401, envelope(ErrorCode.PasswordError, '用户名或密码错误'))
      const err = await catchApiError(() => http.post('/api/v1/user/login', { username: 'a', password: 'b' }))
      expect(err.kind).toBe('unauthorized')
      expect(err.message).toBe('用户名或密码错误')
      expect(onExpired).not.toHaveBeenCalled()
      expect(localStorage.getItem(STORAGE_KEYS.token)).toBe('tok')
    })

    it('403 / 409 / 429 / 503 不触发登出', async () => {
      loginAs('tok')
      for (const status of [403, 409, 429, 503]) {
        stubServer(status, envelope(ErrorCode.Forbidden, 'x'))
        await catchApiError(() => http.get('/x'))
      }
      expect(onExpired).not.toHaveBeenCalled()
      expect(localStorage.getItem(STORAGE_KEYS.token)).toBe('tok')
    })
  })
})
