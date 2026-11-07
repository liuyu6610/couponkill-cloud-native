import type { InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import instance from '../lib/apiClient'
import { authService } from './authService'
import { couponService } from './couponService'
import { notificationService } from './notificationService'
import { orderService } from './orderService'
import { reservationService } from './reservationService'
import { userService } from './userService'

/**
 * 契约测试：钉住前端对 Go 后端的请求形状（方法、路径、JSON 体、查询参数）与响应解包，
 * 后端实现者可以对照这里检查接口是否一致。
 */
interface Captured {
  method?: string
  url?: string
  data: unknown
  params?: unknown
}

let last: Captured
const originalAdapter = instance.defaults.adapter

/** 假服务端：记录最近一次请求，返回给定 data（信封 code=200，状态 200/202） */
function serve(data: unknown, status = 200) {
  instance.defaults.adapter = async (config: InternalAxiosRequestConfig) => {
    last = {
      method: config.method,
      url: config.url,
      data: typeof config.data === 'string' ? JSON.parse(config.data) : config.data,
      params: config.params,
    }
    return {
      data: { code: 200, message: 'ok', data },
      status,
      statusText: '',
      headers: {},
      config,
      request: {},
    }
  }
}

// 雪花 ID 超过 2^53：必须始终是字符串，任何一层都不能转成 Number
const BIG_ID = '9007199254740993'

describe('服务层请求契约', () => {
  beforeEach(() => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })
  afterEach(() => {
    instance.defaults.adapter = originalAdapter
    vi.restoreAllMocks()
  })

  describe('order', () => {
    it('POST /api/v1/order/seckill：JSON 体 {couponId}，不带 query；解包 202 的 {requestId,status}', async () => {
      serve({ requestId: 'r1', status: 'QUEUED' }, 202)
      await expect(orderService.seckill(BIG_ID)).resolves.toEqual({ requestId: 'r1', status: 'QUEUED' })
      expect(last).toMatchObject({ method: 'post', url: '/api/v1/order/seckill', data: { couponId: BIG_ID } })
      expect(last.params).toBeUndefined()
    })

    it('GET /api/v1/order/seckill/result?requestId=：返回 {status, orderId?, reason?} 对象，orderId 是字符串', async () => {
      serve({ status: 'SUCCESS', orderId: BIG_ID })
      const r = await orderService.seckillResult('r1')
      expect(r).toEqual({ status: 'SUCCESS', orderId: BIG_ID })
      expect(typeof r.orderId).toBe('string')
      expect(last).toMatchObject({ method: 'get', url: '/api/v1/order/seckill/result', params: { requestId: 'r1' } })
    })

    it('POST /api/v1/order/create 与 /cancel：JSON 体', async () => {
      serve({ id: BIG_ID })
      await orderService.createOrder('1002')
      expect(last).toMatchObject({ method: 'post', url: '/api/v1/order/create', data: { couponId: '1002' } })
      serve(true)
      await orderService.cancelOrder(BIG_ID)
      expect(last).toMatchObject({ method: 'post', url: '/api/v1/order/cancel', data: { orderId: BIG_ID } })
    })

    it('GET /api/v1/order/user/me?page=&size=：分页信封，ID 保持字符串', async () => {
      serve({ items: [{ id: BIG_ID, userId: '1', couponId: '2', status: 1 }], total: 21, page: 2, size: 10 })
      const page = await orderService.getUserOrders(2, 10)
      expect(last).toMatchObject({ method: 'get', url: '/api/v1/order/user/me', params: { page: 2, size: 10 } })
      expect(page.total).toBe(21)
      expect(page.items[0].id).toBe(BIG_ID)
    })

    it('我的订单兼容裸数组（total 取已知下界）', async () => {
      serve([{ id: '1', userId: '1', couponId: '2', status: 1 }])
      const page = await orderService.getUserOrders(1, 10)
      expect(page).toMatchObject({ total: 1, page: 1, size: 10 })
    })
  })

  describe('coupon', () => {
    it('GET /available 与 /list 兼容裸数组和分页信封', async () => {
      serve([{ id: '1' }])
      expect(await couponService.getAvailableCoupons()).toHaveLength(1)
      expect(last).toMatchObject({ method: 'get', url: '/api/v1/coupon/available' })
      serve({ items: [{ id: '1' }, { id: '2' }], total: 2, page: 1, size: 20 })
      expect(await couponService.getAllCoupons()).toHaveLength(2)
      expect(last.url).toBe('/api/v1/coupon/list')
    })

    it('秒杀券列表 = available 按 type=2 过滤', async () => {
      serve([
        { id: '1', type: 1 },
        { id: '2', type: 2 },
      ])
      expect((await couponService.getSeckillCoupons()).map((c) => c.id)).toEqual(['2'])
    })

    it('管理写接口全部是 JSON 体，时间为 ISO-8601', async () => {
      serve({ id: '1' })
      await couponService.createCoupon({
        name: 'n',
        type: 2,
        faceValue: 10.5,
        minSpend: 0,
        validDays: 7,
        perUserLimit: 1,
        totalStock: 32,
        seckillStartAt: '2026-11-06T10:00:00.000Z',
        seckillEndAt: '2026-11-06T11:00:00.000Z',
      })
      expect(last).toMatchObject({
        method: 'post',
        url: '/api/v1/coupon/create',
        data: { name: 'n', faceValue: 10.5, totalStock: 32, seckillStartAt: '2026-11-06T10:00:00.000Z' },
      })
      expect(last.params).toBeUndefined()

      await couponService.updateSeckillWindow(BIG_ID, '2026-11-06T10:00:00.000Z', '2026-11-06T11:00:00.000Z')
      expect(last).toMatchObject({
        method: 'post',
        url: `/api/v1/coupon/${BIG_ID}/seckill-window`,
        data: { seckillStartAt: '2026-11-06T10:00:00.000Z', seckillEndAt: '2026-11-06T11:00:00.000Z' },
      })

      await couponService.updateCouponStatus(BIG_ID, 0)
      expect(last).toMatchObject({ method: 'post', url: `/api/v1/coupon/${BIG_ID}/status`, data: { status: 0 } })

      await couponService.deleteCoupon(BIG_ID)
      expect(last).toMatchObject({ method: 'delete', url: `/api/v1/coupon/${BIG_ID}` })
    })
  })

  describe('reservation / notification / user', () => {
    it('预约：POST {couponId}、DELETE /{id}、GET /mine', async () => {
      serve({ id: '1' })
      await reservationService.create('7')
      expect(last).toMatchObject({ method: 'post', url: '/api/v1/order/reservations', data: { couponId: '7' } })
      await reservationService.cancel('9')
      expect(last).toMatchObject({ method: 'delete', url: '/api/v1/order/reservations/9' })
      serve([{ id: '1' }])
      expect(await reservationService.listMine()).toHaveLength(1)
      expect(last).toMatchObject({ method: 'get', url: '/api/v1/order/reservations/mine' })
    })

    it('通知：GET /mine?limit=、/unread-count → {count}、POST /{id}/read、/read-all', async () => {
      serve([{ id: '1', title: 't' }])
      expect(await notificationService.listMine(15)).toHaveLength(1)
      expect(last).toMatchObject({ url: '/api/v1/order/notifications/mine', params: { limit: 15 } })
      serve({ count: 3 })
      expect(await notificationService.unreadCount()).toBe(3)
      expect(last.url).toBe('/api/v1/order/notifications/unread-count')
      serve(true)
      await notificationService.markRead('5')
      expect(last).toMatchObject({ method: 'post', url: '/api/v1/order/notifications/5/read' })
      await notificationService.markAllRead()
      expect(last).toMatchObject({ method: 'post', url: '/api/v1/order/notifications/read-all' })
    })

    it('GET /api/v1/user/coupon/count：兼容裸数字与 {count}', async () => {
      serve(5)
      await expect(userService.getCouponCount()).resolves.toBe(5)
      expect(last).toMatchObject({ method: 'get', url: '/api/v1/user/coupon/count' })
      serve({ count: 8 })
      await expect(userService.getCouponCount()).resolves.toBe(8)
      serve(null)
      await expect(userService.getCouponCount()).resolves.toBe(0)
    })

    it('登录 / 注册：JSON 体，口令不进 URL；登录返回 {token,userId,username,roles}', async () => {
      serve({ token: 't', userId: BIG_ID, username: 'demo', roles: ['user'] })
      const r = await authService.login('demo', 'demo1234')
      expect(last).toMatchObject({
        method: 'post',
        url: '/api/v1/user/login',
        data: { username: 'demo', password: 'demo1234' },
      })
      expect(last.params).toBeUndefined()
      expect(r.userId).toBe(BIG_ID)
      expect(r.roles).toEqual(['user'])

      serve({ id: BIG_ID, username: 'u' })
      await authService.register('u', 'pass123', '13800000000')
      expect(last).toMatchObject({
        url: '/api/v1/user/register',
        data: { username: 'u', password: 'pass123', phone: '13800000000' },
      })
    })
  })
})
