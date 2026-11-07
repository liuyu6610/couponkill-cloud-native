import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Mock } from 'vitest'
import { ApiError, ErrorCode } from './apiClient'
import {
  SECKILL_ENTER_ERROR_MESSAGES,
  SECKILL_MAX_POLL_ATTEMPTS,
  SECKILL_REASON_MESSAGES,
  SECKILL_UNKNOWN_REASON_MESSAGE,
  SeckillAbortedError,
  SeckillError,
  SeckillReason,
  abortableSleep,
  backoffMs,
  interpretSeckillResult,
  pollSeckillResult,
  seckillReasonMessage,
  translateEnterError,
} from './seckill'
import type { SeckillResult } from '../types/api'

describe('reason → 中文提示表', () => {
  // 与后端 internal/contracts/events.Reason 词表逐一对应；后端新增原因时这里要同步
  const backendReasons = [
    'OUT_OF_STOCK',
    'REPEAT',
    'TIMEOUT',
    'COUPON_DISABLED',
    'COUPON_NOT_FOUND',
    'LIMIT_EXCEEDED',
    'ENQUEUE_FAILED',
    'SYSTEM_ERROR',
  ]

  it('后端词表里的每个原因都有独立的中文文案', () => {
    const messages = backendReasons.map((r) => seckillReasonMessage(r))
    for (const m of messages) {
      expect(m).not.toBe(SECKILL_UNKNOWN_REASON_MESSAGE)
      expect(m).toMatch(/[一-龥]/)
    }
    expect(new Set(messages).size).toBe(backendReasons.length)
    expect(Object.keys(SECKILL_REASON_MESSAGES).sort()).toEqual([...backendReasons].sort())
  })

  it('关键文案', () => {
    expect(seckillReasonMessage('OUT_OF_STOCK')).toBe('手慢了，券已被抢光')
    expect(seckillReasonMessage('REPEAT')).toContain('重复')
    expect(seckillReasonMessage('LIMIT_EXCEEDED')).toContain('上限')
  })

  it('未知 / 空 / 大小写不符的原因给兜底文案，不显示空白或原始码', () => {
    for (const r of [undefined, null, '', 'NEW_REASON', 'out_of_stock']) {
      expect(seckillReasonMessage(r)).toBe(SECKILL_UNKNOWN_REASON_MESSAGE)
    }
  })

  it('受理阶段的业务码有用户文案', () => {
    for (const code of [
      ErrorCode.OutOfStock,
      ErrorCode.RepeatSeckill,
      ErrorCode.NotStarted,
      ErrorCode.ActivityEnded,
      ErrorCode.CouponDisabled,
      ErrorCode.CoolingDown,
      ErrorCode.RateLimited,
      ErrorCode.SystemBusy,
    ]) {
      expect(SECKILL_ENTER_ERROR_MESSAGES[code]).toBeTruthy()
    }
  })
})

describe('backoffMs', () => {
  it('从 200ms 指数翻倍，封顶 2s', () => {
    expect([0, 1, 2, 3, 4, 5, 10].map(backoffMs)).toEqual([200, 400, 800, 1600, 2000, 2000, 2000])
  })

  it('最大轮询次数下，轮询之间的总等待约 13s（超出后提示去订单页）', () => {
    let total = 0
    for (let i = 0; i < SECKILL_MAX_POLL_ATTEMPTS - 1; i++) total += backoffMs(i)
    expect(total).toBe(13_000)
  })
})

