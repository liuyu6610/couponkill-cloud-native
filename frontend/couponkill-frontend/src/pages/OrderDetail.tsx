import React, { useEffect } from 'react'
import { Card, Typography, Descriptions, Tag, Button, Row, Col, Divider } from 'antd'
import { ArrowLeftOutlined } from '@ant-design/icons'
import { useParams, useNavigate } from 'react-router-dom'
import { useDispatch, useSelector } from 'react-redux'
import { fetchOrderDetail } from '../store/slices/orderSlice'
import type { RootState } from '../store'

const { Title, Text } = Typography

const OrderDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const dispatch = useDispatch()
  const { currentOrder, isLoading } = useSelector((state: RootState) => state.order)

  useEffect(() => {
    if (id) {
      dispatch(fetchOrderDetail(id) as any)
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

  if (!currentOrder) {
    return (
      <div className="order-detail-page">
        <div className="container">
          <div style={{ textAlign: 'center', padding: '50px' }}>
            <Text type="secondary">订单不存在</Text>
            <br />
            <Button type="primary" onClick={() => navigate('/orders')}>
              返回订单列表
            </Button>
          </div>
        </div>
      </div>
    )
  }

  const getStatusText = (status: string) => {
    const statusMap: Record<string, string> = {
      PENDING: '待支付',
      PAID: '已支付',
      SHIPPED: '已发货',
      DELIVERED: '已完成',
      CANCELLED: '已取消',
      REFUNDING: '退款中',
      REFUNDED: '已退款'
    }
    return statusMap[status] || status
  }

  const getStatusColor = (status: string) => {
    const colorMap: Record<string, string> = {
      PENDING: 'orange',
      PAID: 'blue',
      SHIPPED: 'purple',
      DELIVERED: 'green',
      CANCELLED: 'red',
      REFUNDING: 'orange',
      REFUNDED: 'green'
    }
    return colorMap[status] || 'default'
  }

  return (
    <div className="order-detail-page">
      <div className="container">
        {/* 返回按钮 */}
        <Button
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate('/orders')}
          style={{ marginBottom: 24 }}
        >
          返回订单列表
        </Button>

        <Row gutter={[24, 24]}>
          {/* 订单信息 */}
          <Col xs={24} lg={16}>
            <Card>
              <div className="order-header">
                <Title level={3}>订单详情</Title>
                <Tag color={getStatusColor(currentOrder.status)}>
                  {getStatusText(currentOrder.status)}
                </Tag>
              </div>

              <Divider />

              {/* 订单基本信息 */}
              <Descriptions title="订单信息" column={2} bordered>
                <Descriptions.Item label="订单号">
                  {currentOrder.orderNo}
                </Descriptions.Item>
                <Descriptions.Item label="下单时间">
                  {new Date(currentOrder.createdAt).toLocaleString()}
                </Descriptions.Item>
                <Descriptions.Item label="订单金额">
                  ¥{currentOrder.totalAmount.toFixed(2)}
                </Descriptions.Item>
                <Descriptions.Item label="优惠金额">
                  ¥{currentOrder.discountAmount.toFixed(2)}
                </Descriptions.Item>
                <Descriptions.Item label="实付金额">
                  ¥{currentOrder.paymentAmount.toFixed(2)}
                </Descriptions.Item>
                <Descriptions.Item label="支付方式">
                  {currentOrder.paymentMethod || '未支付'}
                </Descriptions.Item>
              </Descriptions>

              {/* 收货地址 */}
              {currentOrder.shippingAddress && (
                <>
                  <Divider />
                  <Descriptions title="收货地址" column={1} bordered>
                    <Descriptions.Item label="收货人">
                      {currentOrder.shippingAddress.name}
                    </Descriptions.Item>
                    <Descriptions.Item label="联系电话">
                      {currentOrder.shippingAddress.phone}
                    </Descriptions.Item>
                    <Descriptions.Item label="收货地址">
                      {currentOrder.shippingAddress.province}
                      {currentOrder.shippingAddress.city}
                      {currentOrder.shippingAddress.district}
                      {currentOrder.shippingAddress.address}
                      {currentOrder.shippingAddress.postalCode && ` (${currentOrder.shippingAddress.postalCode})`}
                    </Descriptions.Item>
                  </Descriptions>
                </>
              )}

              {/* 订单备注 */}
              {currentOrder.remark && (
                <>
                  <Divider />
                  <div>
                    <Text strong>订单备注：</Text>
                    <Text>{currentOrder.remark}</Text>
                  </div>
                </>
              )}
            </Card>
          </Col>

          {/* 订单商品 */}
          <Col xs={24} lg={8}>
            <Card>
              <Title level={4}>商品信息</Title>
              <div className="order-items">
                {currentOrder.orderItems.map((item, index) => (
                  <div key={index} className="order-item">
                    <div className="item-info">
                      <Text strong>{item.couponName}</Text>
                      <Text type="secondary" style={{ fontSize: '12px' }}>
                        {item.couponType}
                      </Text>
                    </div>
                    <div className="item-price">
                      <Text>¥{item.finalPrice}</Text>
                      <Text type="secondary" style={{ fontSize: '12px' }}>
                        ×{item.quantity}
                      </Text>
                    </div>
                  </div>
                ))}
              </div>

              <Divider />

              <div className="order-summary">
                <div className="summary-row">
                  <Text>商品总价：</Text>
                  <Text>¥{currentOrder.totalAmount.toFixed(2)}</Text>
                </div>
                <div className="summary-row">
                  <Text>优惠金额：</Text>
                  <Text type="success">-¥{currentOrder.discountAmount.toFixed(2)}</Text>
                </div>
                <Divider style={{ margin: '12px 0' }} />
                <div className="summary-row">
                  <Text strong>实付金额：</Text>
                  <Text strong style={{ color: '#1890ff', fontSize: '16px' }}>
                    ¥{currentOrder.paymentAmount.toFixed(2)}
                  </Text>
                </div>
              </div>
            </Card>
          </Col>
        </Row>
      </div>
    </div>
  )
}

export default OrderDetail
