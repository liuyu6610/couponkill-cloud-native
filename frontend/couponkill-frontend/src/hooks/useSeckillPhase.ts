import { useEffect, useReducer } from 'react'
import { parseBackendDateTime, seckillWindowPhase } from '../types/api'
import type { Coupon } from '../types/api'

/** setTimeout 上限约 24.8 天；远期窗口按 1 小时分段重算，避免溢出立即触发 */
const MAX_TIMER_MS = 60 * 60 * 1000
/** 越过边界后多等一点，确保 Date.now() 已落在新阶段 */
const BOUNDARY_SLACK_MS = 50

/**
 * 距下一个阶段边界（开售 / 结束）的毫秒数；已结束或无时间窗返回 null。
 * 纯函数，便于单测与复用。
 */
export function msUntilNextPhaseBoundary(
  start: number | null,
  end: number | null,
  now: number = Date.now()
): number | null {
  if (start == null || end == null) return null
  if (now < start) return start - now
  if (now <= end) return end - now
  return null
}

/**
 * 秒杀时间窗阶段（no_window / upcoming / onsale / ended），到达开售或结束时刻自动重渲染。
 *
 * 之前卡片/详情页只在渲染时算一次 phase，倒计时组件内部 tick 不会带动父组件，
 * 导致到点后按钮仍停在「预约帮抢」、结束后仍可点「立即秒杀」，必须手动刷新。
 */
export function useSeckillPhase(
  coupon: Pick<Coupon, 'seckillStartAt' | 'seckillEndAt'> | null | undefined,
  enabled = true
): ReturnType<typeof seckillWindowPhase> {
  const [tick, bump] = useReducer((x: number) => x + 1, 0)
  const start = parseBackendDateTime(coupon?.seckillStartAt)
  const end = parseBackendDateTime(coupon?.seckillEndAt)

  useEffect(() => {
    if (!enabled) return
    const remain = msUntilNextPhaseBoundary(start, end)
    if (remain == null) return
    const timer = window.setTimeout(bump, Math.min(remain + BOUNDARY_SLACK_MS, MAX_TIMER_MS))
    return () => window.clearTimeout(timer)
  }, [enabled, start, end, tick])

  if (!coupon || !enabled) return 'no_window'
  return seckillWindowPhase(coupon)
}
