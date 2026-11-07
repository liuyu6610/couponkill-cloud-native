import type { ReactNode } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, ErrorCode } from '../lib/apiClient'
import { SeckillAbortedError, SeckillError } from '../lib/seckill'
import type { SeckillAccepted } from '../types/api'
import { seckillStageText, useSeckill } from './useSeckill'

const api = vi.hoisted(() => ({
  seckill: vi.fn(),
  seckillResult: vi.fn(),
}))

vi.mock('../services/orderService', () => ({
  orderService: api,
  default: api,
}))

describe('seckillStageText', () => {
  it('只有进行中阶段才覆盖按钮文案', () => {
    expect(seckillStageText('idle')).toBeUndefined()
    expect(seckillStageText('submitting')).toBe('提交中…')
    expect(seckillStageText('queued')).toBe('排队确认中…')
  })
})

/** 可在测试里手动 resolve 的 Promise */
function deferred<T>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

const accepted = (requestId: string): SeckillAccepted => ({ requestId, status: 'QUEUED' })

describe('useSeckill', () => {
  let client: QueryClient

  beforeEach(() => {
    client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
    api.seckill.mockReset()
    api.seckillResult.mockReset()
    client.clear()
  })

  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )

  it('202 受理后轮询到 SUCCESS：返回 requestId + orderId，阶段 submitting → queued → idle', async () => {
    const enter = deferred<SeckillAccepted>()
    api.seckill.mockReturnValueOnce(enter.promise)
    api.seckillResult.mockResolvedValueOnce({ status: 'SUCCESS', orderId: '9007199254740993' })
    const { result } = renderHook(() => useSeckill(), { wrapper })

    let run!: Promise<unknown>
    act(() => {
      run = result.current.mutateAsync({ couponId: 'c1' })
    })
    await waitFor(() => expect(result.current.stage).toBe('submitting'))

    await act(async () => {
      enter.resolve(accepted('r1'))
      await expect(run).resolves.toEqual({ requestId: 'r1', orderId: '9007199254740993' })
    })
    expect(api.seckill).toHaveBeenCalledWith('c1')
    expect(api.seckillResult).toHaveBeenCalledWith('r1', expect.any(AbortSignal))
    expect(result.current.stage).toBe('idle')
  })

  it('终态 FAILED：按 reason 抛带中文文案的 SeckillError，并失效券/订单缓存', async () => {
    api.seckill.mockResolvedValueOnce(accepted('r2'))
    api.seckillResult.mockResolvedValueOnce({ status: 'FAILED', reason: 'OUT_OF_STOCK' })
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    const { result } = renderHook(() => useSeckill(), { wrapper })

    await act(async () => {
      const err = await result.current.mutateAsync({ couponId: 'c1' }).catch((e: unknown) => e)
      expect(err).toBeInstanceOf(SeckillError)
      expect((err as SeckillError).message).toBe('手慢了，券已被抢光')
      expect((err as SeckillError).severity).toBe('warning')
    })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['coupons'] })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['orders'] })
    expect(result.current.stage).toBe('idle')
  })

  it('受理被拒（409 售罄/重复、429 冷却、503 繁忙）：不进入轮询，转成 warning 级 SeckillError', async () => {
    const cases: Array<[ApiError, string]> = [
      [new ApiError(ErrorCode.OutOfStock, '库存不足', { status: 409 }), '手慢了，券已被抢光'],
      [new ApiError(ErrorCode.RepeatSeckill, '请勿重复秒杀', { status: 409 }), '您已抢到过该券，请勿重复秒杀'],
      [new ApiError(ErrorCode.CoolingDown, '冷却', { status: 429 }), '操作过于频繁，请稍后再试'],
      [new ApiError(ErrorCode.SystemBusy, '繁忙', { status: 503 }), '系统繁忙，请稍后再试'],
    ]
    for (const [rejection, text] of cases) {
      api.seckill.mockRejectedValueOnce(rejection)
      const { result, unmount } = renderHook(() => useSeckill(), { wrapper })
      await act(async () => {
        const err = await result.current.mutateAsync({ couponId: 'c1' }).catch((e: unknown) => e)
        expect(err).toBeInstanceOf(SeckillError)
        expect((err as SeckillError).message).toBe(text)
        expect((err as SeckillError).severity).toBe('warning')
      })
      unmount()
    }
    expect(api.seckillResult).not.toHaveBeenCalled()
  })

  it('受理阶段 401 / 403 / 网络错误原样抛出（401 的跳转由 apiClient 负责）', async () => {
    const expired = new ApiError(ErrorCode.TokenInvalid, '令牌无效', { status: 401 })
    api.seckill.mockRejectedValueOnce(expired)
    const { result } = renderHook(() => useSeckill(), { wrapper })
    await act(async () => {
      await expect(result.current.mutateAsync({ couponId: 'c1' })).rejects.toBe(expired)
    })
    expect(api.seckillResult).not.toHaveBeenCalled()
  })

  it('202 却缺少 requestId：不轮询，提示去订单页确认', async () => {
    api.seckill.mockResolvedValueOnce({ status: 'QUEUED' })
    const { result } = renderHook(() => useSeckill(), { wrapper })
    await act(async () => {
      const err = await result.current.mutateAsync({ couponId: 'c1' }).catch((e: unknown) => e)
      expect(err).toBeInstanceOf(SeckillError)
      expect((err as SeckillError).message).toContain('我的订单')
    })
    expect(api.seckillResult).not.toHaveBeenCalled()
  })

  it('新一轮发起后，被中止的旧轮次收尾不会把新一轮的阶段覆盖回 idle', async () => {
    const first = deferred<SeckillAccepted>()
    const second = deferred<SeckillAccepted>()
    api.seckill.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const { result } = renderHook(() => useSeckill(), { wrapper })

    let firstRun!: Promise<unknown>
    act(() => {
      firstRun = result.current.mutateAsync({ couponId: 'c1' })
    })
    await waitFor(() => expect(result.current.stage).toBe('submitting'))

    // 第二轮发起：中止第一轮
    act(() => {
      void result.current.mutateAsync({ couponId: 'c2' }).catch(() => {})
    })
    await waitFor(() => expect(api.seckill).toHaveBeenCalledTimes(2))

    // 第一轮的入队请求晚到：发现已被中止 → 抛 SeckillAbortedError 并走完 onSettled
    await act(async () => {
      first.resolve(accepted('old'))
      await expect(firstRun).rejects.toBeInstanceOf(SeckillAbortedError)
    })

    // 第二轮仍在入队中，按钮文案不能回落到默认
    expect(result.current.stage).toBe('submitting')
    expect(api.seckillResult).not.toHaveBeenCalledWith('old', expect.anything())
  })

  it('组件卸载中止轮询：在途的结果请求拿到的 signal 被 abort，且不再发起新请求', async () => {
    api.seckill.mockResolvedValueOnce(accepted('r3'))
    const inFlight = deferred<never>()
    let seenSignal: AbortSignal | undefined
    api.seckillResult.mockImplementation((_id: string, signal?: AbortSignal) => {
      seenSignal = signal
      return inFlight.promise
    })
    const { result, unmount } = renderHook(() => useSeckill(), { wrapper })

    let run!: Promise<unknown>
    act(() => {
      run = result.current.mutateAsync({ couponId: 'c1' }).catch((e: unknown) => e)
    })
    await waitFor(() => expect(api.seckillResult).toHaveBeenCalledTimes(1))
    expect(seenSignal?.aborted).toBe(false)

    unmount()
    expect(seenSignal?.aborted).toBe(true)
    // 底层请求被取消后抛错：不能被当成暂时性错误继续重试
    inFlight.resolve(undefined as never)
    expect(await run).toBeInstanceOf(SeckillAbortedError)
    expect(api.seckillResult).toHaveBeenCalledTimes(1)
  })
})
