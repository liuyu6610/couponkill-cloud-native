import { ApiError } from './apiClient'

/** 界面提示级别：业务拒绝/限流/繁忙是可预期的 warning；意外错误才是 error */
export type ErrorSeverity = 'warning' | 'error'

/** 带有自定义提示级别的错误（如秒杀业务失败 SeckillError）可通过这个字段声明级别 */
interface SeverityCarrier {
  severity?: ErrorSeverity
}

const TIMEOUT_HINT = '网络较慢或已超时，请检查网络后重试'

/** 统一可行动错误文案：服务端 message 优先；弱网/超时与业务拒绝分开说清楚 */
export function getErrorMessage(err: unknown, fallback = '操作失败，请稍后重试'): string {
  if (err instanceof ApiError) {
    if (err.kind === 'network' || err.kind === 'timeout') {
      return err.kind === 'timeout' ? '请求超时，请检查网络后重试' : TIMEOUT_HINT
    }
    return err.message || fallback
  }
  if (err instanceof Error && err.message) {
    const msg = err.message.toLowerCase()
    if (msg.includes('timeout') || msg.includes('network') || msg.includes('failed to fetch')) {
      return TIMEOUT_HINT
    }
    return err.message
  }
  return fallback
}

/** 取出错误携带的请求号（用于向后端同学/运维报障）；没有则返回 undefined */
export function getRequestId(err: unknown): string | undefined {
  return err instanceof ApiError ? err.requestId : undefined
}

/**
 * 是否「意外错误」：后端 5xx（非 503 繁忙）或无法解析的响应。
 * 这类错误用户无法自行处理，界面应给出请求号方便排障；可预期的业务拒绝/限流/繁忙不需要。
 */
export function isUnexpectedError(err: unknown): boolean {
  return err instanceof ApiError && err.kind === 'server'
}

/** 会话已过期：apiClient 已清会话并跳转登录页，界面不必再弹错误 */
export function isSessionExpiredError(err: unknown): boolean {
  return err instanceof ApiError && err.kind === 'session_expired'
}

/** 错误的提示级别 */
export function errorSeverity(err: unknown): ErrorSeverity {
  if (err instanceof ApiError) {
    switch (err.kind) {
      case 'conflict':
      case 'rate_limited':
      case 'busy':
      case 'forbidden':
      case 'not_found':
      case 'invalid':
      case 'unauthorized':
        return 'warning'
      default:
        return 'error'
    }
  }
  const declared = (err as SeverityCarrier | null)?.severity
  return declared === 'warning' ? 'warning' : 'error'
}

/** 复制文本到剪贴板；优先异步 Clipboard API，失败回退到 execCommand。返回是否成功 */
export async function copyText(text: string): Promise<boolean> {
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch (err) {
    console.warn('[clipboard] 异步复制失败，回退到 execCommand', err)
  }
  try {
    const el = document.createElement('textarea')
    el.value = text
    el.setAttribute('readonly', '')
    el.style.position = 'fixed'
    el.style.opacity = '0'
    document.body.appendChild(el)
    el.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(el)
    return ok
  } catch (err) {
    console.warn('[clipboard] 复制失败', err)
    return false
  }
}
