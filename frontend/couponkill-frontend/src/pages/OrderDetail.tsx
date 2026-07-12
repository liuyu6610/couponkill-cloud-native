import React from 'react'
import { Card, Typography, Descriptions, Tag, Button, Spin, App } from 'antd'
import { ArrowLeftOutlined } from '@ant-design/icons'
import { useParams, useNavigate } from 'react-router-dom'
import { useSelector } from 'react-redux'
import type { RootState } from '../store'
import { useUserOrders, useCancelOrder } from '../hooks/useOrders'
import { OrderStatus, orderStatusColor, orderStatusText } from '../types/api'

const { Title, Text } = Typography

const fmt = (t?: string) => (t ? new Date(t).toLocaleString() : '-')

// 后端无单订单查询接口，从用户订单列表中解析
const OrderDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { message } = App.useApp()
  const { user } = useSelector((state: RootState) => state.auth)

  const { data: orders = [], isLoading } = useUserOrders(user?.id)
  const cancelOrder = useCancelOrder()
  const order = orders.find((o) => o.id === id)

  if (isLoading && !order) {
    return (
      <div className="loading-container" style={{ textAlign: 'center', padding: 50 }}>
        <Spin size="large" />
      </div>
    )
  }

  if (!order) {
    return (
      <div className="order-detail-page">
        <div className="container" style={{ textAlign: 'center', padding: 50 }}>
          <Text type="secondary">订单不存在或未加载</Text>
          <br />
          <Button type="primary" onClick={() => navigate('/orders')} style={{ marginTop: 16 }}>
            返回订单列表
          </Button>
        </div>
      </div>
    )
  }

  const handleCancel = () => {
    if (!user) return
    cancelOrder.mutate(
      { orderId: order.id },
      {
        onSuccess: () => message.success('订单已取消'),
        onError: (err) => message.error((err as Error).message || '取消失败'),
      }
    )
  }

  return (
    <div className="order-detail-page">
      <div className="container">
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/orders')} style={{ marginBottom: 24 }}>
          返回订单列表
        </Button>

        <Card>
          <div className="order-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Title level={3} style={{ margin: 0 }}>
              订单详情
            </Title>
            <Tag color={orderStatusColor(order.status)}>{orderStatusText(order.status)}</Tag>
          </div>

          <Descriptions column={2} bordered style={{ marginTop: 24 }}>
            <Descriptions.Item label="订单号" span={2}>
              {order.id}
            </Descriptions.Item>
            <Descriptions.Item label="优惠券ID">{order.couponId}</Descriptions.Item>
            <Descriptions.Item label="分片ID">{order.virtualId || '-'}</Descriptions.Item>
            <Descriptions.Item label="领取时间">{fmt(order.getTime)}</Descriptions.Item>
            <Descriptions.Item label="过期时间">{fmt(order.expireTime)}</Descriptions.Item>
            <Descriptions.Item label="使用时间">{fmt(order.useTime)}</Descriptions.Item>
            <Descriptions.Item label="取消时间">{fmt(order.cancelTime)}</Descriptions.Item>
            <Descriptions.Item label="创建时间">{fmt(order.createTime)}</Descriptions.Item>
            <Descriptions.Item label="来源">{order.createdByGo ? 'Go 秒杀服务' : 'Java 服务'}</Descriptions.Item>
          </Descriptions>

          {order.status === OrderStatus.CREATED && (
            <Button danger style={{ marginTop: 24 }} loading={cancelOrder.isPending} onClick={handleCancel}>
              取消订单
            </Button>
          )}
        </Card>
      </div>
    </div>
  )
}

export default OrderDetail
