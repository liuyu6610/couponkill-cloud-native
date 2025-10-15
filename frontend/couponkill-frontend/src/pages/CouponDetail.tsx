import React, { useEffect } from 'react'
import { Row, Col, Card, Typography, Button, Descriptions, Tag, Badge, Divider } from 'antd'
import { ArrowLeftOutlined, ShoppingCartOutlined } from '@ant-design/icons'
import { useParams, useNavigate } from 'react-router-dom'
import { useDispatch, useSelector } from 'react-redux'
import { fetchCouponDetail } from '../store/slices/couponSlice'
import type { RootState } from '../store'

const { Title, Text, Paragraph } = Typography

const CouponDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const dispatch = useDispatch()
  const { currentCoupon, isLoading } = useSelector((state: RootState) => state.coupon)

  useEffect(() => {
    if (id) {
      dispatch(fetchCouponDetail(id) as any)
    }
  }, [dispatch, id])

  if (isLoading) {
    return (
      <div className="loading-container">
        <div style={{ textAlign: 'center', padding: '50px' }}>
          <div>加载中...</div>
        </div>
      </div>
    )
  }

  if (!currentCoupon) {
    return (
      <div className="coupon-detail-page">
        <div className="container">
          <div style={{ textAlign: 'center', padding: '50px' }}>
            <Text type="secondary">优惠券不存在</Text>
            <br />
            <Button type="primary" onClick={() => navigate('/coupons')}>
              返回优惠券列表
            </Button>
          </div>
        </div>
      </div>
    )
  }

  const isSeckill = !!currentCoupon.seckillStartTime

  return (
    <div className="coupon-detail-page">
      <div className="container">
        {/* 返回按钮 */}
        <Button
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate('/coupons')}
          style={{ marginBottom: 24 }}
        >
          返回列表
        </Button>

        <Row gutter={[24, 24]}>
          {/* 优惠券详情 */}
          <Col xs={24} lg={16}>
            <Card>
              <div className="coupon-detail-header">
                <div className="coupon-detail-title">
                  <Title level={2}>{currentCoupon.name}</Title>
                  {isSeckill && (
                    <Badge.Ribbon text="秒杀" color="red">
                      <div />
                    </Badge.Ribbon>
                  )}
                </div>

                <div className="coupon-detail-status">
                  <Tag color={
                    currentCoupon.status === 'ACTIVE' ? 'green' :
                    currentCoupon.status === 'INACTIVE' ? 'orange' : 'red'
                  }>
                    {currentCoupon.status === 'ACTIVE' ? '进行中' :
                     currentCoupon.status === 'INACTIVE' ? '未开始' : '已过期'}
                  </Tag>
                </div>
              </div>

              <Divider />

              <div className="coupon-detail-content">
                <Paragraph className="coupon-description">
                  {currentCoupon.description}
                </Paragraph>

                <Descriptions column={2} bordered>
                  <Descriptions.Item label="优惠类型">
                    {currentCoupon.type === 'DISCOUNT' ? '折扣券' :
                     currentCoupon.type === 'CASH' ? '现金券' : '百分比券'}
                  </Descriptions.Item>
                  <Descriptions.Item label="优惠价值">
                    {currentCoupon.type === 'PERCENTAGE' ?
                      `${currentCoupon.value}%` :
                      `¥${currentCoupon.value}`}
                  </Descriptions.Item>

                  <Descriptions.Item label="库存">
                    {currentCoupon.availableStock}/{currentCoupon.totalStock}
                  </Descriptions.Item>
                  <Descriptions.Item label="使用门槛">
                    {currentCoupon.minAmount ? `满¥${currentCoupon.minAmount}` : '无门槛'}
                  </Descriptions.Item>

                  {currentCoupon.maxDiscount && (
                    <Descriptions.Item label="最高折扣">
                      ¥{currentCoupon.maxDiscount}
                    </Descriptions.Item>
                  )}

                  <Descriptions.Item label="开始时间">
                    {new Date(currentCoupon.startTime).toLocaleString()}
                  </Descriptions.Item>

                  <Descriptions.Item label="结束时间">
                    {new Date(currentCoupon.endTime).toLocaleString()}
                  </Descriptions.Item>
                </Descriptions>

                {isSeckill && (
                  <>
                    <Divider>秒杀信息</Divider>
                    <Descriptions column={2} bordered>
                      <Descriptions.Item label="秒杀价格">
                        ¥{(currentCoupon as any).seckillPrice}
                      </Descriptions.Item>
                      <Descriptions.Item label="秒杀库存">
                        {(currentCoupon as any).seckillStock}
                      </Descriptions.Item>
                      <Descriptions.Item label="参与人数">
                        {(currentCoupon as any).seckillParticipants}
                      </Descriptions.Item>
                      {(currentCoupon as any).maxParticipants && (
                        <Descriptions.Item label="限购人数">
                          {(currentCoupon as any).maxParticipants}
                        </Descriptions.Item>
                      )}
                    </Descriptions>
                  </>
                )}

                {/* 标签 */}
                <div className="coupon-tags">
                  <Text strong>标签：</Text>
                  {currentCoupon.tags.map((tag, index) => (
                    <Tag key={index} color="blue">{tag}</Tag>
                  ))}
                </div>
              </div>
            </Card>
          </Col>

          {/* 操作面板 */}
          <Col xs={24} lg={8}>
            <Card>
              <div className="coupon-actions">
                <div className="price-info">
                  <Text type="secondary">优惠价值</Text>
                  <div className="price">
                    <Text strong style={{ fontSize: '24px', color: '#1890ff' }}>
                      {currentCoupon.type === 'PERCENTAGE' ?
                        `${currentCoupon.value}% OFF` :
                        `¥${currentCoupon.value}`}
                    </Text>
                  </div>
                </div>

                <div className="stock-info">
                  <Text type="secondary">剩余库存</Text>
                  <div className="stock">
                    <Text strong style={{ color: currentCoupon.availableStock > 0 ? '#52c41a' : '#ff4d4f' }}>
                      {currentCoupon.availableStock} / {currentCoupon.totalStock}
                    </Text>
                  </div>
                </div>

                <Divider />

                <div className="action-buttons">
                  <Button
                    type="primary"
                    size="large"
                    block
                    icon={<ShoppingCartOutlined />}
                    disabled={currentCoupon.availableStock === 0 || currentCoupon.status !== 'ACTIVE'}
                  >
                    {isSeckill ? '立即秒杀' : '立即抢购'}
                  </Button>

                  <Button size="large" block>
                    分享给好友
                  </Button>
                </div>

                <div className="tips">
                  <Text type="secondary" style={{ fontSize: '12px' }}>
                    {currentCoupon.status === 'ACTIVE' && currentCoupon.availableStock > 0 ?
                      '优惠券数量有限，先到先得' :
                      currentCoupon.status === 'EXPIRED' ?
                      '该优惠券已过期' :
                      '该优惠券暂未开始或已售罄'
                    }
                  </Text>
                </div>
              </div>
            </Card>
          </Col>
        </Row>

        {/* 相关推荐 */}
        <div className="related-section">
          <Title level={3}>相关推荐</Title>
          <Row gutter={[16, 16]}>
            {/* 这里可以添加相关优惠券的推荐逻辑 */}
            <Col span={24}>
              <Text type="secondary">暂无相关推荐</Text>
            </Col>
          </Row>
        </div>
      </div>
    </div>
  )
}

export default CouponDetail
