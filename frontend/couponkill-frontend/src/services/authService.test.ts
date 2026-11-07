import axios from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { STORAGE_KEYS } from '../lib/apiClient'
import { authService } from './authService'

describe('authService.logout', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })
  afterEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
  })

  function seedSession() {
    localStorage.setItem(STORAGE_KEYS.token, 'tok-9')
    localStorage.setItem(STORAGE_KEYS.userId, '42')
    localStorage.setItem(STORAGE_KEYS.username, 'demo')
    localStorage.setItem(STORAGE_KEYS.roles, '["user"]')
  }

  it('fire-and-forget 调用 POST /api/v1/user/logout（带当前令牌）并立即清本地会话', () => {
    const post = vi.spyOn(axios, 'post').mockResolvedValue({ status: 200, data: {} })
    seedSession()
    authService.logout()

    expect(post).toHaveBeenCalledTimes(1)
    const [url, body, config] = post.mock.calls[0]
    expect(url).toBe('/api/v1/user/logout')
    expect(body).toBeNull()
    expect((config as { headers: Record<string, string> }).headers.Authorization).toBe('Bearer tok-9')
    for (const k of Object.values(STORAGE_KEYS)) expect(localStorage.getItem(k)).toBeNull()
  })

  it('服务端吊销失败只记日志，不抛异常、本地会话照样清掉', async () => {
    const post = vi.spyOn(axios, 'post').mockRejectedValue(new Error('boom'))
    seedSession()
    expect(() => authService.logout()).not.toThrow()
    await vi.waitFor(() => expect(console.warn).toHaveBeenCalled())
    expect(post).toHaveBeenCalledTimes(1)
    for (const k of Object.values(STORAGE_KEYS)) expect(localStorage.getItem(k)).toBeNull()
  })

  it('本地没有令牌时不发请求', () => {
    const post = vi.spyOn(axios, 'post').mockResolvedValue({})
    authService.logout()
    expect(post).not.toHaveBeenCalled()
  })
})
