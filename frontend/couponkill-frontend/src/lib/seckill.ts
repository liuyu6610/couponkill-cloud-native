import { ApiError, ErrorCode } from './apiClient'
import type { ErrorSeverity } from './errorMessage'
import type { SeckillResult } from '../types/api'

/** 秒杀请求状态词表（对齐后端 events.Status，大小写敏感） */
export const SeckillStatus = {
  QUEUED: 'QUEUED',
  PROCESSING: 'PROCESSING',
  SUCCESS: 'SUCCESS',
  FAILED: 'FAILED',
} as const

/** 失败原因词表（对齐后端 events.Reason，大小写敏感） */
export const SeckillReason = {
  OUT_OF_STOCK: 'OUT_OF_STOCK',
  REPEAT: 'REPEAT',
  TIMEOUT: 'TIMEOUT',
  COUPON_DISABLED: 'COUPON_DISABLED',
  COUPON_NOT_FOUND: 'COUPON_NOT_FOUND',
  LIMIT_EXCEEDED: 'LIMIT_EXCEEDED',
  ENQUEUE_FAILED: 'ENQUEUE_FAILED',
  SYSTEM_ERROR: 'SYSTEM_ERROR',
} as const

/** 失败原因 → 用户可读文案 */
export const SECKILL_REASON_MESSAGES: Record<string, string> = {
  [SeckillReason.OUT_OF_STOCK]: '手慢了，券已被抢光',
  [SeckillReason.REPEAT]: '您已抢到过该券，请勿重复秒杀',
  [SeckillReason.TIMEOUT]: '秒杀处理超时，已为您回滚，请稍后重试',
  [SeckillReason.COUPON_DISABLED]: '该优惠券已停用',
  [SeckillReason.COUPON_NOT_FOUND]: '该优惠券不存在或已下架',
  [SeckillReason.LIMIT_EXCEEDED]: '已超过该券的领取上限',
  [SeckillReason.ENQUEUE_FAILED]: '系统繁忙，秒杀请求未能受理，请稍后重试',
  [SeckillReason.SYSTEM_ERROR]: '系统开小差了，秒杀未成功，请稍后重试',
}

/** 未知失败原因的兜底文案（后端新增了词表之外的原因时不至于显示空白） */
export const SECKILL_UNKNOWN_REASON_MESSAGE = '秒杀未成功，请稍后重试'

/** 可预期的业务失败（抢光/重复/限购/停用）用 warning；超时与系统类原因用 error */
const REASON_SEVERITY: Record<string, ErrorSeverity> = {
  [SeckillReason.OUT_OF_STOCK]: 'warning',
  [SeckillReason.REPEAT]: 'warning',
  [SeckillReason.LIMIT_EXCEEDED]: 'warning',
  [SeckillReason.COUPON_DISABLED]: 'warning',
  [SeckillReason.COUPON_NOT_FOUND]: 'warning',
}

/** 失败原因 → 中文提示；空值与未知原因给兜底文案 */
export function seckillReasonMessage(reason?: string | null): string {
  if (!reason) return SECKILL_UNKNOWN_REASON_MESSAGE
  return SECKILL_REASON_MESSAGES[reason] ?? SECKILL_UNKNOWN_REASON_MESSAGE
}

/**
 * 秒杀受理阶段（POST /order/seckill）被拒绝的业务码 → 用户文案。
 * 这些码都对应 409/429/503，服务端 message 也可读，这里统一成更贴近场景的说法；表外的码沿用服务端 message。
 */
export const SECKILL_ENTER_ERROR_MESSAGES: Record<number, string> = {
  [ErrorCode.OutOfStock]: '手慢了，券已被抢光',
  [ErrorCode.RepeatSeckill]: '您已抢到过该券，请勿重复秒杀',
  [ErrorCode.NotStarted]: '活动尚未开始，请到点再来或使用「预约帮抢」',
  [ErrorCode.ActivityEnded]: '本场活动已结束',
  [ErrorCode.CouponDisabled]: '该优惠券已停用',
  [ErrorCode.CoolingDown]: '操作过于频繁，请稍后再试',
  [ErrorCode.RateLimited]: '请求过于频繁，请稍后再试',
  [ErrorCode.SystemBusy]: '系统繁忙，请稍后再试',
  [ErrorCode.NotPreheated]: '活动库存尚未就绪，请稍后再试',
}

