import React from 'react'
import { Row, Col, Card, Typography, Button, Descriptions, Tag, Divider, Spin, App } from 'antd'
import { ArrowLeftOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { useParams, useNavigate } from 'react-router-dom'
import { useSelector } from 'react-redux'
import type { RootState } from '../store'
import { useCouponDetail } from '../hooks/useCoupons'
import { useSeckill } from '../hooks/useSeckill'
import { useCreateOrder } from '../hooks/useOrders'
import { couponStockOf, couponTotalStockOf, couponTypeText, isSeckillCoupon } from '../types/api'

const { Title, Text, Paragraph } = Typography

const CouponDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { message } = App.useApp()
  const { isAuthenticated, user } = useSelector((state: RootState) => state.auth)

  const { data: coupon, isLoading } = useCouponDetail(id)
  const seckill = useSeckill()
  const createOrder = useCreateOrder()
  const acting = seckill.isPending || createOrder.isPending

  if (isLoading) {
    return (
      <div className="loading-container" style={{ textAlign: 'center', padding: 50 }}>
        <Spin size="large" />
      </div>
    )
  }

  if (!coupon) {
    return (
      <div className="coupon-detail-page">
        <div className="container" style={{ textAlign: 'center', padding: 50 }}>
          <Text type="secondary">优惠券不存在</Text>
          <br />
          <Button type="primary" onClick={() => navigate('/coupons')} style={{ marginTop: 16 }}>
            返回优惠券列表
          </Button>
        </div>
      </div>
    )
  }

  const seckillType = isSeckillCoupon(coupon)
  const stock = couponStockOf(coupon)
  const totalStock = couponTotalStockOf(coupon)
  const soldOut = stock <= 0
  const invalid = coupon.status !== 1

  const handleClaim = () => {
    if (!isAuthenticated || !user) {
      message.info('请先登录')
      navigate('/login', { state: { from: { pathname: `/coupons/${coupon.id}` } } })
      return
    }
    if (acting) return
    const args = { couponId: coupon.id }
    const opts = {
      onSuccess: () => message.success('领取成功！可在“我的订单”中查看'),
      onError: (err: unknown) => message.error((err as Error).message || '领取失败，请稍后重试'),
    }
    if (seckillType) {
      seckill.mutate(args, opts)
    } else {
      createOrder.mutate(args, opts)
    }
  }

  return (
    <div className="coupon-detail-page">
      <div className="container">
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/coupons')} style={{ marginBottom: 24 }}>
          返回列表
        </Button>

        <Row gutter={[24, 24]}>
          <Col xs={24} lg={16}>
            <Card>
              <div className="coupon-detail-header">
                <Title level={2}>{coupon.name}</Title>
                <Tag color={seckillType ? 'red' : 'blue'}>{couponTypeText(coupon.type)}</Tag>
                <Tag color={invalid ? 'red' : 'green'}>{invalid ? '已失效' : '有效'}</Tag>
              </div>

              <Divider />

              {coupon.description && <Paragraph>{coupon.description}</Paragraph>}

              <Descriptions column={2} bordered>
                <Descriptions.Item label="面值">¥{coupon.faceValue}</Descriptions.Item>
                <Descriptions.Item label="使用门槛">
                  {coupon.minSpend > 0 ? `满¥${coupon.minSpend}` : '无门槛'}
                </Descriptions.Item>
                <Descriptions.Item label="库存">
                  {stock}/{totalStock}
                </Descriptions.Item>
                <Descriptions.Item label="有效期">{coupon.validDays} 天</Descriptions.Item>
                <Descriptions.Item label="每人限领">{coupon.perUserLimit} 张</Descriptions.Item>
                <Descriptions.Item label="类型">{couponTypeText(coupon.type)}</Descriptions.Item>
              </Descriptions>
            </Card>
          </Col>

          <Col xs={24} lg={8}>
            <Card>
              <div className="price-info">
                <Text type="secondary">优惠面值</Text>
                <div>
                  <Text strong style={{ fontSize: 28, color: '#fa541c' }}>
                    ¥{coupon.faceValue}
                  </Text>
                </div>
              </div>

              <div className="stock-info" style={{ marginTop: 16 }}>
                <Text type="secondary">剩余库存</Text>
                <div>
                  <Text strong style={{ color: soldOut ? '#ff4d4f' : '#52c41a' }}>
                    {stock} / {totalStock}
                  </Text>
                </div>
              </div>

              <Divider />

              <Button
                type="primary"
                danger={seckillType}
                size="large"
                block
                icon={seckillType ? <ThunderboltOutlined /> : undefined}
                loading={acting}
                disabled={soldOut || invalid}
                onClick={handleClaim}
              >
                {soldOut ? '已抢光' : seckillType ? '立即秒杀' : '立即领取'}
              </Button>

              <div className="tips" style={{ marginTop: 12 }}>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {invalid ? '该优惠券已失效' : soldOut ? '该优惠券已售罄' : '数量有限，先到先得'}
                </Text>
              </div>
            </Card>
          </Col>
        </Row>
      </div>
    </div>
  )
}

export default CouponDetail
