import React from 'react'
import { Card, Typography, Button, Tag, Space } from 'antd'
import { ThunderboltOutlined, GiftOutlined, ClockCircleOutlined, CheckOutlined } from '@ant-design/icons'
import { useHref, useLinkClickHandler } from 'react-router-dom'
import { preloadRoute } from '../lib/routePreload'
import type { Coupon } from '../types/api'
import {
  couponStockOf,
  couponTotalStockOf,
  formatMoney,
  isPositiveMoney,
  isSeckillCoupon,
} from '../types/api'
import { useSeckillPhase } from '../hooks/useSeckillPhase'
import SeckillCountdown from './SeckillCountdown'

const { Title, Text, Paragraph } = Typography

interface CouponCardProps {
  coupon: Coupon
  showActions?: boolean
  size?: 'default' | 'small'
  seckillLoading?: boolean
  /** 秒杀进行中的按钮文案（如「排队确认中…」），仅 seckillLoading 时生效 */
  seckillLoadingText?: string
  reserveLoading?: boolean
  /** 该券已有活跃预约（PENDING/FIRING/QUEUED） */
  alreadyReserved?: boolean
  bindingLabel?: string | null
  onSeckill?: (coupon: Coupon) => void
  onReserve?: (coupon: Coupon) => void
  onViewReservations?: () => void
}

const CouponCard: React.FC<CouponCardProps> = ({
  coupon,
  showActions = false,
  size = 'default',
  seckillLoading = false,
  seckillLoadingText,
  reserveLoading = false,
  alreadyReserved = false,
  bindingLabel,
  onSeckill,
  onReserve,
  onViewReservations,
}) => {
  const detailPath = `/coupons/${coupon.id}`
  // 「查看详情」渲染为真正的 <a href>：保留 Ctrl/Cmd/中键新标签打开、右键复制链接与读屏的链接语义；
  // 普通左键由 react-router 的 click handler 拦截为 SPA 跳转（与 <Link> 判定规则一致）
  const detailHref = useHref(detailPath)
  const handleDetailClick = useLinkClickHandler<HTMLElement>(detailPath)
  const seckill = isSeckillCoupon(coupon)
  const stock = couponStockOf(coupon)
  const totalStock = couponTotalStockOf(coupon)
  const soldOut = stock <= 0
  const invalid = coupon.status !== 1
  // 到开售/结束时刻自动切换按钮，无需刷新
  const phase = useSeckillPhase(coupon, seckill)

  const renderSeckillCta = () => {
    if (phase === 'no_window') {
      return (
        <Button block disabled>
          未配置时间窗
        </Button>
      )
    }
    if (phase === 'upcoming') {
      if (alreadyReserved) {
        return (
          <Button
            type="default"
            block
            icon={<CheckOutlined />}
            onClick={() => onViewReservations?.()}
          >
            已预约 · 查看
          </Button>
        )
      }
      return (
        <Button
          type="primary"
          block
          icon={<ClockCircleOutlined />}
          loading={reserveLoading}
          disabled={invalid}
          onClick={() => onReserve?.(coupon)}
        >
          预约帮抢
        </Button>
      )
    }
    return (
      <Button
        type="primary"
        danger
        block
        icon={<ThunderboltOutlined />}
        loading={seckillLoading}
        disabled={soldOut || invalid || phase === 'ended'}
        onClick={() => onSeckill?.(coupon)}
      >
        {seckillLoading && seckillLoadingText
          ? seckillLoadingText
          : phase === 'ended'
            ? '已结束'
            : soldOut
              ? '已抢光'
              : '立即秒杀'}
      </Button>
    )
  }

  return (
    <Card
      className={`coupon-card ${size === 'small' ? 'small' : ''} ${soldOut ? 'sold-out' : ''}`}
      hoverable={!soldOut && !invalid}
      size={size === 'small' ? 'small' : 'default'}
    >
      <div className="coupon-header">
        <Title level={size === 'small' ? 5 : 4} className="coupon-name" ellipsis={{ rows: 1 }}>
          {coupon.name}
        </Title>
        <Tag color={seckill ? 'red' : 'blue'} icon={seckill ? <ThunderboltOutlined /> : <GiftOutlined />}>
          {seckill ? '秒抢' : '常驻'}
        </Tag>
      </div>

      <div className="coupon-content">
        {coupon.description && (
          <Paragraph className="coupon-description" type="secondary" ellipsis={{ rows: size === 'small' ? 1 : 2 }}>
            {coupon.description}
          </Paragraph>
        )}

        <div className="coupon-value">
          <Text strong style={{ fontSize: size === 'small' ? 18 : 24, color: '#fa541c' }}>
            ¥{formatMoney(coupon.faceValue)}
          </Text>
          {isPositiveMoney(coupon.minSpend) && (
            <Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>
              满¥{formatMoney(coupon.minSpend)}可用
            </Text>
          )}
        </div>

        <div className="coupon-meta">
          <Space size="middle" wrap>
            <Text type="secondary" style={{ fontSize: 12 }}>
              库存: {stock}/{totalStock}
            </Text>
            <Text type="secondary" style={{ fontSize: 12 }}>
              有效期: {coupon.validDays}天
            </Text>
          </Space>
        </div>

        {seckill && size !== 'small' && (
          <div style={{ marginTop: 8 }}>
            <SeckillCountdown
              title="本场时间窗"
              startTime={coupon.seckillStartAt}
              endTime={coupon.seckillEndAt}
            />
          </div>
        )}

        {bindingLabel != null && (
          <Text type="secondary" style={{ display: 'block', marginTop: 8, fontSize: 12 }}>
            {bindingLabel || '未绑定外部商品'}
          </Text>
        )}
      </div>

      {showActions && (
        <div className="coupon-actions" style={{ marginTop: 12 }}>
          <Space direction="vertical" style={{ width: '100%' }}>
            {seckill ? renderSeckillCta() : null}
            {/* 单一链接样式按钮（antd Button 传 href 时渲染为 <a>），避免 <Button><Link/></Button> 嵌套交互元素；悬停预取详情 chunk */}
            <Button
              block
              href={detailHref}
              onClick={handleDetailClick}
              onMouseEnter={() => preloadRoute(detailPath)}
              onFocus={() => preloadRoute(detailPath)}
            >
              查看详情
            </Button>
          </Space>
        </div>
      )}
    </Card>
  )
}

export default CouponCard
