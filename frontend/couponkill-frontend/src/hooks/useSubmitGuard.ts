import { useRef } from 'react'

/**
 * mutation 防抖：节流窗口内 + 进行中禁止重复提交。
 * 返回 true 表示本次可以继续提交。
 */
export function useSubmitGuard(throttleMs = 1000) {
  const lastClickRef = useRef(0)

  return (isPending: boolean): boolean => {
    const now = Date.now()
    if (isPending || now - lastClickRef.current < throttleMs) {
      return false
    }
    lastClickRef.current = now
    return true
  }
}
