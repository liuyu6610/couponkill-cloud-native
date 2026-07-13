import React, { useState } from 'react'
import { Table, Typography, Button, Tag, Space, Card, Popconfirm, App, Result } from 'antd'
import { EyeOutlined, ReloadOutlined } from '@ant-design/icons'
import { useSelector } from 'react-redux'
import { useNavigate } from 'react-router-dom'
import type { RootState } from '../store'
import { useUserOrders, useCancelOrder, ORDERS_DEFAULT_PAGE_SIZE } from '../hooks/useOrders'
import { useSubmitGuard } from '../hooks/useSubmitGuard'
import { getErrorMessage } from '../lib/errorMessage'
import { OrderStatus, orderStatusColor, orderStatusText } from '../types/api'
import type { Order } from '../types/api'
import type { TableColumnsType, TablePaginationConfig } from 'antd'

const { Title, Text } = Typography

const OrderList: React.FC = () => {
  const navigate = useNavigate()
  const { message } = App.useApp()
  const { user } = useSelector((state: RootState) => state.auth)

  const [pageNum, setPageNum] = useState(1)
  const [pageSize, setPageSize] = useState(ORDERS_DEFAULT_PAGE_SIZE)

  const {
    data: orders = [],
    isLoading,
    isError,
    error,
    refetch,
    isFetching,
    isPlaceholderData,
  } = useUserOrders(user?.id, pageNum, pageSize)
  const cancelOrder = useCancelOrder()
  const canSubmit = useSubmitGuard(800)

  const handleCancel = (order: Order) => {
    if (!user || !canSubmit(cancelOrder.isPending)) return
    cancelOrder.mutate(
      { orderId: order.id },
      {
        onSuccess: () => message.success('订单已取消'),
        onError: (err) => message.error(getErrorMessage(err, '取消失败')),
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

  const pagination: TablePaginationConfig = {
    current: pageNum,
    pageSize,
    // API 未返回 total：用「本页满页则可能还有下一页」近似，避免假总数
    total: isPlaceholderData
      ? pageNum * pageSize + 1
      : orders.length < pageSize
        ? (pageNum - 1) * pageSize + orders.length
        : pageNum * pageSize + 1,
    showSizeChanger: true,
    showQuickJumper: true,
    pageSizeOptions: [10, 20, 50],
    onChange: (page, size) => {
      setPageNum(page)
      setPageSize(size)
    },
  }

  return (
    <div className="order-list-page">
      <div className="container">
        <div className="page-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <div>
            <Title level={2}>我的订单</Title>
            <Text type="secondary">查看和管理您的订单信息</Text>
          </div>
          <Button icon={<ReloadOutlined />} loading={isFetching} onClick={() => void refetch()}>
            刷新
          </Button>
        </div>

        <Card style={{ marginTop: 24 }}>
          {isError && !orders.length ? (
            <Result
              status="error"
              title="订单加载失败"
              subTitle={getErrorMessage(error, '请检查网络后重试')}
              extra={
                <Button type="primary" icon={<ReloadOutlined />} loading={isFetching} onClick={() => void refetch()}>
                  重新加载
                </Button>
              }
            />
          ) : (
            <Table
              columns={columns}
              dataSource={orders}
              loading={isLoading || (isFetching && isPlaceholderData)}
              rowKey="id"
              scroll={{ x: 'max-content' }}
              pagination={pagination}
              locale={{ emptyText: '暂无订单数据' }}
            />
          )}
        </Card>
      </div>
    </div>
  )
}

export default OrderList
