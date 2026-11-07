// 后端契约类型（单一真源），对齐 Go 后端（docs/GO-REWRITE-DESIGN.md §4/§10）。
// - 所有 ID（userId/couponId/orderId/…）在 JSON 里都是字符串，前端全程按 string 处理，不做 Number() 转换，
//   避免雪花 ID 超过 JS Number.MAX_SAFE_INTEGER 后精度丢失。
// - 时间字段是 ISO-8601（RFC3339，带时区）字符串，字段名 createdAt/updatedAt/getTime/expireTime/…。
// - 金额（faceValue/minSpend/price）是两位小数的 JSON 数字（兼容字符串），展示一律用 formatMoney。

// 统一响应包装：成功 `{code:200,message:"ok",data}`；失败为真实 HTTP 状态码 + `{code,message,data:null,requestId}`。
export interface ApiEnvelope<T> {
  code: number
  message: string
  data: T
  /** 失败信封里的请求号，与响应头 X-Request-Id 一致，用于排障 */
  requestId?: string
}

/** 分页信封 data：`{items,total,page,size}` */
export interface Page<T> {
  items: T[]
  total: number
  page: number
  size: number
}

/** 金额：后端输出两位小数的 JSON 数字，个别链路可能是字符串，两者都接受 */
export type Money = number | string

// 优惠券类型
export const CouponType = {
  NORMAL: 1, // 常驻
  SECKILL: 2, // 秒抢
} as const
export type CouponTypeValue = (typeof CouponType)[keyof typeof CouponType]

// 优惠券状态
export const CouponStatus = {
  INVALID: 0,
  VALID: 1,
} as const

// 订单状态
export const OrderStatus = {
  CREATED: 1, // 已创建
  USED: 2, // 已使用
  EXPIRED: 3, // 已过期
  CANCELLED: 4, // 已取消
} as const
export type OrderStatusValue = (typeof OrderStatus)[keyof typeof OrderStatus]

export interface Coupon {
  id: string
  name: string
  description?: string
  type: number // 1-常驻, 2-秒抢
  faceValue: Money
  minSpend: Money
  validDays: number
  perUserLimit: number
  /** 发行总量（秒杀券即秒杀总库存） */
  totalStock: number
  /** 剩余库存（库存桶之和）；常驻与秒杀券统一用这一对字段 */
  remainingStock: number
  status: number // 0-无效, 1-有效
  /** 秒杀开售时间（ISO-8601；活动时间窗） */
  seckillStartAt?: string | null
  /** 秒杀结束时间（ISO-8601） */
  seckillEndAt?: string | null
  createdAt?: string
  updatedAt?: string
}

export interface Order {
  id: string
  userId: string
  couponId: string
  /** 下单时拷贝的券快照，列表无需再查券服务 */
  couponName?: string
  couponType?: number
  faceValue?: Money
  minSpend?: Money
  status: number // 1-已创建,2-已使用,3-已过期,4-已取消
  /** seckill / normal / reservation / admin */
  source?: string
  requestId?: string
  getTime?: string
  expireTime?: string
  useTime?: string | null
  cancelTime?: string | null
  createdAt?: string
  updatedAt?: string
}

export interface UserInfo {
  id: string
  username: string
  phone?: string
  email?: string
  /** user | admin */
  role?: string
  status?: number
  createdAt?: string
  updatedAt?: string
  lastActiveAt?: string | null
}

// 登录返回体：`{token, userId, username, roles}`；roles 与 JWT 里的 roles 一致（如 ["user"] / ["admin"]）
export interface LoginResult {
  token: string
  userId: string
  username: string
  roles?: string[]
}

/** POST /order/seckill 受理成功（HTTP 202）时信封 data 的内容（对齐 events.SeckillAccepted） */
export interface SeckillAccepted {
  requestId: string
  status: string // 恒为 QUEUED
}

/** 秒杀请求状态：QUEUED → PROCESSING → SUCCESS | FAILED（对齐 events.Status） */
export type SeckillStatus = 'QUEUED' | 'PROCESSING' | 'SUCCESS' | 'FAILED'

/** GET /order/seckill/result 的 data（对齐 events.SeckillResult）；orderId 仅 SUCCESS，reason 仅 FAILED */
export interface SeckillResult {
  status: SeckillStatus | string
  orderId?: string
  reason?: string
}

// ---- 展示辅助 ----

export const isSeckillCoupon = (c: Pick<Coupon, 'type'>) => c.type === CouponType.SECKILL

