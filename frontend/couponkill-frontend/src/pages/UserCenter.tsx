import React, { useMemo } from 'react'
import { Card, Typography, Avatar, Button, Row, Col, List, Tag, Spin } from 'antd'
import { UserOutlined } from '@ant-design/icons'
import { useSelector } from 'react-redux'
import { useNavigate } from 'react-router-dom'
import type { RootState } from '../store'
import { useUserProfile } from '../hooks/useUser'
import { useUserOrders } from '../hooks/useOrders'
import { OrderStatus, orderStatusColor, orderStatusText } from '../types/api'

const { Title, Text } = Typography

const UserCenter: React.FC = () => {
  const navigate = useNavigate()
  const { user } = useSelector((state: RootState) => state.auth)

  const { data: profile, isLoading } = useUserProfile(user?.id)
  const { data: orders = [] } = useUserOrders(user?.id)

  // 用户“优惠券”统计由订单派生（每个订单 = 一次领取）
  const stats = useMemo(() => {
    return {
      total: orders.length,
      active: orders.filter((o) => o.status === OrderStatus.CREATED).length,
      used: orders.filter((o) => o.status === OrderStatus.USED).length,
    }
  }, [orders])

  if (isLoading && !profile) {
    return (
      <div className="loading-container" style={{ textAlign: 'center', padding: 50 }}>
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div className="user-center-page">
      <div className="container">
        <Card style={{ marginBottom: 24 }}>
          <div className="user-profile" style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
            <Avatar size={80} icon={<UserOutlined />} />
            <div className="profile-info">
              <Title level={3} style={{ marginBottom: 4 }}>
                {profile?.username || user?.username}
              </Title>
              {profile?.phone && <Text type="secondary">手机号：{profile.phone}</Text>}
              <br />
              <Text type="secondary">
                注册时间：{profile?.createTime ? new Date(profile.createTime).toLocaleDateString() : '-'}
              </Text>
            </div>
          </div>
        </Card>

        <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
          <Col xs={24} sm={8}>
            <Card>
              <div style={{ textAlign: 'center' }}>
                <div style={{ fontSize: 24, fontWeight: 'bold', color: '#fa541c' }}>{stats.total}</div>
                <Text type="secondary">累计领取</Text>
              </div>
            </Card>
          </Col>
          <Col xs={24} sm={8}>
            <Card>
              <div style={{ textAlign: 'center' }}>
                <div style={{ fontSize: 24, fontWeight: 'bold', color: '#52c41a' }}>{stats.active}</div>
                <Text type="secondary">未使用</Text>
              </div>
            </Card>
          </Col>
          <Col xs={24} sm={8}>
            <Card>
              <div style={{ textAlign: 'center' }}>
                <div style={{ fontSize: 24, fontWeight: 'bold', color: '#faad14' }}>{stats.used}</div>
                <Text type="secondary">已使用</Text>
              </div>
            </Card>
          </Col>
        </Row>

        <Card>
          <div className="section-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Title level={4} style={{ margin: 0 }}>
              最近领取
            </Title>
            <Button onClick={() => navigate('/orders')}>查看全部</Button>
          </div>

          <List
            style={{ marginTop: 16 }}
            dataSource={orders.slice(0, 5)}
            locale={{ emptyText: '暂无领取记录' }}
            renderItem={(order) => (
              <List.Item
                actions={[
                  <Button key="detail" type="link" onClick={() => navigate(`/orders/${order.id}`)}>
                    详情
                  </Button>,
                ]}
              >
                <List.Item.Meta
                  title={
                    <span>
                      订单 {order.id}
                      <Tag color={orderStatusColor(order.status)} style={{ marginLeft: 8 }}>
                        {orderStatusText(order.status)}
                      </Tag>
                    </span>
                  }
                  description={`优惠券ID：${order.couponId} · 领取时间：${
                    order.getTime ? new Date(order.getTime).toLocaleString() : '-'
                  }`}
                />
              </List.Item>
            )}
          />
        </Card>
      </div>
    </div>
  )
}

export default UserCenter
