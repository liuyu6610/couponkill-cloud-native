import { ApiError } from './apiClient'

/** 统一可行动错误文案：弱网/超时/业务拒绝分开说清楚 */
export function getErrorMessage(err: unknown, fallback = '操作失败，请稍后重试'): string {
  if (err instanceof ApiError) {
    const msg = (err.message || '').toLowerCase()
    if (
      err.code === -1 ||
      msg.includes('timeout') ||
      msg.includes('network') ||
      msg.includes('网络') ||
      msg.includes('econnaborted')
    ) {
      return '网络较慢或已超时，请检查网络后重试'
    }
    return err.message || fallback
  }
  if (err instanceof Error && err.message) {
    const msg = err.message.toLowerCase()
    if (msg.includes('timeout') || msg.includes('network') || msg.includes('failed to fetch')) {
      return '网络较慢或已超时，请检查网络后重试'
    }
    return err.message
  }
  return fallback
}
