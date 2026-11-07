import { useQuery } from '@tanstack/react-query'
import { userService } from '../services/userService'
import { queryKeys, staleTimes } from '../lib/queryClient'

export function useUserProfile(userId?: string) {
  return useQuery({
    queryKey: queryKeys.user.profile(userId ?? 'me'),
    queryFn: () => userService.getProfile(),
    enabled: !!userId,
    staleTime: staleTimes.userProfile,
  })
}

/** 我的优惠券数量（GET /api/v1/user/coupon/count）；失败时调用方回退到订单列表的 total */
export function useUserCouponCount(userId?: string) {
  return useQuery({
    // 挂在 orders 前缀下：领取/取消订单后 invalidate(orders.all) 会一并刷新
    queryKey: queryKeys.orders.couponCount(userId ?? 'me'),
    queryFn: () => userService.getCouponCount(),
    enabled: !!userId,
    staleTime: staleTimes.orders,
    retry: false,
  })
}