/** 秒杀流程里「用户可理解」的失败：受理被拒、处理终态失败、轮询超时。携带提示级别与原因 */
export class SeckillError extends Error {
  /** 终态失败原因（FAILED 时），或 POLL_TIMEOUT / NOT_FOUND 等前端自定义原因 */
  reason?: string
  /** 受理阶段被拒时的业务码 */
  code?: number
  requestId?: string
  severity: ErrorSeverity

  constructor(message: string, init: { reason?: string; code?: number; requestId?: string; severity?: ErrorSeverity } = {}) {
    super(message)
    this.name = 'SeckillError'
    this.reason = init.reason
    this.code = init.code
    this.requestId = init.requestId
    this.severity = init.severity ?? 'error'
  }
}

/** 组件卸载/重复发起时中止轮询抛出的错误；调用方无需提示用户 */
export class SeckillAbortedError extends Error {
  constructor() {
    super('秒杀结果轮询已中止')
    this.name = 'SeckillAbortedError'
  }
}

/**
 * 受理阶段的错误翻译：409/429/503 这类可预期拒绝转成带 warning 级别的 SeckillError；
 * 401/403/网络等其它错误原样返回（401 已由 apiClient 处理会话）。
 */
export function translateEnterError(err: unknown): unknown {
  if (!(err instanceof ApiError)) return err
  const predictable = err.kind === 'conflict' || err.kind === 'rate_limited' || err.kind === 'busy'
  if (!predictable) return err
  return new SeckillError(SECKILL_ENTER_ERROR_MESSAGES[err.code] ?? err.message, {
    code: err.code,
    requestId: err.requestId,
    severity: 'warning',
  })
}

/** 一次 /seckill/result 响应的解读 */
export type SeckillOutcome =
  | { kind: 'pending' }
  | { kind: 'success'; orderId?: string }
  | { kind: 'failed'; reason: string; message: string }

/**
 * 解读 /seckill/result 的 data：
 * - SUCCESS → 成功（带 orderId）；FAILED → 失败（reason 映射成中文）；
 * - QUEUED / PROCESSING 以及任何不认识的状态（后端新增词表时的向前兼容）→ 进行中，由轮询上限兜底。
 */
export function interpretSeckillResult(result: SeckillResult | null | undefined): SeckillOutcome {
  if (!result || typeof result !== 'object') return { kind: 'pending' }
  if (result.status === SeckillStatus.SUCCESS) {
    return { kind: 'success', orderId: result.orderId ? String(result.orderId) : undefined }
  }
  if (result.status === SeckillStatus.FAILED) {
    const reason = result.reason ?? ''
    return { kind: 'failed', reason, message: seckillReasonMessage(reason) }
  }
  return { kind: 'pending' }
}

/** 把终态失败转成 SeckillError，供 mutation 抛出 */
export function seckillFailure(outcome: { reason: string; message: string }, requestId?: string): SeckillError {
  return new SeckillError(outcome.message, {
    reason: outcome.reason,
    requestId,
    severity: REASON_SEVERITY[outcome.reason] ?? 'error',
  })
}

/** 结果轮询最大次数：配合退避总等待约 13s，之后提示去订单页确认 */
export const SECKILL_MAX_POLL_ATTEMPTS = 10

/** 指数退避：200ms → 400 → 800 → … 上限 2s */
export function backoffMs(attempt: number): number {
  return Math.min(200 * 2 ** attempt, 2000)
}

/** 轮询中遇到这些错误立刻停止：重试没有意义（会话失效、无权限、请求不存在、参数错误） */
function isFatalPollError(err: ApiError): boolean {
  return (
    err.kind === 'session_expired' ||
    err.kind === 'unauthorized' ||
    err.kind === 'forbidden' ||
    err.kind === 'not_found' ||
    err.kind === 'invalid'
  )
}

