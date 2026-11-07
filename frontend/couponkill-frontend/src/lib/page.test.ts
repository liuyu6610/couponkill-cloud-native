import { describe, expect, it } from 'vitest'
import { toList, toPage } from './page'

describe('toList', () => {
  it('裸数组与分页信封都规整成数组，空值为空数组', () => {
    expect(toList([1, 2])).toEqual([1, 2])
    expect(toList({ items: [3] })).toEqual([3])
    expect(toList(null)).toEqual([])
    expect(toList(undefined)).toEqual([])
    expect(toList({ items: undefined } as unknown as { items: number[] })).toEqual([])
  })
})

describe('toPage', () => {
  it('分页信封原样采用后端的 total/page/size', () => {
    expect(toPage({ items: ['a'], total: 41, page: 3, size: 20 }, 1, 10)).toEqual({
      items: ['a'],
      total: 41,
      page: 3,
      size: 20,
    })
  })

  it('信封缺 total 时按已知下界推算', () => {
    expect(toPage({ items: ['a', 'b'] } as never, 2, 10).total).toBe(12)
  })

  it('裸数组：不满页 total 为精确值，满页则假设还有下一页', () => {
    expect(toPage(['a', 'b'], 1, 10)).toEqual({ items: ['a', 'b'], total: 2, page: 1, size: 10 })
    expect(toPage(Array(10).fill('x'), 2, 10).total).toBe(21)
  })

  it('空值返回空页', () => {
    expect(toPage(null, 1, 10)).toEqual({ items: [], total: 0, page: 1, size: 10 })
  })
})
