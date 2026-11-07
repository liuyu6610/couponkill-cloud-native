import { act, fireEvent, render, screen } from '@testing-library/react'
import { Provider } from 'react-redux'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom'
import type { NavigateFunction } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { store } from '../store'
import Header from './Header'

/** 暴露 navigate 与当前地址，模拟「列表页内改关键字」等 URL 变化 */
function RouterProbe({ onReady }: { onReady: (nav: NavigateFunction) => void }) {
  const navigate = useNavigate()
  const location = useLocation()
  onReady(navigate)
  return <div data-testid="location">{location.pathname + location.search}</div>
}

function renderHeader(initialEntry: string) {
  let navigate!: NavigateFunction
  const client = new QueryClient()
  const ui = (
    <Provider store={store}>
      <QueryClientProvider client={client}>
        <AntApp>
          <MemoryRouter initialEntries={[initialEntry]}>
            <Header />
            <RouterProbe onReady={(n) => (navigate = n)} />
          </MemoryRouter>
        </AntApp>
      </QueryClientProvider>
    </Provider>
  )
  render(ui)
  const input = screen.getByPlaceholderText('搜索优惠券...') as HTMLInputElement
  return { input, navigate: (to: string) => act(() => void navigate(to)) }
}

describe('Header 顶部搜索与 ?search= 同步', () => {
  it('打开带关键字的券列表时顶部框显示该关键字', () => {
    const { input } = renderHeader('/coupons?search=%E6%BB%A1%E5%87%8F')
    expect(input.value).toBe('满减')
  })

  it('列表页内修改 / 清空关键字后顶部框跟着变', () => {
    const { input, navigate } = renderHeader('/coupons?search=old')
    expect(input.value).toBe('old')
    navigate('/coupons?search=new')
    expect(input.value).toBe('new')
    navigate('/coupons')
    expect(input.value).toBe('')
  })

  it('顶部搜索提交后跳转到带关键字的列表页，离开列表页后清空', () => {
    const { input, navigate } = renderHeader('/')
    fireEvent.change(input, { target: { value: '  秒杀 ' } })
    expect(input.value).toBe('  秒杀 ')
    fireEvent.keyDown(input, { key: 'Enter', code: 'Enter', keyCode: 13 })
    expect(screen.getByTestId('location').textContent).toBe('/coupons?search=%E7%A7%92%E6%9D%80')
    expect(input.value).toBe('秒杀')
    navigate('/seckill')
    expect(input.value).toBe('')
  })

  it('非列表页上未提交的输入不会被重渲染覆盖', () => {
    const { input, navigate } = renderHeader('/')
    fireEvent.change(input, { target: { value: '草稿' } })
    navigate('/seckill')
    expect(input.value).toBe('草稿')
  })
})