/** 429 的 Retry-After 最多采纳 5 秒，避免一个异常头把轮询拖死 */
const MAX_RETRY_AFTER_MS = 5000

export interface PollOptions {
  /** 查询一次结果；应把 signal 透传给底层请求，卸载时才能真正取消在途 GET */
  fetchResult: (signal: AbortSignal) => Promise<SeckillResult>
  signal: AbortSignal
  requestId: string
  /** 可注入的 sleep（测试用）；默认可被 signal 打断的定时器 */
  sleep?: (ms: number, signal: AbortSignal) => Promise<void>
  maxAttempts?: number
}

/** 可被 AbortSignal 打断的 sleep：卸载后不再挂着定时器 */
export function abortableSleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new SeckillAbortedError())
      return
    }
    const timer = window.setTimeout(() => {
      signal.removeEventListener('abort', onAbort)
      resolve()
    }, ms)
    const onAbort = () => {
      window.clearTimeout(timer)
      reject(new SeckillAbortedError())
    }
    signal.addEventListener('abort', onAbort, { once: true })
  })
}

/**
 * 有界退避轮询秒杀结果，直到终态或停止条件：
 * - SUCCESS → 返回 orderId；FAILED → 抛 SeckillError(reason 文案)；
 * - 暂时性错误（网络/超时/429/503/5xx）当作进行中继续重试，429 带 Retry-After 时多等一会；
 * - 致命错误（401/403/404/400）立即停止：404 转 SeckillError（请求不存在或已过期），其余原样抛出；
 * - 达到 maxAttempts 仍未终态 → 抛 SeckillError(POLL_TIMEOUT)，提示去「我的订单」确认（订单可能已落库）；
 * - signal 中止 → 抛 SeckillAbortedError（含正在 sleep 与正在请求的两种时刻）。
 */
export async function pollSeckillResult(opts: PollOptions): Promise<{ orderId?: string }> {
  const { fetchResult, signal, requestId } = opts
  const sleep = opts.sleep ?? abortableSleep
  const maxAttempts = opts.maxAttempts ?? SECKILL_MAX_POLL_ATTEMPTS

  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    if (signal.aborted) throw new SeckillAbortedError()
    let extraWaitMs = 0
    try {
      const outcome = interpretSeckillResult(await fetchResult(signal))
      if (signal.aborted) throw new SeckillAbortedError()
      if (outcome.kind === 'success') return { orderId: outcome.orderId }
      if (outcome.kind === 'failed') throw seckillFailure(outcome, requestId)
    } catch (err) {
      if (err instanceof SeckillError || err instanceof SeckillAbortedError) throw err
      if (signal.aborted) throw new SeckillAbortedError()
      if (!(err instanceof ApiError)) throw err
      if (err.kind === 'not_found') {
        throw new SeckillError('秒杀请求不存在或已过期，请到「我的订单」确认是否已领取', {
          reason: 'NOT_FOUND',
          code: err.code,
          requestId: err.requestId ?? requestId,
          severity: 'warning',
        })
      }
      if (isFatalPollError(err)) throw err
      // 暂时性错误：弱网 / 限流 / 繁忙 / 5xx，按进行中处理继续退避重试
      console.warn('[seckill] 结果查询失败，稍后重试', { requestId, attempt, status: err.status, code: err.code })
      if (err.kind === 'rate_limited' && err.retryAfterSec) {
        extraWaitMs = Math.min(err.retryAfterSec * 1000, MAX_RETRY_AFTER_MS)
      }
    }
    if (attempt < maxAttempts - 1) {
      await sleep(Math.max(backoffMs(attempt), extraWaitMs), signal)
    }
  }
  throw new SeckillError('秒杀受理中，订单尚未确认，请稍后在「我的订单」查看', {
    reason: 'POLL_TIMEOUT',
    requestId,
    severity: 'warning',
  })
}
