import { QueryClient } from '@tanstack/react-query'

// 全局 QueryClient：服务端状态的缓存/失效/重试统一在此配置
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000, // 默认 30s；各 hook 可按数据新鲜度覆盖
      gcTime: 5 * 60_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
    mutations: {
      retry: 0,
    },
  },
})

/**
 * 查询键集中管理。
 * 秒杀券与可用券共用 `coupons` 前缀，invalidateQueries({ queryKey: queryKeys.coupons.root })
 * 即可一次刷掉列表侧缓存，避免散落双 key 漏刷。
 */
export const queryKeys = {
  coupons: {
    root: ['coupons'] as const,
    available: ['coupons', 'available'] as const,
    seckill: ['coupons', 'seckill'] as const,
    detail: (id: string) => ['coupons', 'detail', id] as const,
  },
  orders: {
    all: ['orders'] as const,
    byUser: (userId: string) => ['orders', 'user', userId] as const,
  },
  user: {
    profile: (userId: string) => ['user', 'profile', userId] as const,
  },
  connector: {
    platforms: ['connector', 'platforms'] as const,
    bindings: ['connector', 'bindings'] as const,
    bindingByCoupon: (couponId: string) => ['connector', 'binding', couponId] as const,
  },
  reservations: {
    root: ['reservations'] as const,
    mine: ['reservations', 'mine'] as const,
  },
}

/** 各域推荐 staleTime（毫秒），避免全局一刀切过宽/过窄 */
export const staleTimes = {
  couponsAvailable: 60_000,
  couponsSeckill: 10_000,
  couponsDetail: 60_000,
  orders: 20_000,
  userProfile: 5 * 60_000,
  connectorPlatforms: 60_000,
  connectorBindings: 15_000,
  reservations: 5_000,
} as const
