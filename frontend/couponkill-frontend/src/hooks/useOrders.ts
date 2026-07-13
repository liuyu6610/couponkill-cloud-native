import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { orderService } from '../services/orderService'
import { queryKeys, staleTimes } from '../lib/queryClient'
import type { Order } from '../types/api'

/** 默认拉一页足够列表/用户中心/详情共用同一缓存键，避免重复请求 */
export const ORDERS_DEFAULT_PAGE_SIZE = 50

export function useUserOrders(
  userId?: string,
  pageNum = 1,
  pageSize = ORDERS_DEFAULT_PAGE_SIZE
) {
  return useQuery({
    queryKey: [...queryKeys.orders.byUser(userId ?? 'me'), pageNum, pageSize],
    queryFn: () => orderService.getUserOrders(pageNum, pageSize),
    enabled: !!userId,
    staleTime: staleTimes.orders,
    // 翻页时保留上一页数据，减少表格闪空（v5 placeholderData）
    placeholderData: keepPreviousData,
  })
}

/** 后端无单订单接口：从用户订单列表缓存中解析，与列表页共享 query */
export function useOrderFromList(userId?: string, orderId?: string) {
  return useQuery({
    queryKey: [...queryKeys.orders.byUser(userId ?? 'me'), 1, ORDERS_DEFAULT_PAGE_SIZE],
    queryFn: () => orderService.getUserOrders(1, ORDERS_DEFAULT_PAGE_SIZE),
    enabled: !!userId && !!orderId,
    staleTime: staleTimes.orders,
    select: (orders: Order[]) => orders.find((o) => o.id === orderId),
  })
}

export function useCreateOrder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ couponId }: { couponId: string }) => orderService.createOrder(couponId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.orders.all })
      void qc.invalidateQueries({ queryKey: queryKeys.coupons.root })
    },
  })
}

export function useCancelOrder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ orderId }: { orderId: string }) => orderService.cancelOrder(orderId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.orders.all })
    },
  })
}