describe('interpretSeckillResult', () => {
  it('QUEUED / PROCESSING / 空 / 未知状态都视为进行中（向前兼容）', () => {
    for (const r of [
      undefined,
      null,
      { status: 'QUEUED' },
      { status: 'PROCESSING' },
      { status: 'SOMETHING_NEW' },
      { status: '' },
    ] as Array<SeckillResult | null | undefined>) {
      expect(interpretSeckillResult(r)).toEqual({ kind: 'pending' })
    }
  })

  it('SUCCESS 带 orderId（字符串原样保留，不丢精度）', () => {
    expect(interpretSeckillResult({ status: 'SUCCESS', orderId: '9007199254740993' })).toEqual({
      kind: 'success',
      orderId: '9007199254740993',
    })
    expect(interpretSeckillResult({ status: 'SUCCESS' })).toEqual({ kind: 'success', orderId: undefined })
  })

  it('FAILED 带 reason → 中文文案；缺 reason 给兜底', () => {
    expect(interpretSeckillResult({ status: 'FAILED', reason: 'OUT_OF_STOCK' })).toEqual({
      kind: 'failed',
      reason: 'OUT_OF_STOCK',
      message: '手慢了，券已被抢光',
    })
    expect(interpretSeckillResult({ status: 'FAILED' })).toMatchObject({
      kind: 'failed',
      message: SECKILL_UNKNOWN_REASON_MESSAGE,
    })
  })

  it('旧的字符串协议（SUCCESS:orderId）不再被当作成功', () => {
    expect(interpretSeckillResult('SUCCESS:1' as unknown as SeckillResult)).toEqual({ kind: 'pending' })
  })
})

describe('translateEnterError', () => {
  const api = (status: number, code: number, message: string) =>
    new ApiError(code, message, { status, requestId: 'rid' })

  it('409 / 429 / 503 转成 warning 级别的 SeckillError，使用场景文案并保留 code/requestId', () => {
    const cases: Array<[ApiError, string]> = [
      [api(409, ErrorCode.OutOfStock, '库存不足'), '手慢了，券已被抢光'],
      [api(409, ErrorCode.RepeatSeckill, '请勿重复秒杀'), '您已抢到过该券，请勿重复秒杀'],
      [api(409, ErrorCode.NotStarted, '活动尚未开始'), '活动尚未开始，请到点再来或使用「预约帮抢」'],
      [api(429, ErrorCode.CoolingDown, '冷却'), '操作过于频繁，请稍后再试'],
      [api(503, ErrorCode.SystemBusy, '繁忙'), '系统繁忙，请稍后再试'],
    ]
    for (const [err, text] of cases) {
      const out = translateEnterError(err)
      expect(out).toBeInstanceOf(SeckillError)
      expect((out as SeckillError).message).toBe(text)
      expect((out as SeckillError).severity).toBe('warning')
      expect((out as SeckillError).code).toBe(err.code)
      expect((out as SeckillError).requestId).toBe('rid')
    }
  })

  it('表外的 409 业务码沿用服务端 message', () => {
    const out = translateEnterError(api(409, ErrorCode.CouponLimitExceeded, '已超过领取上限'))
    expect((out as SeckillError).message).toBe('已超过领取上限')
  })

  it('401 / 403 / 网络 / 500 / 普通 Error 原样返回，不吞掉', () => {
    for (const e of [
      api(401, ErrorCode.TokenInvalid, 'x'),
      api(403, ErrorCode.Forbidden, 'x'),
      api(500, ErrorCode.SysError, 'x'),
      new ApiError(-1, 'net', { status: 0 }),
      new Error('boom'),
    ]) {
      expect(translateEnterError(e)).toBe(e)
    }
  })
})

