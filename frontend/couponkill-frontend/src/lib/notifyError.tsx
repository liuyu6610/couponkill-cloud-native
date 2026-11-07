import { Button } from 'antd'
import type { MessageInstance } from 'antd/es/message/interface'
import {
  copyText,
  errorSeverity,
  getErrorMessage,
  getRequestId,
  isSessionExpiredError,
  isUnexpectedError,
} from './errorMessage'

/**
 * 统一的错误提示入口，替代各处手写 `message.error(getErrorMessage(...))`：
 * - 会话过期（401）不弹错误：apiClient 已清会话并跳转登录页，登录页会给出一次性提示；
 * - 业务拒绝 / 限流（429）/ 繁忙（503）/ 403 用 warning，意外错误（5xx）用 error；
 * - 意外错误且响应带 requestId 时附「复制请求ID」，方便报障；同时在控制台输出请求号。
 *
 * @param api  antd `App.useApp()` 里的 message
 * @param err  捕获到的错误
 * @param fallback  错误没有可用文案时的兜底
 */
export function notifyError(api: MessageInstance, err: unknown, fallback?: string): void {
  if (isSessionExpiredError(err)) return
  const text = getErrorMessage(err, fallback)
  const requestId = getRequestId(err)
  const severity = errorSeverity(err)

  if (isUnexpectedError(err) && requestId) {
    console.error('[error] 意外错误', { message: text, requestId })
    api.error({
      duration: 6,
      content: (
        <span>
          {text}（请求号 {requestId}）
          <Button
            type="link"
            size="small"
            onClick={() => {
              void copyText(requestId).then((ok) =>
                ok ? api.success('已复制请求ID') : api.warning('复制失败，请手动记录请求号')
              )
            }}
          >
            复制请求ID
          </Button>
        </span>
      ),
    })
    return
  }
  if (severity === 'warning') {
    api.warning(text)
    return
  }
  api.error(text)
}
