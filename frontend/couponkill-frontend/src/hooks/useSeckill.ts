import { useEffect, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { orderService } from '../services/orderService'
import { queryKeys } from '../lib/queryClient'
import {
  SeckillAbortedError,
  SeckillError,
  pollSeckillResult,
  translateEnterError,
} from '../lib/seckill'

// 轮询策略、结果解读与失败原因文案都在 lib/seckill.ts（纯函数，便于单测）。

export interface SeckillArgs {
  couponId: string
}

/** 秒杀成功的结果：requestId 用于排障，orderId 用于直达订单详情（后端偶发缺省时为 undefined） */
export interface SeckillSuccess {
  requestId: string
  orderId?: string
}

/** idle：空闲；submitting：入队请求中；queued：已受理（202），正在轮询落单结果 */
export type SeckillStage = 'idle' | 'submitting' | 'queued'

/**
 * 秒杀 mutation：POST /seckill 入队拿 requestId（202 QUEUED），再有界退避轮询 /seckill/result。
 *
 * - 轮询最多 SECKILL_MAX_POLL_ATTEMPTS（10）次，超出提示到「我的订单」确认；
 * - 受理阶段 409/429/503（售罄/重复/未开始/冷却/限流/繁忙…）转成带 warning 级别的 SeckillError；
 *   终态 FAILED 按 reason 映射成中文文案；401/403/网络等错误原样抛出；
 * - 组件卸载或再次发起时中止上一轮轮询（含进行中的 GET 与退避定时器），避免泄漏请求；
 * - 无论成功/失败/超时都失效券与订单缓存：失败意味着库存变化，超时也可能已落单；
 * - 额外返回 stage，供按钮展示「提交中 / 排队确认中」；stage 只由最新一轮写入，旧轮次收尾不会覆盖。
 */
export function useSeckill() {
  const qc = useQueryClient()
  const abortRef = useRef<AbortController | null>(null)
  const [stage, setStage] = useState<SeckillStage>('idle')

  useEffect(() => {
    return () => {
      abortRef.current?.abort()
    }
  }, [])

  const mutation = useMutation({
    mutationFn: async ({ couponId }: SeckillArgs): Promise<SeckillSuccess> => {
      abortRef.current?.abort()
      const controller = new AbortController()
      abortRef.current = controller
      const { signal } = controller

      // 阶段只允许「当前轮次」写入：被新一轮中止的旧轮次会异步收尾，
      // 若无条件 setStage('idle') 会把新一轮的 submitting/queued 覆盖掉，按钮文案提前回落
      const setRunStage = (next: SeckillStage) => {
        if (abortRef.current === controller) setStage(next)
      }

      try {
        setRunStage('submitting')
        let accepted
        try {
          accepted = await orderService.seckill(couponId)
        } catch (err) {
          throw translateEnterError(err)
        }
        if (signal.aborted) throw new SeckillAbortedError()
        const requestId = accepted?.requestId?.trim()
        if (!requestId) {
          // 202 但没有 requestId：契约被破坏，无法轮询；订单可能已受理，引导去订单页确认
          throw new SeckillError('秒杀已受理但缺少请求号，请稍后在「我的订单」确认', {
            reason: 'NO_REQUEST_ID',
            severity: 'warning',
          })
        }
        setRunStage('queued')

        const { orderId } = await pollSeckillResult({
          requestId,
          signal,
          fetchResult: (s) => orderService.seckillResult(requestId, s),
        })
        return { requestId, orderId }
      } finally {
        setRunStage('idle')
      }
    },
    onError: (err) => {
      if (!(err instanceof SeckillAbortedError)) {
        console.warn('[seckill] 秒杀未完成', err)
      }
    },
    onSettled: () => {
      // stage 已在 mutationFn 的 finally 中按轮次复位，这里只做缓存失效
      void qc.invalidateQueries({ queryKey: queryKeys.coupons.root })
      void qc.invalidateQueries({ queryKey: queryKeys.orders.all })
    },
  })

  return { ...mutation, stage }
}

/** 按秒杀阶段给出按钮文案；非进行中返回 undefined 使用默认文案 */
export function seckillStageText(stage: SeckillStage): string | undefined {
  if (stage === 'submitting') return '提交中…'
  if (stage === 'queued') return '排队确认中…'
  return undefined
}
