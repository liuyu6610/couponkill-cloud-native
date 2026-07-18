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

/**
 * 解析 /seckill/result 返回值。
 * SUCCESS / SUCCESS:orderId → 成功；FAIL* → 失败；其余视为进行中。
 */
export function interpretSeckillResult(result: string | null | undefined): 'pending' | 'success' | 'fail' {
  if (!result || result === 'PENDING' || result === 'UNKNOWN') return 'pending'
  if (result.startsWith('SUCCESS')) return 'success'
  if (result.startsWith('FAIL')) return 'fail'
  return 'pending'
}

export function useSeckill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ couponId }: SeckillArgs): Promise<boolean> => {
      // 热路径立即返回 QUEUED + requestId；业务拒绝由 http 解包抛 ApiError
      const enter = await orderService.seckill(couponId)
      const requestId = enter?.requestId?.trim()
      if (!requestId) {
        // 无 requestId 时降级旧 check 路径，避免沙箱/异常响应导致前端卡死
        for (let i = 0; i < 10; i++) {
          const received = await orderService.checkReceived(couponId).catch(() => false)
          if (received) return true
          await sleep(backoffMs(i))
        }
        throw new Error('秒杀受理中，订单尚未确认，请稍后在「我的订单」查看')
      }

      for (let i = 0; i < 10; i++) {
        const raw = await orderService.seckillResult(requestId).catch(() => 'PENDING')
        const kind = interpretSeckillResult(raw)
        if (kind === 'success') return true
        if (kind === 'fail') {
          throw new Error('秒杀未成功，库存或资格可能已变化，请稍后重试')
        }
        await sleep(backoffMs(i))
      }
      throw new Error('秒杀受理中，订单尚未确认，请稍后在「我的订单」查看')
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.coupons.root })
      qc.invalidateQueries({ queryKey: queryKeys.orders.all })
    },
  })
}
