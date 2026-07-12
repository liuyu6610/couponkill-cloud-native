import { useMutation, useQueryClient } from '@tanstack/react-query'
import { orderService } from '../services/orderService'
import { queryKeys } from '../lib/queryClient'

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

export interface SeckillArgs {
  couponId: string
}

export function useSeckill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ couponId }: SeckillArgs): Promise<boolean> => {
      // 热路径立即返回 QUEUED；REJECTED 由 http 解包抛 ApiError
      await orderService.seckill(couponId)
      // 轮询落单结果：超时视为未成功，禁止假成功
      for (let i = 0; i < 10; i++) {
        const received = await orderService.checkReceived(couponId).catch(() => false)
        if (received) return true
        await sleep(500)
      }
      throw new Error('秒杀受理中，订单尚未确认，请稍后在「我的订单」查看')
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.coupons.available })
      qc.invalidateQueries({ queryKey: queryKeys.coupons.seckill })
      qc.invalidateQueries({ queryKey: queryKeys.orders.all })
    },
  })
}
