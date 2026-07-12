import React, { useState } from 'react'
import { Statistic } from 'antd'

const { Countdown } = Statistic

// 计算下一个整点/半点的时间戳（作为“本场秒杀”的结束点）
function nextRoundDeadline(): number {
  const now = new Date()
  const d = new Date(now)
  if (now.getMinutes() < 30) {
    d.setMinutes(30, 0, 0)
  } else {
    d.setHours(now.getHours() + 1, 0, 0, 0)
  }
  return d.getTime()
}

interface SeckillCountdownProps {
  title?: string
}

// 本场秒杀倒计时。注意：后端当前无秒杀时间窗字段，这里为客户端“场次”计时，
// 到点后自动开启下一场（后续后端补充活动时间后可替换为真实 deadline）。
const SeckillCountdown: React.FC<SeckillCountdownProps> = ({ title = '本场秒杀剩余' }) => {
  const [deadline, setDeadline] = useState<number>(() => nextRoundDeadline())

  return (
    <Countdown
      title={title}
      value={deadline}
      format="HH:mm:ss"
      onFinish={() => setDeadline(nextRoundDeadline())}
    />
  )
}

export default SeckillCountdown
