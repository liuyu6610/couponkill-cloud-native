import React, { useEffect } from 'react'
import { Table, Typography, Button, Tag, Space, Card, Row, Col } from 'antd'
import { EyeOutlined } from '@ant-design/icons'
import { useDispatch, useSelector } from 'react-redux'
import { useNavigate } from 'react-router-dom'
import { fetchOrders } from '../store/slices/orderSlice'
import type { RootState } from '../store'

const { Title, Text } = Typography

const OrderList: React.FC = () => {
  const dispatch = useDispatch()
  const navigate = useNavigate()
  const { orders, isLoading } = useSelector((state: RootState) => state.order)

  useEffect(() => {
    dispatch(fetchOrders({}) as any)
  }, [dispatch])

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

  const columns = [
    {
      title: '订单号',
      dataIndex: 'orderNo',
      key: 'orderNo',
      width: 200,
    },
    {
      title: '商品信息',
      key: 'items',
      render: (record: any) => (
        <div>
          {record.orderItems.map((item: any, index: number) => (
            <div key={index} style={{ marginBottom: '4px' }}>
              <Text>{item.couponName}</Text>
              <br />
              <Text type="secondary" style={{ fontSize: '12px' }}>
                数量: {item.quantity} | 价格: ¥{item.finalPrice}
              </Text>
            </div>
          ))}
        </div>
      ),
    },
    {
      title: '订单金额',
      dataIndex: 'paymentAmount',
      key: 'paymentAmount',
      render: (amount: number) => `¥${amount.toFixed(2)}`,
      width: 100,
    },
    {
      title: '订单状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Tag color={getStatusColor(status)}>
          {getStatusText(status)}
        </Tag>
      ),
      width: 100,
    },
    {
      title: '下单时间',
      dataIndex: 'createdAt',
      key: 'createdAt',
      render: (date: string) => new Date(date).toLocaleString(),
      width: 150,
    },
    {
      title: '操作',
      key: 'actions',
      render: (record: any) => (
        <Space>
          <Button
            type="link"
            icon={<EyeOutlined />}
            onClick={() => navigate(`/orders/${record.id}`)}
          >
            查看详情
          </Button>
        </Space>
      ),
      width: 100,
    },
  ]

  return (
    <div className="order-list-page">
      <div className="container">
        {/* 页面标题 */}
        <div className="page-header">
          <Title level={2}>我的订单</Title>
          <Text type="secondary">查看和管理您的订单信息</Text>
        </div>

        {/* 订单统计 */}
        <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
          <Col xs={24} sm={6}>
            <Card>
              <div style={{ textAlign: 'center' }}>
                <div style={{ fontSize: '24px', fontWeight: 'bold', color: '#1890ff' }}>
                  {orders.filter(order => order.status === 'PENDING').length}
                </div>
                <Text type="secondary">待支付</Text>
              </div>
            </Card>
          </Col>
          <Col xs={24} sm={6}>
            <Card>
              <div style={{ textAlign: 'center' }}>
                <div style={{ fontSize: '24px', fontWeight: 'bold', color: '#52c41a' }}>
                  {orders.filter(order => order.status === 'DELIVERED').length}
                </div>
                <Text type="secondary">已完成</Text>
              </div>
            </Card>
          </Col>
          <Col xs={24} sm={6}>
            <Card>
              <div style={{ textAlign: 'center' }}>
                <div style={{ fontSize: '24px', fontWeight: 'bold', color: '#faad14' }}>
                  {orders.filter(order => order.status === 'SHIPPED').length}
                </div>
                <Text type="secondary">待收货</Text>
              </div>
            </Card>
          </Col>
          <Col xs={24} sm={6}>
            <Card>
              <div style={{ textAlign: 'center' }}>
                <div style={{ fontSize: '24px', fontWeight: 'bold', color: '#ff4d4f' }}>
                  {orders.filter(order => ['CANCELLED', 'REFUNDED'].includes(order.status)).length}
                </div>
                <Text type="secondary">已取消/退款</Text>
              </div>
            </Card>
          </Col>
        </Row>

        {/* 订单表格 */}
        <Card>
          <Table
            columns={columns}
            dataSource={orders}
            loading={isLoading}
            rowKey="id"
            pagination={{
              total: orders.length,
              pageSize: 10,
              showSizeChanger: true,
              showQuickJumper: true,
              showTotal: (total, range) =>
                `第 ${range[0]}-${range[1]} 条，共 ${total} 条`,
            }}
            locale={{
              emptyText: '暂无订单数据'
            }}
          />
        </Card>
      </div>
    </div>
  )
}

export default OrderList
