import React from 'react'
import { Card, Typography, Button, Tag, Space } from 'antd'
import { ThunderboltOutlined, GiftOutlined } from '@ant-design/icons'
import { Link } from 'react-router-dom'
import type { Coupon } from '../types/api'
import { couponStockOf, couponTotalStockOf, isSeckillCoupon } from '../types/api'

const { Title, Text, Paragraph } = Typography

interface CouponCardProps {
  coupon: Coupon
  showActions?: boolean
  size?: 'default' | 'small'
  seckillLoading?: boolean
  onSeckill?: (coupon: Coupon) => void
}

const CouponCard: React.FC<CouponCardProps> = ({
  coupon,
  showActions = false,
  size = 'default',
  seckillLoading = false,
  onSeckill,
}) => {
  const seckill = isSeckillCoupon(coupon)
  const stock = couponStockOf(coupon)
  const totalStock = couponTotalStockOf(coupon)
  const soldOut = stock <= 0
  const invalid = coupon.status !== 1

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
            ¥{coupon.faceValue}
          </Text>
          {coupon.minSpend > 0 && (
            <Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>
              满¥{coupon.minSpend}可用
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
      </div>

      {showActions && (
        <div className="coupon-actions" style={{ marginTop: 12 }}>
          <Space direction="vertical" style={{ width: '100%' }}>
            {seckill ? (
              <Button
                type="primary"
                danger
                block
                icon={<ThunderboltOutlined />}
                loading={seckillLoading}
                disabled={soldOut || invalid}
                onClick={() => onSeckill?.(coupon)}
              >
                {soldOut ? '已抢光' : '立即秒杀'}
              </Button>
            ) : null}
            <Button block>
              <Link to={`/coupons/${coupon.id}`}>查看详情</Link>
            </Button>
          </Space>
        </div>
      )}
    </Card>
  )
}

export default CouponCard
