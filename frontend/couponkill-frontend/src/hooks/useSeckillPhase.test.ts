import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { msUntilNextPhaseBoundary, useSeckillPhase } from './useSeckillPhase'

describe('msUntilNextPhaseBoundary', () => {
  const start = 10_000
  const end = 20_000

  it('无时间窗返回 null', () => {
    expect(msUntilNextPhaseBoundary(null, end, 0)).toBeNull()
    expect(msUntilNextPhaseBoundary(start, null, 0)).toBeNull()
  })

  it('开售前返回距开售的毫秒数', () => {
    expect(msUntilNextPhaseBoundary(start, end, 4_000)).toBe(6_000)
  })

  it('时间窗内（含开售时刻与结束时刻）返回距结束的毫秒数', () => {
    expect(msUntilNextPhaseBoundary(start, end, start)).toBe(10_000)
    expect(msUntilNextPhaseBoundary(start, end, 15_000)).toBe(5_000)
    expect(msUntilNextPhaseBoundary(start, end, end)).toBe(0)
  })

  it('已结束返回 null，不再排定时器', () => {
    expect(msUntilNextPhaseBoundary(start, end, end + 1)).toBeNull()
  })
})

/** 本地时间格式化成后端 `yyyy-MM-dd HH:mm:ss` */
function backendTime(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

describe('useSeckillPhase', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('到达开售与结束时刻自动切换阶段，无需外部重渲染', () => {
    vi.useFakeTimers()
    const now = new Date(2026, 10, 2, 9, 59, 58)
    vi.setSystemTime(now)
    const coupon = {
      seckillStartAt: backendTime(new Date(now.getTime() + 2_000)),
      seckillEndAt: backendTime(new Date(now.getTime() + 5_000)),
    }
    const { result } = renderHook(() => useSeckillPhase(coupon))
    expect(result.current).toBe('upcoming')

    act(() => {
      vi.advanceTimersByTime(2_100)
    })
    expect(result.current).toBe('onsale')

    act(() => {
      vi.advanceTimersByTime(3_100)
    })
    expect(result.current).toBe('ended')
  })

  it('disabled 或无券时返回 no_window', () => {
    const { result } = renderHook(() => useSeckillPhase(null))
    expect(result.current).toBe('no_window')
    const { result: disabled } = renderHook(() =>
      useSeckillPhase({ seckillStartAt: '2026-11-02 10:00:00', seckillEndAt: '2026-11-02 11:00:00' }, false)
    )
    expect(disabled.current).toBe('no_window')
  })
})
