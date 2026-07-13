import { useMutation, useQueryClient } from '@tanstack/react-query'
import { orderService } from '../services/orderService'
import { queryKeys } from '../lib/queryClient'

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

/** 指数退避：200ms → 400 → 800 → … 上限 2s；总尝试约 10 次 */
function backoffMs(attempt: number): number {
  return Math.min(200 * 2 ** attempt, 2000)
}

export interface SeckillArgs {
  couponId: string
}

export function useSeckill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ couponId }: SeckillArgs): Promise<boolean> => {
      // 热路径立即返回 QUEUED；REJECTED 由 http 解包抛 ApiError
      await orderService.seckill(couponId)
      // 轮询落单结果：指数退避，超时视为未成功，禁止假成功
      for (let i = 0; i < 10; i++) {
        const received = await orderService.checkReceived(couponId).catch(() => false)
        if (received) return true
        await sleep(backoffMs(i))
      }
      throw new Error('秒杀受理中，订单尚未确认，请稍后在「我的订单」查看')
    },
    onSuccess: () => {
      // 合并失效：一次刷掉 available + seckill 列表
      qc.invalidateQueries({ queryKey: queryKeys.coupons.root })
      qc.invalidateQueries({ queryKey: queryKeys.orders.all })
    },
  })
}
