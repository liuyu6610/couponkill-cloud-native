import React from 'react'
import { Card, Typography, Button, Tag, Badge, Space } from 'antd'
import { ShoppingCartOutlined } from '@ant-design/icons'
import { Link } from 'react-router-dom'

const { Title, Text, Paragraph } = Typography

export interface Coupon {
  id: string
  name: string
  description: string
  type: 'DISCOUNT' | 'CASH' | 'PERCENTAGE'
  value: number
  minAmount?: number
  maxDiscount?: number
  availableStock: number
  totalStock: number
  startTime: string
  endTime: string
  status: 'ACTIVE' | 'INACTIVE' | 'EXPIRED'
  tags: string[]
  seckillPrice?: number
  seckillStock?: number
  seckillParticipants?: number
  maxParticipants?: number
  seckillStartTime?: string
  seckillEndTime?: string
}

interface CouponCardProps {
  coupon: Coupon
  showActions?: boolean
  size?: 'default' | 'small'
}

const CouponCard: React.FC<CouponCardProps> = ({
  coupon,
  showActions = false,
  size = 'default'
}) => {
  const isSeckill = coupon.seckillPrice && coupon.seckillStock
  const isExpired = new Date(coupon.endTime) < new Date()
  const isNotStarted = new Date(coupon.startTime) > new Date()

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'ACTIVE':
        return 'green'
      case 'INACTIVE':
        return 'orange'
      case 'EXPIRED':
        return 'red'
      default:
        return 'default'
    }
  }

  const getStatusText = (status: string) => {
    switch (status) {
      case 'ACTIVE':
        return '进行中'
      case 'INACTIVE':
        return '未开始'
      case 'EXPIRED':
        return '已过期'
      default:
        return status
    }
  }

  const formatValue = (value: number, type: string) => {
    if (type === 'PERCENTAGE') {
      return `${value}% OFF`
    } else {
      return `¥${value}`
    }
  }

  const cardContent = (
    <Card
      className={`coupon-card ${size === 'small' ? 'small' : ''} ${isExpired ? 'expired' : ''}`}
      hoverable={!isExpired}
      size={size === 'small' ? 'small' : 'default'}
    >
      <div className="coupon-header">
        <div className="coupon-title">
          <Title level={size === 'small' ? 5 : 4} className="coupon-name">
            {coupon.name}
          </Title>
          {isSeckill && (
            <Badge.Ribbon text="秒杀" color="red">
              <div />
            </Badge.Ribbon>
          )}
        </div>

        <div className="coupon-status">
          <Tag color={getStatusColor(coupon.status)}>
            {getStatusText(coupon.status)}
          </Tag>
        </div>
      </div>

      <div className="coupon-content">
        <Paragraph
          className="coupon-description"
          ellipsis={{ rows: size === 'small' ? 1 : 2 }}
        >
          {coupon.description}
        </Paragraph>

        <div className="coupon-value">
          <div className="value-main">
            <Text strong style={{ fontSize: size === 'small' ? '16px' : '20px', color: '#1890ff' }}>
              {formatValue(coupon.value, coupon.type)}
            </Text>
            {isSeckill && coupon.seckillPrice && (
              <div className="seckill-price">
                <Text delete style={{ fontSize: '12px' }}>
                  ¥{coupon.value}
                </Text>
                <Text strong style={{ fontSize: '14px', color: '#ff4d4f', marginLeft: 8 }}>
                  ¥{coupon.seckillPrice}
                </Text>
              </div>
            )}
          </div>

          {coupon.minAmount && (
            <Text type="secondary" style={{ fontSize: '12px' }}>
              满¥{coupon.minAmount}使用
            </Text>
          )}
        </div>

        <div className="coupon-stock">
          <Space size="middle">
            <Text type="secondary" style={{ fontSize: '12px' }}>
              库存: {coupon.availableStock}/{coupon.totalStock}
            </Text>
            <Text type="secondary" style={{ fontSize: '12px' }}>
              有效期至: {new Date(coupon.endTime).toLocaleDateString()}
            </Text>
          </Space>
        </div>

        {isSeckill && coupon.seckillParticipants !== undefined && (
          <div className="seckill-info">
            <Text type="secondary" style={{ fontSize: '12px' }}>
              已参与: {coupon.seckillParticipants}人
            </Text>
          </div>
        )}

        <div className="coupon-tags">
          {coupon.tags.slice(0, size === 'small' ? 2 : 3).map((tag, index) => (
            <Tag key={index}>{tag}</Tag>
          ))}
        </div>
      </div>

      {showActions && (
        <div className="coupon-actions">
          <Space direction="vertical" style={{ width: '100%' }}>
            <Button
              type="primary"
              block
              icon={<ShoppingCartOutlined />}
              disabled={isExpired || isNotStarted || coupon.availableStock === 0}
            >
              {isSeckill ? '立即秒杀' : '立即抢购'}
            </Button>
            <Button block>
              <Link to={`/coupons/${coupon.id}`}>
                查看详情
              </Link>
            </Button>
          </Space>
        </div>
      )}
    </Card>
  )

  return cardContent
}

export default CouponCard
