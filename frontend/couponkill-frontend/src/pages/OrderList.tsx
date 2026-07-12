import React from 'react'
import { Table, Typography, Button, Tag, Space, Card, Popconfirm, App } from 'antd'
import { EyeOutlined } from '@ant-design/icons'
import { useSelector } from 'react-redux'
import { useNavigate } from 'react-router-dom'
import type { RootState } from '../store'
import { useUserOrders, useCancelOrder } from '../hooks/useOrders'
import { OrderStatus, orderStatusColor, orderStatusText } from '../types/api'
import type { Order } from '../types/api'
import type { TableColumnsType } from 'antd'

const { Title, Text } = Typography

const OrderList: React.FC = () => {
  const navigate = useNavigate()
  const { message } = App.useApp()
  const { user } = useSelector((state: RootState) => state.auth)

  const { data: orders = [], isLoading } = useUserOrders(user?.id)
  const cancelOrder = useCancelOrder()

  const handleCancel = (order: Order) => {
    if (!user) return
    cancelOrder.mutate(
      { orderId: order.id },
      {
        onSuccess: () => message.success('订单已取消'),
        onError: (err) => message.error((err as Error).message || '取消失败'),
      }
    )
  }

  const columns: TableColumnsType<Order> = [
    { title: '订单号', dataIndex: 'id', key: 'id', width: 220, ellipsis: true },
    { title: '优惠券ID', dataIndex: 'couponId', key: 'couponId', width: 160, ellipsis: true },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: number) => <Tag color={orderStatusColor(status)}>{orderStatusText(status)}</Tag>,
    },
    {
      title: '下单时间',
      dataIndex: 'createTime',
      key: 'createTime',
      width: 180,
      render: (t?: string) => (t ? new Date(t).toLocaleString() : '-'),
    },
    {
      title: '操作',
      key: 'actions',
      width: 180,
      render: (_: unknown, record: Order) => (
        <Space>
          <Button type="link" icon={<EyeOutlined />} onClick={() => navigate(`/orders/${record.id}`)}>
            详情
          </Button>
          {record.status === OrderStatus.CREATED && (
            <Popconfirm title="确认取消该订单？" onConfirm={() => handleCancel(record)}>
              <Button type="link" danger loading={cancelOrder.isPending}>
                取消
              </Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div className="order-list-page">
      <div className="container">
        <div className="page-header">
          <Title level={2}>我的订单</Title>
          <Text type="secondary">查看和管理您的订单信息</Text>
        </div>

        <Card style={{ marginTop: 24 }}>
          <Table
            columns={columns}
            dataSource={orders}
            loading={isLoading}
            rowKey="id"
            scroll={{ x: 'max-content' }}
            pagination={{ pageSize: 10, showSizeChanger: true, showQuickJumper: true }}
            locale={{ emptyText: '暂无订单数据' }}
          />
        </Card>
      </div>
    </div>
  )
}

export default OrderList
