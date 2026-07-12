import { useQuery } from '@tanstack/react-query'
import { couponService } from '../services/couponService'
import { queryKeys } from '../lib/queryClient'

// 可用优惠券列表
export function useAvailableCoupons() {
  return useQuery({
    queryKey: queryKeys.coupons.available,
    queryFn: () => couponService.getAvailableCoupons(),
  })
}

// 秒杀优惠券列表
export function useSeckillCoupons() {
  return useQuery({
    queryKey: queryKeys.coupons.seckill,
    queryFn: () => couponService.getSeckillCoupons(),
  })
}

// 优惠券详情
export function useCouponDetail(id?: string) {
  return useQuery({
    queryKey: queryKeys.coupons.detail(id ?? ''),
    queryFn: () => couponService.getCouponById(id as string),
    enabled: !!id,
  })
}
