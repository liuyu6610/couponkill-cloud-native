import React from 'react'
import { Table, Tag, Button, Typography, Space, App, Empty, Spin, Popconfirm, Result } from 'antd'
import { useNavigate } from 'react-router-dom'
import { useSelector } from 'react-redux'
import type { ColumnsType } from 'antd/es/table'
import type { RootState } from '../store'
import { useMyReservations, useCancelReservation } from '../hooks/useReservations'
import {
  formatBackendDateTime,
  reservationStatusColor,
  reservationStatusText,
  ReservationStatus,
  type SeckillReservation,
} from '../types/api'
import { getErrorMessage } from '../lib/errorMessage'
import { notifyError } from '../lib/notifyError'
import { SECKILL_REASON_MESSAGES } from '../lib/seckill'

const { Title, Text, Paragraph } = Typography

const MyReservations: React.FC = () => {
  const navigate = useNavigate()
  const { message } = App.useApp()
  const { isAuthenticated } = useSelector((state: RootState) => state.auth)
  const { data = [], isLoading, isError, error, isFetching, refetch } =
    useMyReservations(isAuthenticated)
  const cancel = useCancelReservation()

  const columns: ColumnsType<SeckillReservation> = [
    {
      title: '预约ID',
      dataIndex: 'id',
      width: 100,
    },
    {
      title: '券ID',
      dataIndex: 'couponId',
      render: (id: string) => (
        <Button type="link" onClick={() => navigate(`/coupons/${id}`)}>
          {id}
        </Button>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      render: (s: string) => <Tag color={reservationStatusColor(s)}>{reservationStatusText(s)}</Tag>,
    },
    {
      title: '计划开抢',
      dataIndex: 'triggerAt',
      render: (v?: string | null) => formatBackendDateTime(v),
    },
    {
      title: 'requestId',
      dataIndex: 'requestId',
      ellipsis: true,
      render: (v?: string | null) => v || '-',
    },
    {
      title: '失败原因',
      dataIndex: 'failReason',
      ellipsis: true,
      // 失败原因可能是秒杀词表里的原因码（OUT_OF_STOCK…），已知的映射成中文，其余原样展示
      render: (v?: string | null) => (v ? (SECKILL_REASON_MESSAGES[v] ?? v) : '-'),
    },
    {
      title: '操作',
      key: 'actions',
      render: (_, row) => (
        <Space>
          {row.status === ReservationStatus.PENDING && (
            <Popconfirm
              title="确认取消该预约？"
              onConfirm={() =>
                cancel.mutate(row.id, {
                  onSuccess: () => message.success('已取消预约'),
                  onError: (e) => notifyError(message, e, '取消失败'),
                })
              }
            >
              <Button
                size="small"
                danger
                loading={cancel.isPending && cancel.variables === row.id}
                disabled={cancel.isPending && cancel.variables !== row.id}
              >
                取消
              </Button>
            </Popconfirm>
          )}
          {row.status === ReservationStatus.SUCCESS && row.orderId && (
            <Button size="small" type="link" onClick={() => navigate('/orders')}>
              查看订单
            </Button>
          )}
        </Space>
      ),
    },
  ]

  if (!isAuthenticated) {
    return (
      <div className="container" style={{ padding: 48, textAlign: 'center' }}>
        <Empty description="请先登录查看预约" />
        <Button type="primary" onClick={() => navigate('/login')} style={{ marginTop: 16 }}>
          去登录
        </Button>
      </div>
    )
  }

  return (
    <div className="container" style={{ padding: '24px 0' }}>
      <Title level={3}>我的预约帮抢</Title>
      <Paragraph type="secondary">
        状态机：待开抢(PENDING) → 抢购中(FIRING) → 已入队(QUEUED) → 已抢到(SUCCESS) / 失败(FAILED)。
        帮抢的是本站秒杀券，不是电商平台代下单。
      </Paragraph>
      <Space style={{ marginBottom: 16 }}>
        <Button onClick={() => void refetch()} loading={isFetching}>
          刷新
        </Button>
        <Button type="primary" onClick={() => navigate('/seckill')}>
          去秒杀专区
        </Button>
      </Space>
      {isLoading ? (
        <div style={{ textAlign: 'center', padding: 48 }}>
          <Spin size="large" />
        </div>
      ) : isError && !data.length ? (
        // 之前加载失败会落到空表「暂无预约」，用户误以为预约丢了
        <Result
          status="error"
          title="预约列表加载失败"
          subTitle={getErrorMessage(error, '请检查网络后重试')}
          extra={
            <Button type="primary" loading={isFetching} onClick={() => void refetch()}>
              重新加载
            </Button>
          }
        />
      ) : (
        <Table
          rowKey="id"
          columns={columns}
          dataSource={data}
          pagination={{ pageSize: 10 }}
          locale={{ emptyText: <Empty description="暂无预约" /> }}
        />
      )}
      {!data.length && !isLoading && !isError && (
        <Text type="secondary">开售前在秒杀专区点「预约帮抢」即可。</Text>
      )}
    </div>
  )
}

export default MyReservations
