import React, { useState } from 'react'
import { Table, Typography, Button, Tag, Space, Card, Popconfirm, App, Result } from 'antd'
import { EyeOutlined, ReloadOutlined } from '@ant-design/icons'
import { useSelector } from 'react-redux'
import { useNavigate } from 'react-router-dom'
import type { RootState } from '../store'
import { useUserOrders, useCancelOrder, ORDERS_DEFAULT_PAGE_SIZE } from '../hooks/useOrders'
import { useSubmitGuard } from '../hooks/useSubmitGuard'
import { getErrorMessage } from '../lib/errorMessage'
import { notifyError } from '../lib/notifyError'
import {
  OrderStatus,
  formatBackendDateTime,
  formatMoney,
  orderSourceText,
  orderStatusColor,
  orderStatusText,
} from '../types/api'
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
    data: ordersPage,
    isLoading,
    isError,
    error,
    refetch,
    isFetching,
    isPlaceholderData,
  } = useUserOrders(user?.id, pageNum, pageSize)
  const orders = ordersPage?.items ?? []
  const cancelOrder = useCancelOrder()
  const canSubmit = useSubmitGuard(800)

  const handleCancel = (order: Order) => {
    if (!user || !canSubmit(cancelOrder.isPending)) return
    cancelOrder.mutate(
      { orderId: order.id },
      {
        onSuccess: () => message.success('订单已取消'),
        onError: (err) => notifyError(message, err, '取消失败'),
      }
    )
  }

  const columns: TableColumnsType<Order> = [
    { title: '订单号', dataIndex: 'id', key: 'id', width: 220, ellipsis: true },
    {
      title: '优惠券',
      key: 'coupon',
      width: 200,
      ellipsis: true,
      render: (_: unknown, record: Order) => record.couponName || record.couponId,
    },
    {
      title: '面值',
      dataIndex: 'faceValue',
      key: 'faceValue',
      width: 90,
      render: (v?: Order['faceValue']) => (v == null ? '-' : `¥${formatMoney(v)}`),
    },
    {
      title: '来源',
      dataIndex: 'source',
      key: 'source',
      width: 100,
      render: (s?: string) => orderSourceText(s),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: number) => <Tag color={orderStatusColor(status)}>{orderStatusText(status)}</Tag>,
    },
    {
      title: '下单时间',
      dataIndex: 'createdAt',
      key: 'createdAt',
      width: 180,
      render: (t?: string) => formatBackendDateTime(t),
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
              <Button
                type="link"
                danger
                loading={cancelOrder.isPending && cancelOrder.variables?.orderId === record.id}
              >
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
    // 总数以后端分页信封的 total 为准；翻页加载占位期间沿用上一份已知总数，避免分页器抖动
    total: ordersPage?.total ?? 0,
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
