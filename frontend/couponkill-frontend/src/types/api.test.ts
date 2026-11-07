import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  couponStockOf,
  couponTotalStockOf,
  formatBackendDateTime,
  formatMoney,
  isPositiveMoney,
  orderSourceText,
  parseBackendDateTime,
  seckillWindowPhase,
} from './api'

describe('formatMoney', () => {
  it('固定两位小数：整数补零、一位小数补零、多位小数四舍五入', () => {
    expect(formatMoney(20)).toBe('20.00')
    expect(formatMoney(9.5)).toBe('9.50')
    expect(formatMoney(0)).toBe('0.00')
    expect(formatMoney(19.999)).toBe('20.00')
  })

  it('兼容字符串金额', () => {
    expect(formatMoney('100')).toBe('100.00')
    expect(formatMoney('12.3')).toBe('12.30')
  })

  it('空值显示 -，无法解析的原样返回（不显示 NaN）', () => {
    expect(formatMoney(undefined)).toBe('-')
    expect(formatMoney(null)).toBe('-')
    expect(formatMoney('')).toBe('-')
    expect(formatMoney('abc')).toBe('abc')
  })
})

describe('isPositiveMoney', () => {
  it('只有大于 0 的有限数才算有门槛', () => {
    expect(isPositiveMoney(100)).toBe(true)
    expect(isPositiveMoney('0.01')).toBe(true)
    expect(isPositiveMoney(0)).toBe(false)
    expect(isPositiveMoney('0.00')).toBe(false)
    expect(isPositiveMoney(undefined)).toBe(false)
    expect(isPositiveMoney('abc')).toBe(false)
  })
})

describe('库存与订单来源辅助', () => {
  it('常驻与秒杀券统一读 remainingStock / totalStock', () => {
    expect(couponStockOf({ remainingStock: 3 })).toBe(3)
    expect(couponTotalStockOf({ totalStock: 32 })).toBe(32)
  })

  it('订单来源映射，未知来源原样展示', () => {
    expect(orderSourceText('seckill')).toBe('秒杀')
    expect(orderSourceText('normal')).toBe('直接领取')
    expect(orderSourceText('reservation')).toBe('预约帮抢')
    expect(orderSourceText('admin')).toBe('管理员发放')
    expect(orderSourceText('future')).toBe('future')
    expect(orderSourceText(undefined)).toBe('-')
  })
})

describe('parseBackendDateTime', () => {
  it('解析后端 ISO-8601（RFC3339，带时区/小数秒）', () => {
    expect(parseBackendDateTime('2026-11-02T02:30:15Z')).toBe(Date.UTC(2026, 10, 2, 2, 30, 15))
    expect(parseBackendDateTime('2026-11-02T10:30:15+08:00')).toBe(Date.UTC(2026, 10, 2, 2, 30, 15))
    expect(parseBackendDateTime('2026-11-02T02:30:15.123456Z')).toBe(Date.UTC(2026, 10, 2, 2, 30, 15, 123))
  })

  it('解析后端 JsonFormat `yyyy-MM-dd HH:mm:ss` 为本地时间戳（Safari 不认空格分隔）', () => {
    expect(parseBackendDateTime('2026-11-02 10:30:15')).toBe(new Date(2026, 10, 2, 10, 30, 15).getTime())
  })

  it('也接受只到分钟的格式与 ISO 字符串', () => {
    expect(parseBackendDateTime('2026-11-02 10:30')).toBe(new Date(2026, 10, 2, 10, 30).getTime())
    expect(parseBackendDateTime('2026-11-02T02:30:15Z')).toBe(Date.UTC(2026, 10, 2, 2, 30, 15))
  })

  it('空值与无法解析的字符串返回 null', () => {
    expect(parseBackendDateTime(undefined)).toBeNull()
    expect(parseBackendDateTime(null)).toBeNull()
    expect(parseBackendDateTime('')).toBeNull()
    expect(parseBackendDateTime('not-a-date')).toBeNull()
  })
})

describe('formatBackendDateTime', () => {
  it('无值显示 -', () => {
    expect(formatBackendDateTime(undefined)).toBe('-')
    expect(formatBackendDateTime(null)).toBe('-')
    expect(formatBackendDateTime('')).toBe('-')
  })

  it('解析失败原样展示，而不是 Invalid Date', () => {
    expect(formatBackendDateTime('明天上午')).toBe('明天上午')
  })

  it('按模式输出本地化日期时间 / 日期', () => {
    const d = new Date(2026, 10, 2, 10, 30, 15)
    expect(formatBackendDateTime('2026-11-02 10:30:15')).toBe(d.toLocaleString())
    expect(formatBackendDateTime('2026-11-02 10:30:15', 'date')).toBe(d.toLocaleDateString())
    expect(formatBackendDateTime('2026-11-02 10:30:15')).not.toContain('Invalid')
  })
})

describe('seckillWindowPhase', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  const coupon = { seckillStartAt: '2026-11-02 10:00:00', seckillEndAt: '2026-11-02 11:00:00' }

  it('缺任一端点时视为未配置时间窗', () => {
    expect(seckillWindowPhase({ seckillStartAt: null, seckillEndAt: '2026-11-02 11:00:00' })).toBe('no_window')
    expect(seckillWindowPhase({ seckillStartAt: '2026-11-02 10:00:00' })).toBe('no_window')
  })

  it('开售前 / 时间窗内（含端点）/ 结束后', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date(2026, 10, 2, 9, 59, 59))
    expect(seckillWindowPhase(coupon)).toBe('upcoming')
    vi.setSystemTime(new Date(2026, 10, 2, 10, 0, 0))
    expect(seckillWindowPhase(coupon)).toBe('onsale')
    vi.setSystemTime(new Date(2026, 10, 2, 11, 0, 0))
    expect(seckillWindowPhase(coupon)).toBe('onsale')
    vi.setSystemTime(new Date(2026, 10, 2, 11, 0, 1))
    expect(seckillWindowPhase(coupon)).toBe('ended')
  })
})
