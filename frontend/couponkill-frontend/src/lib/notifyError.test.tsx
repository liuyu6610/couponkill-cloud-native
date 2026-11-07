import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { MessageInstance } from 'antd/es/message/interface'
import { ApiError, ErrorCode } from './apiClient'
import { notifyError } from './notifyError'
import { SeckillError } from './seckill'

function fakeMessage() {
  return {
    error: vi.fn(),
    warning: vi.fn(),
    success: vi.fn(),
    info: vi.fn(),
  } as unknown as MessageInstance & Record<'error' | 'warning' | 'success' | 'info', ReturnType<typeof vi.fn>>
}

const apiErr = (status: number, code: number, message: string, requestId?: string) =>
  new ApiError(code, message, { status, requestId })

describe('notifyError', () => {
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('会话过期（401）不弹错误：apiClient 已跳转登录页', () => {
    const m = fakeMessage()
    notifyError(m, apiErr(401, ErrorCode.TokenInvalid, '令牌无效'))
    expect(m.error).not.toHaveBeenCalled()
    expect(m.warning).not.toHaveBeenCalled()
  })

  it('409 / 429 / 503 / 403 用 warning 展示服务端文案', () => {
    for (const [status, code, text] of [
      [409, ErrorCode.OutOfStock, '库存不足'],
      [429, ErrorCode.CoolingDown, '操作过于频繁，请稍后再试'],
      [503, ErrorCode.SystemBusy, '系统繁忙，请稍后再试'],
      [403, ErrorCode.Forbidden, '无权限访问'],
    ] as const) {
      const m = fakeMessage()
      notifyError(m, apiErr(status, code, text, 'rid'))
      expect(m.warning).toHaveBeenCalledWith(text)
      expect(m.error).not.toHaveBeenCalled()
    }
  })

  it('登录口令错误（401 + 20005）展示为 warning，而不是被当作会话过期吞掉', () => {
    const m = fakeMessage()
    notifyError(m, apiErr(401, ErrorCode.PasswordError, '用户名或密码错误'))
    expect(m.warning).toHaveBeenCalledWith('用户名或密码错误')
  })

  it('意外错误（500）且有 requestId：error 里带请求号和「复制请求ID」，点击复制并提示', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    const m = fakeMessage()
    notifyError(m, apiErr(500, ErrorCode.SysError, '系统错误', 'rid-42'), '兜底')

    expect(m.error).toHaveBeenCalledTimes(1)
    const arg = m.error.mock.calls[0][0] as { content: ReactNode }
    render(<div>{arg.content}</div>)
    expect(screen.getByText(/系统错误/).textContent).toContain('rid-42')
    fireEvent.click(screen.getByRole('button', { name: '复制请求ID' }))
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith('rid-42'))
    await vi.waitFor(() => expect(m.success).toHaveBeenCalledWith('已复制请求ID'))
    // 控制台同时留下请求号，便于没有 UI 时排障
    expect(console.error).toHaveBeenCalledWith(
      '[error] 意外错误',
      expect.objectContaining({ requestId: 'rid-42' })
    )
  })

  it('意外错误没有 requestId 时只展示文案（error）', () => {
    const m = fakeMessage()
    notifyError(m, apiErr(502, 502, '服务暂时不可用，请稍后重试'))
    expect(m.error).toHaveBeenCalledWith('服务暂时不可用，请稍后重试')
  })

  it('网络错误用 error；普通 Error 用其文案；未知值用兜底', () => {
    const m = fakeMessage()
    notifyError(m, new ApiError(-1, 'Network Error', { status: 0, kind: 'network' }))
    expect(m.error).toHaveBeenCalledWith(expect.stringContaining('网络'))
    notifyError(m, new Error('具体原因'))
    expect(m.error).toHaveBeenCalledWith('具体原因')
    notifyError(m, 'str', '兜底文案')
    expect(m.error).toHaveBeenCalledWith('兜底文案')
  })

  it('SeckillError 按自带的级别展示：抢光 warning，系统错误 error', () => {
    const m = fakeMessage()
    notifyError(m, new SeckillError('手慢了，券已被抢光', { severity: 'warning' }))
    expect(m.warning).toHaveBeenCalledWith('手慢了，券已被抢光')
    notifyError(m, new SeckillError('系统开小差了'))
    expect(m.error).toHaveBeenCalledWith('系统开小差了')
  })
})
