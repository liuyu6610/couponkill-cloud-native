// 后端契约类型（单一真源）。字段与 Java POJO / Controller 严格对齐。
// 注意：所有 ID（userId/couponId/orderId）在前端统一按 string 处理，
// 避免后端 Long/雪花 ID 超过 JS Number.MAX_SAFE_INTEGER 时精度丢失。

// 统一响应包装：user/coupon 服务用 ApiResponse(code=0 成功)，
// order 服务用 Result(code=200 成功)。apiClient 会同时兼容两者。
export interface ApiEnvelope<T> {
  code: number
  message: string
  data: T
}

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
  faceValue: number
  minSpend: number
  validDays: number
  perUserLimit: number
  totalStock: number
  seckillTotalStock: number
  remainingStock: number
  seckillRemainingStock: number
  status: number // 0-无效, 1-有效
  /** 秒杀开售时间（活动时间窗） */
  seckillStartAt?: string | null
  /** 秒杀结束时间 */
  seckillEndAt?: string | null
  createTime?: string
  updateTime?: string
  version?: number
  shardIndex?: number
}

export interface Order {
  id: string
  userId: string
  couponId: string
  virtualId?: string
  status: number // 1-已创建,2-已使用,3-已过期,4-已取消
  getTime?: string
  expireTime?: string
  useTime?: string
  cancelTime?: string
  createTime?: string
  updateTime?: string
  createdByJava?: number
  createdByGo?: number
  requestId?: string
  version?: number
}

export interface UserInfo {
  id: string
  username: string
  phone?: string
  email?: string
  status?: number
  createTime?: string
  updateTime?: string
  lastActiveTime?: string
}

// 登录返回体：后端 UserServiceImpl.login 返回 {token, userId, username}
export interface LoginResult {
  token: string
  userId: string
  username: string
  roles?: string[]
}

// ---- 展示辅助 ----

export const isSeckillCoupon = (c: Pick<Coupon, 'type'>) => c.type === CouponType.SECKILL

export const couponTypeText = (type: number) =>
  type === CouponType.SECKILL ? '秒抢' : '常驻'

export const couponStockOf = (c: Coupon) =>
  isSeckillCoupon(c) ? c.seckillRemainingStock : c.remainingStock

export const couponTotalStockOf = (c: Coupon) =>
  isSeckillCoupon(c) ? c.seckillTotalStock : c.totalStock

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
  createTime?: string
  updateTime?: string
}

export interface SkuBindingCommand {
  platform: PlatformTypeValue | string
  externalSkuId: string
  couponId: string | number
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
  price?: number | null
  onSale?: boolean | null
  rawStatus?: string | null
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
  version?: number
  createTime?: string
  updateTime?: string
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

/** 兼容后端 JsonFormat `yyyy-MM-dd HH:mm:ss` */
export function parseBackendDateTime(v?: string | null): number | null {
  if (v == null || v === '') return null
  const normalized = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}/.test(v) ? v.replace(' ', 'T') : v
  const n = Date.parse(normalized)
  return Number.isNaN(n) ? null : n
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

