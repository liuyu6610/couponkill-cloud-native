/**
 * 从 JWT 载荷里读取 roles（仅用于界面展示/路由守卫的提示，不做验签——真正的鉴权在网关与后端）。
 * 令牌格式不对、载荷不是 JSON、roles 缺失或类型不对时返回空数组，不抛异常。
 */
export function rolesFromJwt(token: string | null | undefined): string[] {
  if (!token) return []
  const parts = token.split('.')
  if (parts.length !== 3) return []
  try {
    const b64 = parts[1].replace(/-/g, '+').replace(/_/g, '/')
    const padded = b64 + '='.repeat((4 - (b64.length % 4)) % 4)
    const bytes = Uint8Array.from(atob(padded), (c) => c.charCodeAt(0))
    const payload = JSON.parse(new TextDecoder().decode(bytes)) as { roles?: unknown }
    return Array.isArray(payload.roles) ? payload.roles.map(String) : []
  } catch {
    return []
  }
}
