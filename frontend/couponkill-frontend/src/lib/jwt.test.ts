import { describe, expect, it } from 'vitest'
import { rolesFromJwt } from './jwt'

/** 构造一个只有载荷有意义的 JWT（base64url，无填充） */
function tokenWith(payload: unknown): string {
  const b64url = (s: string) =>
    btoa(String.fromCharCode(...new TextEncoder().encode(s)))
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=+$/, '')
  return `${b64url('{"alg":"HS256"}')}.${b64url(JSON.stringify(payload))}.sig`
}

describe('rolesFromJwt', () => {
  it('读取载荷里的 roles', () => {
    expect(rolesFromJwt(tokenWith({ sub: '1', roles: ['user', 'admin'] }))).toEqual(['user', 'admin'])
  })

  it('能处理需要 base64url 填充和非 ASCII 的载荷', () => {
    expect(rolesFromJwt(tokenWith({ name: '管理员甲', roles: ['admin'] }))).toEqual(['admin'])
  })

  it('格式不对 / 缺 roles / 类型不对 / 空值都返回空数组，不抛异常', () => {
    expect(rolesFromJwt(undefined)).toEqual([])
    expect(rolesFromJwt('')).toEqual([])
    expect(rolesFromJwt('not-a-jwt')).toEqual([])
    expect(rolesFromJwt('a.%%%.c')).toEqual([])
    expect(rolesFromJwt(tokenWith({ sub: '1' }))).toEqual([])
    expect(rolesFromJwt(tokenWith({ roles: 'admin' }))).toEqual([])
  })
})