export const couponTypeText = (type: number) =>
  type === CouponType.SECKILL ? '秒抢' : '常驻'

/** 剩余库存：常驻与秒杀券统一读 remainingStock */
export const couponStockOf = (c: Pick<Coupon, 'remainingStock'>) => c.remainingStock

/** 发行总量：常驻与秒杀券统一读 totalStock */
export const couponTotalStockOf = (c: Pick<Coupon, 'totalStock'>) => c.totalStock

export const orderStatusText = (status: number): string => {
  switch (status) {
    case OrderStatus.CREATED:
      return '已创建'
    case OrderStatus.USED:
      return '已使用'
    case OrderStatus.EXPIRED:
      return '已过期'
    case OrderStatus.CANCELLED:
      return '已取消'
    default:
      return '未知'
  }
}

/** 订单来源（ord.orders.source）→ 中文 */
export const orderSourceText = (source?: string | null): string => {
  switch (source) {
    case 'seckill':
      return '秒杀'
    case 'normal':
      return '直接领取'
    case 'reservation':
      return '预约帮抢'
    case 'admin':
      return '管理员发放'
    default:
      return source || '-'
  }
}

export const orderStatusColor = (status: number): string => {
  switch (status) {
    case OrderStatus.CREATED:
      return 'blue'
    case OrderStatus.USED:
      return 'green'
    case OrderStatus.EXPIRED:
      return 'default'
    case OrderStatus.CANCELLED:
      return 'red'
    default:
      return 'default'
  }
}

// ---- Connector（电商适配层）----

export const PlatformType = {
  MOCK: 'MOCK',
  JD: 'JD',
  TB: 'TB',
  PDD: 'PDD',
} as const
export type PlatformTypeValue = (typeof PlatformType)[keyof typeof PlatformType]

export interface ConnectorHealth {
  platform: PlatformTypeValue | string
  status: 'UP' | 'DOWN' | 'DISABLED' | string
  message?: string
}

export interface ConnectorPlatformInfo extends ConnectorHealth {
  jdEnabled?: boolean
  jdCredentialsConfigured?: boolean
  jdServerUrl?: string
  jdDefaultArea?: string
  jdAppKeyMasked?: string
}

export interface PlatformSkuBinding {
  id: string
  platform: PlatformTypeValue | string
  externalSkuId: string
  couponId: string
  syncEnabled: boolean
  lastStock?: number | null
  lastSyncAt?: string | null
  lastSyncStatus?: string | null
  lastError?: string | null
  createdAt?: string
  updatedAt?: string
}

export interface SkuBindingCommand {
  platform: PlatformTypeValue | string
  externalSkuId: string
  couponId: string
  syncEnabled?: boolean
}

export interface PlatformStockSnapshot {
  platform: PlatformTypeValue | string
  externalSkuId: string
  stockQty?: number | null
  stockAvailable?: boolean | null
  stockStateDesc?: string | null
  area?: string | null
}

export interface PlatformProductSnapshot {
  platform: PlatformTypeValue | string
  externalSkuId: string
  title?: string
  price?: Money | null
  onSale?: boolean | null
  rawStatus?: string | null
}

/** 同品比价条目（对齐 GET /connector/price-compare） */
export interface PriceCompareItem {
  platform: PlatformTypeValue | string
  externalSkuId: string
  title?: string | null
  price?: Money | null
  currency?: string | null
  fetchedAt?: string | null
  /** PROBE / MANUAL / BINDING */
  source?: string | null
  /** HIGH / MEDIUM / LOW */
  confidence?: string | null
  message?: string | null
}

export interface PriceCompareResult {
  couponId: string
  bindingId?: string | null
  items: PriceCompareItem[]
}

/** 同品比价手工映射（admin） */
export interface CouponPriceMap {
  id: string
  couponId: string
  platform: PlatformTypeValue | string
  externalSkuId: string
  title?: string | null
  manualPrice?: Money | null
  currency?: string | null
  enabled?: boolean
  createdAt?: string
  updatedAt?: string
}

export interface CouponPriceMapCommand {
  couponId: string
  platform: PlatformTypeValue | string
  externalSkuId: string
  title?: string
  manualPrice?: number | null
  currency?: string
  enabled?: boolean
}

export interface SyncAllResult {
  syncedOk: number
  failed?: number
  skipped?: number
  force?: boolean
}

// ---- 预约帮抢 ----

