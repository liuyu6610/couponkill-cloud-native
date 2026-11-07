import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, ErrorCode } from './apiClient'
import {
  copyText,
  errorSeverity,
  getErrorMessage,
  getRequestId,
  isSessionExpiredError,
  isUnexpectedError,
} from './errorMessage'
import { SeckillError } from './seckill'

const apiErr = (status: number, code: number, message: string, requestId?: string) =>
  new ApiError(code, message, { status, requestId })

describe('getErrorMessage', () => {
  it('ApiError 优先使用服务端 message', () => {
    expect(getErrorMessage(apiErr(409, ErrorCode.OutOfStock, '库存不足'))).toBe('库存不足')
    expect(getErrorMessage(apiErr(429, ErrorCode.CoolingDown, '操作过于频繁，请稍后再试'))).toBe(
      '操作过于频繁，请稍后再试'
    )
  })

  it('网络错误与超时给可行动的提示，不暴露英文报错', () => {
    expect(getErrorMessage(new ApiError(-1, 'Network Error', { status: 0, kind: 'network' }))).toContain('网络')
    expect(getErrorMessage(new ApiError(-1, 'timeout', { status: 0, kind: 'timeout' }))).toContain('超时')
  })

  it('空 message 时使用兜底；非 Error 值使用兜底', () => {
    expect(getErrorMessage(apiErr(500, ErrorCode.SysError, ''), '兜底')).toBe('兜底')
    expect(getErrorMessage('boom', '兜底')).toBe('兜底')
    expect(getErrorMessage(undefined)).toBe('操作失败，请稍后重试')
  })

  it('普通 Error：带 timeout / network 字样归为弱网提示，其余原样', () => {
    expect(getErrorMessage(new Error('timeout of 15000ms exceeded'))).toContain('网络')
    expect(getErrorMessage(new Error('Failed to fetch'))).toContain('网络')
    expect(getErrorMessage(new Error('具体业务原因'))).toBe('具体业务原因')
  })
})

describe('requestId / 意外错误 / 会话过期判定', () => {
  it('getRequestId 只从 ApiError 取', () => {
    expect(getRequestId(apiErr(500, 50000, 'x', 'rid-1'))).toBe('rid-1')
    expect(getRequestId(new Error('x'))).toBeUndefined()
    expect(getRequestId(apiErr(500, 50000, 'x'))).toBeUndefined()
  })

  it('只有 5xx（非 503 繁忙）与无法解析的响应算意外错误', () => {
    expect(isUnexpectedError(apiErr(500, 50000, 'x'))).toBe(true)
    expect(isUnexpectedError(apiErr(502, 502, 'x'))).toBe(true)
    for (const [s, c] of [
      [503, ErrorCode.SystemBusy],
      [409, ErrorCode.OutOfStock],
      [429, ErrorCode.RateLimited],
      [403, ErrorCode.Forbidden],
      [404, ErrorCode.CouponNotFound],
    ] as const) {
      expect(isUnexpectedError(apiErr(s, c, 'x'))).toBe(false)
    }
    expect(isUnexpectedError(new Error('x'))).toBe(false)
  })

  it('只有 401（非口令错误）算会话过期', () => {
    expect(isSessionExpiredError(apiErr(401, ErrorCode.TokenInvalid, 'x'))).toBe(true)
    expect(isSessionExpiredError(apiErr(401, ErrorCode.PasswordError, 'x'))).toBe(false)
    expect(isSessionExpiredError(apiErr(403, ErrorCode.Forbidden, 'x'))).toBe(false)
  })
})

describe('errorSeverity', () => {
  it('可预期的拒绝（409/429/503/403/404/400）是 warning，意外与网络错误是 error', () => {
    for (const [s, c] of [
      [409, ErrorCode.OutOfStock],
      [429, ErrorCode.RateLimited],
      [503, ErrorCode.SystemBusy],
      [403, ErrorCode.Forbidden],
      [404, ErrorCode.CouponNotFound],
      [400, ErrorCode.InvalidRequest],
      [401, ErrorCode.PasswordError],
    ] as const) {
      expect(errorSeverity(apiErr(s, c, 'x'))).toBe('warning')
    }
    expect(errorSeverity(apiErr(500, ErrorCode.SysError, 'x'))).toBe('error')
    expect(errorSeverity(new ApiError(-1, 'net', { status: 0 }))).toBe('error')
    expect(errorSeverity(new Error('x'))).toBe('error')
    expect(errorSeverity(undefined)).toBe('error')
  })

  it('自定义错误通过 severity 字段声明级别', () => {
    expect(errorSeverity(new SeckillError('x', { severity: 'warning' }))).toBe('warning')
    expect(errorSeverity(new SeckillError('x'))).toBe('error')
  })
})

describe('copyText', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('优先使用异步剪贴板 API', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    await expect(copyText('rid-1')).resolves.toBe(true)
    expect(writeText).toHaveBeenCalledWith('rid-1')
    vi.unstubAllGlobals()
  })

  it('剪贴板 API 失败时回退到 execCommand；都失败返回 false', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } })
    const exec = vi.fn().mockReturnValue(true)
    Object.defineProperty(document, 'execCommand', { value: exec, configurable: true })
    await expect(copyText('rid-2')).resolves.toBe(true)
    expect(exec).toHaveBeenCalledWith('copy')

    exec.mockReturnValue(false)
    await expect(copyText('rid-3')).resolves.toBe(false)
    vi.unstubAllGlobals()
  })
})
