import React, { useEffect } from 'react'
import { Card, Typography, Avatar, Button, Row, Col, List, Tag } from 'antd'
import { UserOutlined, EditOutlined } from '@ant-design/icons'
import { useDispatch, useSelector } from 'react-redux'
import { Routes, Route, useNavigate } from 'react-router-dom'
import { fetchUserProfile, fetchUserCoupons, fetchUserStats } from '../store/slices/userSlice'
import type { RootState } from '../store'

const { Title, Text } = Typography
const UserCenter: React.FC = () => {
  const dispatch = useDispatch()
  const navigate = useNavigate()
  const { profile, coupons, stats, isLoading } = useSelector((state: RootState) => state.user)

  useEffect(() => {
    dispatch(fetchUserProfile() as any)
    dispatch(fetchUserCoupons({}) as any)
    dispatch(fetchUserStats() as any)
  }, [dispatch])

  const getCouponStatusText = (status: string) => {
    const statusMap: Record<string, string> = {
      AVAILABLE: '可用',
      USED: '已使用',
      EXPIRED: '已过期'
    }
    return statusMap[status] || status
  }

  const getCouponStatusColor = (status: string) => {
    const colorMap: Record<string, string> = {
      AVAILABLE: 'green',
      USED: 'blue',
      EXPIRED: 'red'
    }
    return colorMap[status] || 'default'
  }

  if (isLoading && !profile) {
    return (
      <div className="loading-container">
        <div style={{ textAlign: 'center', padding: '50px' }}>
          <div>加载中...</div>
        </div>
      </div>
    )
  }

  return (
    <div className="user-center-page">
      <div className="container">
        <Routes>
          <Route path="/" element={
            <>
              {/* 用户资料卡片 */}
              <Card style={{ marginBottom: 24 }}>
                <div className="user-profile">
                  <div className="profile-left">
                    <Avatar size={80} icon={<UserOutlined />} src={profile?.avatar} />
                    <div className="profile-info">
                      <Title level={3}>{profile?.username}</Title>
                      <Text type="secondary">{profile?.email}</Text>
                      <br />
                      <Text type="secondary">
                        注册时间：{profile?.createdAt ? new Date(profile.createdAt).toLocaleDateString() : ''}
                      </Text>
                    </div>
                  </div>
                  <div className="profile-actions">
                    <Button
                      icon={<EditOutlined />}
                      onClick={() => navigate('/user/profile')}
                    >
                      编辑资料
                    </Button>
                  </div>
                </div>
              </Card>

              {/* 统计数据 */}
              <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
                <Col xs={24} sm={8}>
                  <Card>
                    <div style={{ textAlign: 'center' }}>
                      <div style={{ fontSize: '24px', fontWeight: 'bold', color: '#1890ff' }}>
                        {stats?.totalCoupons || 0}
                      </div>
                      <Text type="secondary">累计获得优惠券</Text>
                    </div>
                  </Card>
                </Col>
                <Col xs={24} sm={8}>
                  <Card>
                    <div style={{ textAlign: 'center' }}>
                      <div style={{ fontSize: '24px', fontWeight: 'bold', color: '#52c41a' }}>
                        {stats?.availableCoupons || 0}
                      </div>
                      <Text type="secondary">可用优惠券</Text>
                    </div>
                  </Card>
                </Col>
                <Col xs={24} sm={8}>
                  <Card>
                    <div style={{ textAlign: 'center' }}>
                      <div style={{ fontSize: '24px', fontWeight: 'bold', color: '#faad14' }}>
                        ¥{stats?.totalSavings?.toFixed(2) || '0.00'}
                      </div>
                      <Text type="secondary">累计节省金额</Text>
                    </div>
                  </Card>
                </Col>
              </Row>

              {/* 我的优惠券 */}
              <Card>
                <div className="section-header">
                  <Title level={4}>我的优惠券</Title>
                  <Button onClick={() => navigate('/user/coupons')}>
                    查看全部
                  </Button>
                </div>

                <List
                  dataSource={coupons.slice(0, 5)}
                  renderItem={coupon => (
                    <List.Item>
                      <List.Item.Meta
                        title={
                          <div>
                            <Text strong>{coupon.couponName}</Text>
                            <Tag
                              color={getCouponStatusColor(coupon.status)}
                              style={{ marginLeft: 8 }}
                            >
                              {getCouponStatusText(coupon.status)}
                            </Tag>
                          </div>
                        }
                        description={
                          <div>
                            <Text type="secondary">
                              获得时间：{new Date(coupon.obtainedAt).toLocaleDateString()}
                            </Text>
                            <br />
                            <Text type="secondary">
                              到期时间：{new Date(coupon.expiresAt).toLocaleDateString()}
                            </Text>
                          </div>
                        }
                      />
                    </List.Item>
                  )}
                  locale={{
                    emptyText: '暂无优惠券'
                  }}
                />
              </Card>
            </>
          } />

          <Route path="/profile" element={<div>编辑资料页面（待实现）</div>} />
          <Route path="/coupons" element={<div>优惠券管理页面（待实现）</div>} />
          <Route path="/orders" element={<div>订单管理页面（待实现）</div>} />
        </Routes>
      </div>
    </div>
  )
}

export default UserCenter