export const ReservationStatus = {
  PENDING: 'PENDING',
  FIRING: 'FIRING',
  QUEUED: 'QUEUED',
  SUCCESS: 'SUCCESS',
  FAILED: 'FAILED',
  CANCELLED: 'CANCELLED',
  EXPIRED: 'EXPIRED',
} as const
export type ReservationStatusValue =
  (typeof ReservationStatus)[keyof typeof ReservationStatus]

export interface SeckillReservation {
  id: string
  userId: string
  couponId: string
  status: ReservationStatusValue | string
  reserveAt?: string
  triggerAt?: string
  firedAt?: string
  requestId?: string | null
  orderId?: string | null
  failCode?: number | null
  failReason?: string | null
  retryCount?: number
  createdAt?: string
  updatedAt?: string
}

export interface UserNotification {
  id: string
  userId: string
  type: string
  title: string
  content?: string | null
  refId?: string | null
  readFlag?: boolean
  createdAt?: string
}

export const reservationStatusText = (status: string): string => {
  switch (status) {
    case ReservationStatus.PENDING:
      return '待开抢'
    case ReservationStatus.FIRING:
      return '抢购中'
    case ReservationStatus.QUEUED:
      return '已入队'
    case ReservationStatus.SUCCESS:
      return '已抢到'
    case ReservationStatus.FAILED:
      return '失败'
    case ReservationStatus.CANCELLED:
      return '已取消'
    case ReservationStatus.EXPIRED:
      return '已过期'
    default:
      return status || '未知'
  }
}

export const reservationStatusColor = (status: string): string => {
  switch (status) {
    case ReservationStatus.PENDING:
      return 'blue'
    case ReservationStatus.FIRING:
    case ReservationStatus.QUEUED:
      return 'processing'
    case ReservationStatus.SUCCESS:
      return 'success'
    case ReservationStatus.FAILED:
    case ReservationStatus.EXPIRED:
      return 'error'
    case ReservationStatus.CANCELLED:
      return 'default'
    default:
      return 'default'
  }
}

/**
 * 金额展示：固定两位小数（"20" → "20.00"，"9.5" → "9.50"）。
 * 后端输出两位小数的 JSON 数字（个别链路可能是字符串），都接受；空值显示 '-'，无法解析的原样返回，不显示 NaN。
 */
export function formatMoney(v?: Money | null): string {
  if (v == null || v === '') return '-'
  const n = typeof v === 'number' ? v : Number(v)
  if (!Number.isFinite(n)) return String(v)
  return n.toFixed(2)
}

/** 金额是否大于 0（用于「满 X 可用」的门槛判断）；字符串金额同样适用 */
export function isPositiveMoney(v?: Money | null): boolean {
  if (v == null || v === '') return false
  const n = typeof v === 'number' ? v : Number(v)
  return Number.isFinite(n) && n > 0
}

/**
 * 解析后端时间为毫秒时间戳。后端统一输出 ISO-8601（RFC3339，带时区）；
 * 同时兼容历史 `yyyy-MM-dd HH:mm:ss`（Safari 不认空格分隔，替换成 T 再解析）。
 */
export function parseBackendDateTime(v?: string | null): number | null {
  if (v == null || v === '') return null
  const normalized = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}/.test(v) ? v.replace(' ', 'T') : v
  const n = Date.parse(normalized)
  return Number.isNaN(n) ? null : n
}

/**
 * 后端时间展示：统一经 parseBackendDateTime 解析（ISO-8601 为主，兼容旧的空格分隔格式，
 * 后者直接 `new Date()` 在 Safari/iOS 会得到 Invalid Date）；无值显示 '-'，解析失败原样展示而不是 "Invalid Date"。
 */
export function formatBackendDateTime(
  v?: string | null,
  mode: 'datetime' | 'date' = 'datetime'
): string {
  if (v == null || v === '') return '-'
  const ts = parseBackendDateTime(v)
  if (ts == null) return v
  const d = new Date(ts)
  return mode === 'date' ? d.toLocaleDateString() : d.toLocaleString()
}

/** 开售前可预约；已开售可直接秒杀 */
export function seckillWindowPhase(c: Pick<Coupon, 'seckillStartAt' | 'seckillEndAt'>):
  | 'no_window'
  | 'upcoming'
  | 'onsale'
  | 'ended' {
  const start = parseBackendDateTime(c.seckillStartAt)
  const end = parseBackendDateTime(c.seckillEndAt)
  if (start == null || end == null) return 'no_window'
  const now = Date.now()
  if (now < start) return 'upcoming'
  if (now > end) return 'ended'
  return 'onsale'
}

