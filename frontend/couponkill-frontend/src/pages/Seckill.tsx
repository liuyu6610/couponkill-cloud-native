import React, { useRef, useState } from 'react'
import { Row, Col, Typography, Empty, Spin, Card, App } from 'antd'
import { ThunderboltOutlined } from '@ant-design/icons'
import { useSelector } from 'react-redux'
import { useNavigate } from 'react-router-dom'
import type { RootState } from '../store'
import CouponCard from '../components/CouponCard'
import SeckillCountdown from '../components/SeckillCountdown'
import { useSeckillCoupons } from '../hooks/useCoupons'
import { useSeckill } from '../hooks/useSeckill'
import type { Coupon } from '../types/api'

const { Title, Text } = Typography

// 节流窗口：同一秒内的重复点击直接忽略，防止手抖/连点
const THROTTLE_MS = 1000

const Seckill: React.FC = () => {
  const navigate = useNavigate()
  const { message } = App.useApp()
  const { isAuthenticated, user } = useSelector((state: RootState) => state.auth)

  const { data: seckillCoupons = [], isLoading } = useSeckillCoupons()
  const seckill = useSeckill()
  const [seckillingId, setSeckillingId] = useState<string | null>(null)
  const lastClickRef = useRef<number>(0)

  const handleSeckill = (coupon: Coupon) => {
    if (!isAuthenticated || !user) {
      message.info('请先登录后再参与秒杀')
      navigate('/login', { state: { from: { pathname: '/seckill' } } })
      return
    }
    // 节流 + 进行中防抖
    const now = Date.now()
    if (now - lastClickRef.current < THROTTLE_MS || seckill.isPending) {
      return
    }
    lastClickRef.current = now

    setSeckillingId(coupon.id)
    seckill.mutate(
      { couponId: coupon.id },
      {
        onSuccess: () => message.success('秒杀成功！可在“我的订单”中查看'),
        onError: (err) => message.error((err as Error).message || '秒杀失败，请稍后重试'),
        onSettled: () => setSeckillingId(null),
      }
    )
  }

  return (
    <div className="seckill-page">
      <div className="container">
        <div className="page-header">
          <Title level={2}>
            <ThunderboltOutlined /> 秒杀专区
          </Title>
          <Text type="secondary">每日限时秒杀，超值优惠不容错过</Text>
        </div>

        <Card style={{ margin: '24px 0', textAlign: 'center' }}>
          <SeckillCountdown title="本场秒杀剩余" />
          <Text type="secondary" style={{ display: 'block', marginTop: 8 }}>
            每人每券限领 1 张，秒杀成功后进入短暂冷却，请勿重复提交
          </Text>
        </Card>

        <div className="seckill-products">
          {isLoading ? (
            <div style={{ textAlign: 'center', padding: 50 }}>
              <Spin size="large" />
            </div>
          ) : seckillCoupons.length > 0 ? (
            <Row gutter={[16, 16]}>
              {seckillCoupons.map((coupon) => (
                <Col xs={24} sm={12} lg={8} key={coupon.id}>
                  <CouponCard
                    coupon={coupon}
                    showActions
                    seckillLoading={seckillingId === coupon.id}
                    onSeckill={handleSeckill}
                  />
                </Col>
              ))}
            </Row>
          ) : (
            <Empty description="暂无秒杀活动" image={Empty.PRESENTED_IMAGE_SIMPLE} />
          )}
        </div>
      </div>
    </div>
  )
}

export default Seckill
