import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, ErrorCode, STORAGE_KEYS } from '../../lib/apiClient'
import { store } from '../index'
import { loginUser, logout, selectIsAdmin } from './authSlice'

const svc = vi.hoisted(() => ({
  login: vi.fn(),
  register: vi.fn(),
  logout: vi.fn(),
  getProfile: vi.fn(),
}))

vi.mock('../../services/authService', () => ({ authService: svc, default: svc }))

/** 构造 JWT（只有载荷有意义） */
function jwtWithRoles(roles: string[]): string {
  const b64url = (s: string) => btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
  return `${b64url('{"alg":"HS256"}')}.${b64url(JSON.stringify({ sub: '1', roles }))}.sig`
}

describe('authSlice 登录与角色', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
    store.dispatch(logout())
  })

  it('角色以登录响应的 roles 为准，ID 保持字符串，并写入本地会话', async () => {
    svc.login.mockResolvedValueOnce({
      token: 'tok',
      userId: '9007199254740993',
      username: 'admin',
      roles: ['admin'],
    })
    await store.dispatch(loginUser({ username: 'admin', password: 'admin1234' }))
    const state = store.getState()
    expect(state.auth.user).toEqual({ id: '9007199254740993', username: 'admin', roles: ['admin'] })
    expect(selectIsAdmin(state)).toBe(true)
    expect(localStorage.getItem(STORAGE_KEYS.userId)).toBe('9007199254740993')
    expect(localStorage.getItem(STORAGE_KEYS.roles)).toBe('["admin"]')
  })

  it('响应没带 roles 时回退读 JWT 载荷里的 roles', async () => {
    svc.login.mockResolvedValueOnce({ token: jwtWithRoles(['admin']), userId: '1', username: 'a' })
    await store.dispatch(loginUser({ username: 'a', password: 'secret1' }))
    expect(selectIsAdmin(store.getState())).toBe(true)
  })

  it('普通用户不是管理员', async () => {
    svc.login.mockResolvedValueOnce({ token: 't', userId: '1', username: 'demo', roles: ['user'] })
    await store.dispatch(loginUser({ username: 'demo', password: 'demo1234' }))
    expect(selectIsAdmin(store.getState())).toBe(false)
  })

  it('登录失败（401 + 20005）：展示服务端文案，不写会话', async () => {
    svc.login.mockRejectedValueOnce(
      new ApiError(ErrorCode.PasswordError, '用户名或密码错误', { status: 401 })
    )
    await store.dispatch(loginUser({ username: 'demo', password: 'bad-pass' }))
    expect(store.getState().auth.error).toBe('用户名或密码错误')
    expect(store.getState().auth.isAuthenticated).toBe(false)
    expect(localStorage.getItem(STORAGE_KEYS.token)).toBeNull()
  })

  it('退出登录调用服务端吊销并清空状态', async () => {
    svc.login.mockResolvedValueOnce({ token: 't', userId: '1', username: 'demo', roles: ['user'] })
    await store.dispatch(loginUser({ username: 'demo', password: 'demo1234' }))
    store.dispatch(logout())
    expect(svc.logout).toHaveBeenCalled()
    expect(store.getState().auth).toMatchObject({ user: null, token: null, isAuthenticated: false })
  })
})
