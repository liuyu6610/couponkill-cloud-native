import { http } from '../lib/apiClient'
import type {
  ConnectorHealth,
  ConnectorPlatformInfo,
  CouponPriceMap,
  CouponPriceMapCommand,
  PlatformProductSnapshot,
  PlatformSkuBinding,
  PlatformStockSnapshot,
  PriceCompareResult,
  SkuBindingCommand,
  SyncAllResult,
} from '../types/api'

/** 电商 Connector 管理 API（对齐 connector-service） */
export const connectorService = {
  getHealth(): Promise<ConnectorHealth[]> {
    return http.get<ConnectorHealth[]>('/api/v1/connector/health')
  },

  getPlatforms(): Promise<ConnectorPlatformInfo[]> {
    return http.get<ConnectorPlatformInfo[]>('/api/v1/connector/platforms')
  },

  listBindings(): Promise<PlatformSkuBinding[]> {
    return http.get<PlatformSkuBinding[]>('/api/v1/connector/bindings')
  },

  createOrUpdateBinding(cmd: SkuBindingCommand): Promise<PlatformSkuBinding> {
    // couponId 是字符串（雪花 ID），原样发送，避免大整数精度丢失
    return http.post<PlatformSkuBinding>('/api/v1/connector/bindings', {
      platform: cmd.platform,
      externalSkuId: cmd.externalSkuId,
      couponId: cmd.couponId,
      syncEnabled: cmd.syncEnabled,
    })
  },

  syncOne(id: string, force = false): Promise<PlatformSkuBinding> {
    return http.post<PlatformSkuBinding>(`/api/v1/connector/sync/${id}`, null, {
      params: { force },
    })
  },

  syncAll(force = false): Promise<SyncAllResult> {
    return http.post<SyncAllResult>('/api/v1/connector/sync', null, {
      params: { force },
    })
  },

  probeStock(platform: string, skuId: string): Promise<PlatformStockSnapshot> {
    return http.get<PlatformStockSnapshot>(
      `/api/v1/connector/probe/${encodeURIComponent(platform)}/stock`,
      { params: { skuId } }
    )
  },

  probeProduct(platform: string, skuId: string): Promise<PlatformProductSnapshot> {
    return http.get<PlatformProductSnapshot>(
      `/api/v1/connector/probe/${encodeURIComponent(platform)}/product`,
      { params: { skuId } }
    )
  },

  /** C 端只读：按券查绑定（可匿名） */
  getBindingByCoupon(couponId: string): Promise<PlatformSkuBinding | null> {
    return http.get<PlatformSkuBinding | null>(
      `/api/v1/connector/bindings/by-coupon/${couponId}`
    )
  },

  /** C 端只读：同品比价（绑定 + probe；可匿名） */
  priceCompare(couponId: string): Promise<PriceCompareResult> {
    return http.get<PriceCompareResult>('/api/v1/connector/price-compare', {
      params: { couponId },
    })
  },

  listPriceMaps(couponId: string): Promise<CouponPriceMap[]> {
    return http.get<CouponPriceMap[]>('/api/v1/connector/price-maps', {
      params: { couponId },
    })
  },

  upsertPriceMap(cmd: CouponPriceMapCommand): Promise<CouponPriceMap> {
    return http.post<CouponPriceMap>('/api/v1/connector/price-maps', cmd)
  },

  deletePriceMap(id: string): Promise<void> {
    return http.del<void>(`/api/v1/connector/price-maps/${id}`)
  },
}

export default connectorService
