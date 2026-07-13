import React, { useEffect, useState } from 'react'
import { Typography, Alert } from 'antd'
import { InfoCircleOutlined, ClockCircleOutlined } from '@ant-design/icons'

const { Text } = Typography

interface SeckillCountdownProps {
  /** 开售时间 */
  startTime?: string | number | null
  /** 结束时间 */
  endTime?: string | number | null
  title?: string
}

/** 兼容后端 JsonFormat `yyyy-MM-dd HH:mm:ss`（Safari 对空格分隔不友好） */
function parseTs(v?: string | number | null): number | null {
  if (v == null || v === '') return null
  if (typeof v === 'number') return Number.isNaN(v) ? null : v
  const normalized = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}/.test(v) ? v.replace(' ', 'T') : v
  const n = Date.parse(normalized)
  return Number.isNaN(n) ? null : n
}

function formatRemain(remainMs: number): string {
  const totalSec = Math.max(0, Math.floor(remainMs / 1000))
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${pad(h)}:${pad(m)}:${pad(s)}`
}

/**
 * 秒杀时间窗倒计时：接真实 seckillStartAt / seckillEndAt；无窗口则诚实提示。
 */
const SeckillCountdown: React.FC<SeckillCountdownProps> = ({
  startTime,
  endTime,
  title = '秒杀活动',
}) => {
  const start = parseTs(startTime)
  const end = parseTs(endTime)
  const [, setTick] = useState(0)

  useEffect(() => {
    if (start == null && end == null) return
    const t = window.setInterval(() => setTick((x) => x + 1), 1000)
    return () => window.clearInterval(t)
  }, [start, end])

  if (start == null || end == null) {
    return (
      <Alert
        type="info"
        showIcon
        icon={<InfoCircleOutlined />}
        message={title}
        description={
          <Text type="secondary">
            活动以实时库存为准，先到先得。该场次尚未配置活动时间窗，请勿依赖倒计时判断开抢/结束。
          </Text>
        }
      />
    )
  }

  const now = Date.now()
  if (now < start) {
    return (
      <Alert
        type="info"
        showIcon
        icon={<ClockCircleOutlined />}
        message={title}
        description={
          <Text>
            距开抢还剩 {formatRemain(start - now)} · 可预约帮抢，到点系统代发本站秒杀入队
          </Text>
        }
      />
    )
  }

  if (now > end) {
    return (
      <Alert
        type="warning"
        showIcon
        message="本场活动已结束"
        description="请刷新页面查看是否有新库存或其它场次。"
      />
    )
  }

  return (
    <Alert
      type="success"
      showIcon
      message={title}
      description={
        <Text>
          开售中 · 距结束约 {formatRemain(end - now)}（以服务端时间为准，请及时抢购）
        </Text>
      }
    />
  )
}

export default SeckillCountdown
