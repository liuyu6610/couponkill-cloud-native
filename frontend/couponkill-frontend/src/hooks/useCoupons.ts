import { useQuery, useQueryClient } from '@tanstack/react-query'
import { couponService } from '../services/couponService'
import { queryKeys, staleTimes } from '../lib/queryClient'
import type { Coupon } from '../types/api'

// 可用优惠券列表（变更不频繁）
export function useAvailableCoupons() {
  return useQuery({
    queryKey: queryKeys.coupons.available,
    queryFn: () => couponService.getAvailableCoupons(),
    staleTime: staleTimes.couponsAvailable,
  })
}

// 秒杀优惠券列表（库存变化快，缩短新鲜窗口）
export function useSeckillCoupons() {
  return useQuery({
    queryKey: queryKeys.coupons.seckill,
    queryFn: () => couponService.getSeckillCoupons(),
    staleTime: staleTimes.couponsSeckill,
  })
}

/** 从列表缓存里捞详情占位，避免详情页白屏闪一下 */
function findCouponInListCache(
  qc: ReturnType<typeof useQueryClient>,
  id: string
): Coupon | undefined {
  const available = qc.getQueryData<Coupon[]>(queryKeys.coupons.available)
  const seckill = qc.getQueryData<Coupon[]>(queryKeys.coupons.seckill)
  return available?.find((c) => c.id === id) ?? seckill?.find((c) => c.id === id)
}

// 优惠券详情：有列表缓存时先展示占位，再后台刷新
export function useCouponDetail(id?: string) {
  const qc = useQueryClient()
  return useQuery({
    queryKey: queryKeys.coupons.detail(id ?? ''),
    queryFn: () => couponService.getCouponById(id as string),
    enabled: !!id,
    staleTime: staleTimes.couponsDetail,
    placeholderData: () => (id ? findCouponInListCache(qc, id) : undefined),
  })
}
