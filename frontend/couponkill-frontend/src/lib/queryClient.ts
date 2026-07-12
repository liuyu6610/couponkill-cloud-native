import { QueryClient } from '@tanstack/react-query'

// 全局 QueryClient：服务端状态的缓存/失效/重试统一在此配置
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000, // 30s 内视为新鲜，避免重复请求
      gcTime: 5 * 60_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
    mutations: {
      retry: 0,
    },
  },
})

// 集中管理查询键，避免字符串散落
export const queryKeys = {
  coupons: {
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
}