describe('pollSeckillResult 轮询与停止条件', () => {
  const REQ = 'req-1'
  let controller: AbortController
  let sleep: Mock<(ms: number, signal: AbortSignal) => Promise<void>>

  beforeEach(() => {
    controller = new AbortController()
    sleep = vi.fn<(ms: number, signal: AbortSignal) => Promise<void>>(async () => {})
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  /** 依次返回给定结果；Error 实例则抛出；用完后一直返回最后一个 */
  function script(...steps: Array<SeckillResult | Error>) {
    let i = 0
    return vi.fn(async () => {
      const step = steps[Math.min(i++, steps.length - 1)]
      if (step instanceof Error) throw step
      return step
    })
  }

  const run = (fetchResult: (s: AbortSignal) => Promise<SeckillResult>, maxAttempts?: number) =>
    pollSeckillResult({ fetchResult, signal: controller.signal, requestId: REQ, sleep, maxAttempts })

  it('QUEUED → PROCESSING → SUCCESS：返回 orderId，按退避间隔等待', async () => {
    const fetchResult = script(
      { status: 'QUEUED' },
      { status: 'PROCESSING' },
      { status: 'SUCCESS', orderId: '123456789012345678' }
    )
    await expect(run(fetchResult)).resolves.toEqual({ orderId: '123456789012345678' })
    expect(fetchResult).toHaveBeenCalledTimes(3)
    expect(sleep.mock.calls.map((c) => c[0])).toEqual([200, 400])
    expect(fetchResult).toHaveBeenCalledWith(controller.signal)
  })

  it('首次即 SUCCESS 不等待', async () => {
    await run(script({ status: 'SUCCESS', orderId: '1' }))
    expect(sleep).not.toHaveBeenCalled()
  })

  it('FAILED：立即停止，抛带 reason 文案的 SeckillError；抢光/重复为 warning，系统类为 error', async () => {
    const cases: Array<[string, string, 'warning' | 'error']> = [
      [SeckillReason.OUT_OF_STOCK, '手慢了，券已被抢光', 'warning'],
      [SeckillReason.REPEAT, '您已抢到过该券，请勿重复秒杀', 'warning'],
      [SeckillReason.SYSTEM_ERROR, SECKILL_REASON_MESSAGES.SYSTEM_ERROR, 'error'],
      [SeckillReason.TIMEOUT, SECKILL_REASON_MESSAGES.TIMEOUT, 'error'],
    ]
    for (const [reason, text, severity] of cases) {
      const fetchResult = script({ status: 'PROCESSING' }, { status: 'FAILED', reason })
      const err = await run(fetchResult).catch((e: unknown) => e)
      expect(err).toBeInstanceOf(SeckillError)
      expect((err as SeckillError).message).toBe(text)
      expect((err as SeckillError).reason).toBe(reason)
      expect((err as SeckillError).severity).toBe(severity)
      expect((err as SeckillError).requestId).toBe(REQ)
      expect(fetchResult).toHaveBeenCalledTimes(2)
    }
  })

  it('一直进行中：恰好轮询 maxAttempts 次、之间等待 maxAttempts-1 次，然后抛 POLL_TIMEOUT', async () => {
    const fetchResult = script({ status: 'QUEUED' })
    const err = await run(fetchResult).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(SeckillError)
    expect((err as SeckillError).reason).toBe('POLL_TIMEOUT')
    expect((err as SeckillError).message).toContain('我的订单')
    expect((err as SeckillError).severity).toBe('warning')
    expect(fetchResult).toHaveBeenCalledTimes(SECKILL_MAX_POLL_ATTEMPTS)
    expect(sleep).toHaveBeenCalledTimes(SECKILL_MAX_POLL_ATTEMPTS - 1)
  })

  it('不认识的状态按进行中处理，仍受轮询上限约束', async () => {
    const fetchResult = script({ status: 'FUTURE_STATE' })
    await expect(run(fetchResult, 3)).rejects.toMatchObject({ reason: 'POLL_TIMEOUT' })
    expect(fetchResult).toHaveBeenCalledTimes(3)
  })

  it('暂时性错误（网络 / 超时 / 503 / 500 / 429）当作进行中继续重试，恢复后拿到结果', async () => {
    const fetchResult = script(
      new ApiError(-1, 'net', { status: 0, kind: 'network' }),
      new ApiError(-1, 'timeout', { status: 0, kind: 'timeout' }),
      new ApiError(ErrorCode.SystemBusy, 'busy', { status: 503 }),
      new ApiError(ErrorCode.SysError, 'oops', { status: 500 }),
      new ApiError(ErrorCode.RateLimited, 'slow down', { status: 429 }),
      { status: 'SUCCESS', orderId: '7' }
    )
    await expect(run(fetchResult)).resolves.toEqual({ orderId: '7' })
    expect(fetchResult).toHaveBeenCalledTimes(6)
  })

  it('429 带 Retry-After 时等待取 max(退避, Retry-After)，且最多采纳 5s', async () => {
    const fetchResult = script(
      new ApiError(ErrorCode.RateLimited, 'x', { status: 429, retryAfterSec: 3 }),
      new ApiError(ErrorCode.RateLimited, 'x', { status: 429, retryAfterSec: 60 }),
      new ApiError(ErrorCode.RateLimited, 'x', { status: 429, retryAfterSec: 1 }),
      { status: 'SUCCESS', orderId: '1' }
    )
    await run(fetchResult)
    // 第 0、1、2 次的退避为 200/400/800ms
    expect(sleep.mock.calls.map((c) => c[0])).toEqual([3000, 5000, 1000])
  })

  it('401（会话失效）立即停止并原样抛出，不再重试', async () => {
    const expired = new ApiError(ErrorCode.TokenInvalid, '令牌无效', { status: 401 })
    const fetchResult = script({ status: 'QUEUED' }, expired)
    await expect(run(fetchResult)).rejects.toBe(expired)
    expect(fetchResult).toHaveBeenCalledTimes(2)
  })

  it('403 / 400 立即停止并原样抛出', async () => {
    for (const status of [403, 400]) {
      const err = new ApiError(status === 403 ? ErrorCode.Forbidden : ErrorCode.InvalidRequest, 'x', { status })
      const fetchResult = script(err)
      await expect(run(fetchResult)).rejects.toBe(err)
      expect(fetchResult).toHaveBeenCalledTimes(1)
    }
  })

  it('404（非属主 / 请求不存在）立即停止，转成引导去订单页确认的 warning', async () => {
    const fetchResult = script(new ApiError(ErrorCode.OrderNotFound, 'nf', { status: 404, requestId: 'r404' }))
    const err = await run(fetchResult).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(SeckillError)
    expect((err as SeckillError).reason).toBe('NOT_FOUND')
    expect((err as SeckillError).severity).toBe('warning')
    expect((err as SeckillError).requestId).toBe('r404')
    expect(fetchResult).toHaveBeenCalledTimes(1)
  })

  it('已中止的 signal：不发任何请求，抛 SeckillAbortedError', async () => {
    const fetchResult = script({ status: 'QUEUED' })
    controller.abort()
    await expect(run(fetchResult)).rejects.toBeInstanceOf(SeckillAbortedError)
    expect(fetchResult).not.toHaveBeenCalled()
  })

  it('请求在途时中止（底层请求被取消）→ SeckillAbortedError，而不是当作暂时性错误继续重试', async () => {
    const fetchResult = vi.fn(async () => {
      controller.abort()
      throw new Error('canceled')
    })
    await expect(run(fetchResult)).rejects.toBeInstanceOf(SeckillAbortedError)
    expect(fetchResult).toHaveBeenCalledTimes(1)
    expect(sleep).not.toHaveBeenCalled()
  })

  it('响应返回时已被中止：丢弃结果，不当作成功', async () => {
    const fetchResult = vi.fn(async () => {
      controller.abort()
      return { status: 'SUCCESS', orderId: '1' } as SeckillResult
    })
    await expect(run(fetchResult)).rejects.toBeInstanceOf(SeckillAbortedError)
  })

  it('退避等待期间被中止：sleep 的中止错误向上传播，不再发起下一次请求', async () => {
    const fetchResult = script({ status: 'QUEUED' })
    const abortingSleep = vi.fn(async () => {
      controller.abort()
      throw new SeckillAbortedError()
    })
    await expect(
      pollSeckillResult({ fetchResult, signal: controller.signal, requestId: REQ, sleep: abortingSleep })
    ).rejects.toBeInstanceOf(SeckillAbortedError)
    expect(fetchResult).toHaveBeenCalledTimes(1)
  })

  it('非 ApiError 的意外异常（编程错误）不被吞掉', async () => {
    const boom = new TypeError('boom')
    await expect(run(script(boom))).rejects.toBe(boom)
  })
})

describe('abortableSleep', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('到时间 resolve', async () => {
    vi.useFakeTimers()
    const c = new AbortController()
    const done = vi.fn()
    void abortableSleep(500, c.signal).then(done)
    await vi.advanceTimersByTimeAsync(499)
    expect(done).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(done).toHaveBeenCalled()
  })

  it('中止时立即 reject 并清掉定时器；已中止的 signal 直接 reject', async () => {
    vi.useFakeTimers()
    const c = new AbortController()
    const p = abortableSleep(10_000, c.signal)
    const settled = p.catch((e: unknown) => e)
    c.abort()
    expect(await settled).toBeInstanceOf(SeckillAbortedError)
    expect(vi.getTimerCount()).toBe(0)
    await expect(abortableSleep(1, c.signal)).rejects.toBeInstanceOf(SeckillAbortedError)
  })
})
