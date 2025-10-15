import React, { useEffect } from 'react'
import { Row, Col, Typography, Badge } from 'antd'
import { ThunderboltOutlined, ClockCircleOutlined } from '@ant-design/icons'
import { useDispatch, useSelector } from 'react-redux'
import { fetchSeckillCoupons } from '../store/slices/couponSlice'
import type { RootState } from '../store'
import CouponCard from '../components/CouponCard'

const { Title, Text } = Typography

const Seckill: React.FC = () => {
  const dispatch = useDispatch()
  const { seckillCoupons, isLoading } = useSelector((state: RootState) => state.coupon)

  useEffect(() => {
    dispatch(fetchSeckillCoupons() as any)
  }, [dispatch])

  return (
    <div className="seckill-page">
      <div className="container">
        {/* 页面标题 */}
        <div className="page-header">
          <Title level={2}>
            <ThunderboltOutlined /> 秒杀专区
          </Title>
          <Text type="secondary">
            每日限时秒杀，超值优惠不容错过
          </Text>
        </div>

        {/* 秒杀介绍 */}
        <div className="seckill-intro">
          <div className="intro-card">
            <div className="intro-icon">
              <ClockCircleOutlined />
            </div>
            <div className="intro-content">
              <Title level={4}>限时秒杀规则</Title>
              <ul>
                <li>每日多场秒杀活动，每场限时抢购</li>
                <li>每个用户每场限购1件，先到先得</li>
                <li>秒杀商品享受专属折扣，优惠力度更大</li>
                <li>抢购成功后请及时支付，逾期将自动取消</li>
              </ul>
            </div>
          </div>
        </div>

        {/* 秒杀商品列表 */}
        <div className="seckill-products">
          <Row gutter={[16, 16]}>
            {seckillCoupons.map((coupon) => (
              <Col xs={24} sm={12} lg={8} key={coupon.id}>
                <Badge.Ribbon text="秒杀中" color="red">
                  <CouponCard coupon={coupon} showActions={true} />
                </Badge.Ribbon>
              </Col>
            ))}
          </Row>

          {seckillCoupons.length === 0 && !isLoading && (
            <div style={{ textAlign: 'center', padding: '50px' }}>
              <Text type="secondary">暂无秒杀活动</Text>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

export default Seckill
