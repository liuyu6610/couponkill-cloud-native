import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import type { Coupon } from '../types/api'
import { CouponType } from '../types/api'
import CouponCard from './CouponCard'

const coupon: Coupon = {
  id: '9007199254740993',
  name: '满100减20',
  type: CouponType.NORMAL,
  faceValue: 20,
  minSpend: 100,
  validDays: 7,
  perUserLimit: 1,
  totalStock: 100,
  remainingStock: 50,
  status: 1,
}

function LocationProbe() {
  const location = useLocation()
  return <div data-testid="location">{location.pathname}</div>
}

function renderCard() {
  render(
    <MemoryRouter initialEntries={['/coupons']}>
      <Routes>
        <Route path="*" element={<CouponCard coupon={coupon} showActions />} />
      </Routes>
      <LocationProbe />
    </MemoryRouter>
  )
  return screen.getByRole('link', { name: '查看详情' })
}

describe('CouponCard 金额与库存展示', () => {
  it('金额固定两位小数，门槛为 0 时不显示「满…可用」', () => {
    const { rerender } = render(
      <MemoryRouter>
        <CouponCard coupon={{ ...coupon, faceValue: 20, minSpend: '100' }} />
      </MemoryRouter>
    )
    expect(screen.getByText('¥20.00')).toBeTruthy()
    expect(screen.getByText('满¥100.00可用')).toBeTruthy()
    rerender(
      <MemoryRouter>
        <CouponCard coupon={{ ...coupon, faceValue: '9.5', minSpend: 0 }} />
      </MemoryRouter>
    )
    expect(screen.getByText('¥9.50')).toBeTruthy()
    expect(screen.queryByText(/可用/)).toBeNull()
  })

  it('常驻与秒杀券统一展示 remainingStock/totalStock', () => {
    render(
      <MemoryRouter>
        <CouponCard coupon={{ ...coupon, type: CouponType.SECKILL, remainingStock: 7, totalStock: 32 }} />
      </MemoryRouter>
    )
    expect(screen.getByText('库存: 7/32')).toBeTruthy()
  })
})

describe('CouponCard 查看详情', () => {
  it('渲染为带 href 的链接（可新标签打开 / 复制链接，读屏播报为链接）', () => {
    const link = renderCard()
    expect(link.tagName).toBe('A')
    expect(link.getAttribute('href')).toBe(`/coupons/${coupon.id}`)
  })

  it('普通左键点击在应用内跳转（SPA，不整页刷新）', () => {
    const link = renderCard()
    const notPrevented = fireEvent.click(link, { button: 0 })
    expect(notPrevented).toBe(false)
    expect(screen.getByTestId('location').textContent).toBe(`/coupons/${coupon.id}`)
  })

  it('Ctrl/Cmd/Shift 点击交给浏览器默认行为（新标签/新窗口），不做应用内跳转', () => {
    const link = renderCard()
    // 记录 react-router 处理后的 defaultPrevented，再在 document 兜底拦截，避免 jsdom 真去做整页导航（刷屏 Not implemented）
    let prevented = false
    const guard = (e: Event) => {
      prevented = e.defaultPrevented
      e.preventDefault()
    }
    document.addEventListener('click', guard)
    try {
      for (const mod of [{ ctrlKey: true }, { metaKey: true }, { shiftKey: true }]) {
        fireEvent.click(link, { button: 0, ...mod })
        expect(prevented).toBe(false)
        expect(screen.getByTestId('location').textContent).toBe('/coupons')
      }
    } finally {
      document.removeEventListener('click', guard)
    }
  })
})
