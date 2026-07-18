import React from 'react'
import { Row, Col, Card, Typography, Button, Descriptions, Tag, Divider, Spin, App, Result, Space, Table } from 'antd'
import { ArrowLeftOutlined, ThunderboltOutlined, ReloadOutlined, ClockCircleOutlined, CheckOutlined } from '@ant-design/icons'
import { useParams, useNavigate } from 'react-router-dom'
import { useSelector } from 'react-redux'
import { useQuery } from '@tanstack/react-query'
import type { RootState } from '../store'
import { useCouponDetail } from '../hooks/useCoupons'
import { useSeckill } from '../hooks/useSeckill'
import { useCreateOrder } from '../hooks/useOrders'
import { useActiveReservationMap, useCreateReservation } from '../hooks/useReservations'
import { useSubmitGuard } from '../hooks/useSubmitGuard'
import { getErrorMessage } from '../lib/errorMessage'
import { connectorService } from '../services/connectorService'
import { queryKeys, staleTimes } from '../lib/queryClient'
import SeckillCountdown from '../components/SeckillCountdown'
import {
  couponStockOf,
  couponTotalStockOf,
  couponTypeText,
  isSeckillCoupon,
  seckillWindowPhase,
} from '../types/api'

const { Title, Text, Paragraph } = Typography

const CouponDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { message } = App.useApp()
  const { isAuthenticated, user } = useSelector((state: RootState) => state.auth)

  const { data: coupon, isLoading, isError, error, refetch, isFetching, isPlaceholderData } =
    useCouponDetail(id)
  const seckill = useSeckill()
  const createOrder = useCreateOrder()
  const reserve = useCreateReservation()
  const { activeByCouponId } = useActiveReservationMap(isAuthenticated)
  const canSubmit = useSubmitGuard(1000)
  const acting = seckill.isPending || createOrder.isPending || reserve.isPending
  const alreadyReserved = !!id && activeByCouponId.has(String(id))

  const { data: binding } = useQuery({
    queryKey: queryKeys.connector.bindingByCoupon(id ?? ''),
    queryFn: () => connectorService.getBindingByCoupon(id as string),
    enabled: !!id,
    staleTime: staleTimes.connectorBindings,
  })

  const { data: priceCompare, isFetching: priceCompareFetching } = useQuery({
    queryKey: queryKeys.connector.priceCompare(id ?? ''),
    queryFn: () => connectorService.priceCompare(id as string),
    enabled: !!id && !!coupon && isSeckillCoupon(coupon),
    staleTime: 30_000,
  })

  if (isLoading && !coupon) {
    return (
      <div className="loading-container" style={{ textAlign: 'center', padding: 50 }}>
        <Spin size="large" />
      </div>
    )
  }

  if (isError && !coupon) {
    return (
      <Result
        status="error"
        title="优惠券详情加载失败"
        subTitle={getErrorMessage(error, '请检查网络后重试')}
        extra={[
          <Button key="back" onClick={() => navigate('/coupons')}>
            返回列表
          </Button>,
          <Button
            key="retry"
            type="primary"
            icon={<ReloadOutlined />}
            loading={isFetching}
            onClick={() => void refetch()}
          >
            重试
          </Button>,
        ]}
      />
    )
  }

  if (!coupon) {
    return (
      <div className="coupon-detail-page">
        <div className="container" style={{ textAlign: 'center', padding: 50 }}>
          <Text type="secondary">优惠券不存在或已下架</Text>
          <br />
          <Button type="primary" onClick={() => navigate('/coupons')} style={{ marginTop: 16 }}>
            返回优惠券列表
          </Button>
        </div>
      </div>
    )
  }

  const seckillType = isSeckillCoupon(coupon)
  const stock = couponStockOf(coupon)
  const totalStock = couponTotalStockOf(coupon)
  const soldOut = stock <= 0
  const invalid = coupon.status !== 1
  const phase = seckillType ? seckillWindowPhase(coupon) : 'no_window'

  const handleClaim = () => {
    if (!isAuthenticated || !user) {
      message.info('请先登录')
      navigate('/login', { state: { from: { pathname: `/coupons/${coupon.id}` } } })
      return
    }
    if (!canSubmit(acting)) return
    const args = { couponId: coupon.id }
    const opts = {
      onSuccess: () => message.success('领取成功！可在“我的订单”中查看'),
      onError: (err: unknown) => message.error(getErrorMessage(err, '领取失败，请稍后重试')),
    }
    if (seckillType) {
      seckill.mutate(args, opts)
    } else {
      createOrder.mutate(args, opts)
    }
  }

  const handleReserve = () => {
    if (!isAuthenticated || !user) {
      message.info('请先登录')
      navigate('/login', { state: { from: { pathname: `/coupons/${coupon.id}` } } })
      return
    }
    if (!canSubmit(acting)) return
    reserve.mutate(coupon.id, {
      onSuccess: () => {
        message.success('预约成功！开售时系统将代发本站秒杀入队')
        navigate('/reservations')
      },
      onError: (err) => message.error(getErrorMessage(err, '预约失败')),
    })
  }

  return (
    <div className="coupon-detail-page">
      <div className="container">
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/coupons')} style={{ marginBottom: 24 }}>
          返回列表
        </Button>

        {isPlaceholderData && (
          <Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>
            正在同步最新库存…
          </Text>
        )}

        <Row gutter={[24, 24]}>
          <Col xs={24} lg={16}>
            <Card>
              <div className="coupon-detail-header">
                <Title level={2}>{coupon.name}</Title>
                <Tag color={seckillType ? 'red' : 'blue'}>{couponTypeText(coupon.type)}</Tag>
                <Tag color={invalid ? 'red' : 'green'}>{invalid ? '已失效' : '有效'}</Tag>
              </div>

              <Divider />

              {coupon.description && <Paragraph>{coupon.description}</Paragraph>}

              {seckillType && (
                <div style={{ marginBottom: 16 }}>
                  <SeckillCountdown
                    title="活动时间窗"
                    startTime={coupon.seckillStartAt}
                    endTime={coupon.seckillEndAt}
                  />
                </div>
              )}

              <Descriptions column={2} bordered>
                <Descriptions.Item label="面值">¥{coupon.faceValue}</Descriptions.Item>
                <Descriptions.Item label="使用门槛">
                  {coupon.minSpend > 0 ? `满¥${coupon.minSpend}` : '无门槛'}
                </Descriptions.Item>
                <Descriptions.Item label="库存">
                  {stock}/{totalStock}
                </Descriptions.Item>
                <Descriptions.Item label="有效期">{coupon.validDays} 天</Descriptions.Item>
                <Descriptions.Item label="每人限领">{coupon.perUserLimit} 张</Descriptions.Item>
                <Descriptions.Item label="类型">{couponTypeText(coupon.type)}</Descriptions.Item>
                {seckillType && (
                  <Descriptions.Item label="外部绑定" span={2}>
                    {binding
                      ? `${binding.platform} · SKU ${binding.externalSkuId}`
                      : '未绑定外部商品'}
                  </Descriptions.Item>
                )}
              </Descriptions>

              {seckillType && (
                <>
                  <Divider />
                  <Title level={5}>同品参考价</Title>
                  <Text type="secondary" style={{ display: 'block', marginBottom: 12, fontSize: 12 }}>
                    仅供决策参考，不保证最低价；请查看可信度与抓取时间。不是跨平台代下单。
                  </Text>
                  <Table
                    size="small"
                    rowKey={(r) => `${r.platform}-${r.externalSkuId}`}
                    loading={priceCompareFetching}
                    pagination={false}
                    locale={{ emptyText: binding ? '暂无比价数据' : '未绑定外部商品，暂无比价' }}
                    dataSource={priceCompare?.items ?? []}
                    columns={[
                      { title: '平台', dataIndex: 'platform', width: 80 },
                      { title: 'SKU', dataIndex: 'externalSkuId', ellipsis: true },
                      {
                        title: '价格',
                        dataIndex: 'price',
                        width: 100,
                        render: (v: number | null | undefined, row) =>
                          v != null ? `${row.currency || 'CNY'} ${v}` : '-',
                      },
                      {
                        title: '可信度',
                        dataIndex: 'confidence',
                        width: 90,
                        render: (c: string | null | undefined) => {
                          const color =
                            c === 'HIGH' ? 'green' : c === 'MEDIUM' ? 'orange' : 'default'
                          return <Tag color={color}>{c || '-'}</Tag>
                        },
                      },
                      { title: '来源', dataIndex: 'source', width: 80 },
                      { title: '抓取时间', dataIndex: 'fetchedAt', width: 160 },
                    ]}
                  />
                </>
              )}
            </Card>
          </Col>

          <Col xs={24} lg={8}>
            <Card>
              <div className="price-info">
                <Text type="secondary">优惠面值</Text>
                <div>
                  <Text strong style={{ fontSize: 28, color: '#fa541c' }}>
                    ¥{coupon.faceValue}
                  </Text>
                </div>
              </div>

              <div className="stock-info" style={{ marginTop: 16 }}>
                <Text type="secondary">剩余库存</Text>
                <div>
                  <Text strong style={{ color: soldOut ? '#ff4d4f' : '#52c41a' }}>
                    {stock} / {totalStock}
                  </Text>
                </div>
              </div>

              <Divider />

              <Space direction="vertical" style={{ width: '100%' }}>
                {seckillType && phase === 'no_window' ? (
                  <Button size="large" block disabled>
                    未配置时间窗
                  </Button>
                ) : seckillType && phase === 'upcoming' && alreadyReserved ? (
                  <Button
                    type="default"
                    size="large"
                    block
                    icon={<CheckOutlined />}
                    onClick={() => navigate('/reservations')}
                  >
                    已预约 · 查看
                  </Button>
                ) : seckillType && phase === 'upcoming' ? (
                  <Button
                    type="primary"
                    size="large"
                    block
                    icon={<ClockCircleOutlined />}
                    loading={reserve.isPending}
                    disabled={invalid}
                    onClick={handleReserve}
                  >
                    预约帮抢
                  </Button>
                ) : (
                  <Button
                    type="primary"
                    danger={seckillType}
                    size="large"
                    block
                    icon={seckillType ? <ThunderboltOutlined /> : undefined}
                    loading={acting}
                    disabled={soldOut || invalid || phase === 'ended'}
                    onClick={handleClaim}
                  >
                    {phase === 'ended'
                      ? '已结束'
                      : soldOut
                        ? '已抢光'
                        : seckillType
                          ? '立即秒杀'
                          : '立即领取'}
                  </Button>
                )}
              </Space>

              <div className="tips" style={{ marginTop: 12 }}>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {invalid
                    ? '该优惠券已失效'
                    : soldOut
                      ? '该优惠券已售罄'
                      : seckillType && phase === 'no_window'
                        ? '请先由运营配置 seckillStartAt / seckillEndAt'
                        : seckillType && phase === 'upcoming' && alreadyReserved
                          ? '已预约，开售时系统代发本站秒杀入队'
                          : seckillType && phase === 'upcoming'
                            ? '开售时系统代发本站秒杀入队（不是跨平台代下单）'
                            : seckillType
                              ? '提交中请勿连点；弱网超时后请到订单页确认'
                              : '数量有限，先到先得'}
                </Text>
              </div>
            </Card>
          </Col>
        </Row>
      </div>
    </div>
  )
}

export default CouponDetail
