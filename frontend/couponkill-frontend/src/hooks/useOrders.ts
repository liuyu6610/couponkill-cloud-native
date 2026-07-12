import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { orderService } from '../services/orderService'
import { queryKeys } from '../lib/queryClient'

export function useUserOrders(userId?: string, pageNum = 1, pageSize = 20) {
  return useQuery({
    queryKey: [...queryKeys.orders.byUser(userId ?? 'me'), pageNum, pageSize],
    queryFn: () => orderService.getUserOrders(pageNum, pageSize),
    enabled: !!userId,
  })
}

export function useCreateOrder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ couponId }: { couponId: string }) => orderService.createOrder(couponId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.orders.all })
      qc.invalidateQueries({ queryKey: queryKeys.coupons.available })
    },
  })
}

export function useCancelOrder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ orderId }: { orderId: string }) => orderService.cancelOrder(orderId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.orders.all })
    },
  })
}
