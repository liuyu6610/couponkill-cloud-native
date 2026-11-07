import { describe, expect, it } from 'vitest'
import { headerSearchKeyword } from './couponSearch'

describe('headerSearchKeyword', () => {
  it('券列表页跟随 ?search=（trim，与列表页一致）', () => {
    expect(headerSearchKeyword('/coupons', '?search=%20%E6%BB%A1%E5%87%8F%20')).toBe('满减')
    expect(headerSearchKeyword('/coupons/', 'search=abc')).toBe('abc')
    expect(headerSearchKeyword('/coupons', '?type=2&search=a%26b')).toBe('a&b')
  })

  it('券列表页无关键字时为空', () => {
    expect(headerSearchKeyword('/coupons', '')).toBe('')
    expect(headerSearchKeyword('/coupons', '?search=')).toBe('')
  })

  it('其它页面（含券详情）一律为空，不残留旧关键字', () => {
    expect(headerSearchKeyword('/', '?search=abc')).toBe('')
    expect(headerSearchKeyword('/coupons/123', '?search=abc')).toBe('')
    expect(headerSearchKeyword('/seckill', '')).toBe('')
  })
})
