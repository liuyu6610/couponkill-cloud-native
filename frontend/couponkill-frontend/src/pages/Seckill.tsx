import React, { useState } from 'react'
import { Row, Col, Typography, Empty, Spin, Card, App, Button, Result, Space } from 'antd'
import { ThunderboltOutlined, ReloadOutlined, UnorderedListOutlined } from '@ant-design/icons'
import { useSelector } from 'react-redux'
import { useNavigate } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import type { RootState } from '../store'
import CouponCard from '../components/CouponCard'
import SeckillCountdown from '../components/SeckillCountdown'
import { useSeckillCoupons } from '../hooks/useCoupons'
import { useSeckill } from '../hooks/useSeckill'
import { useActiveReservationMap, useCreateReservation } from '../hooks/useReservations'
import { useSubmitGuard } from '../hooks/useSubmitGuard'
import { getErrorMessage } from '../lib/errorMessage'
import { connectorService } from '../services/connectorService'
import { queryKeys, staleTimes } from '../lib/queryClient'
import type { Coupon } from '../types/api'

const { Title, Text } = Typography

const Seckill: React.FC = () => {
  const navigate = useNavigate()
  const { message, modal } = App.useApp()
  const { isAuthenticated, user } = useSelector((state: RootState) => state.auth)

  const {
    data: seckillCoupons = [],
    isLoading,
    isError,
    error,
    refetch,
    isFetching,
  } = useSeckillCoupons()
  const seckill = useSeckill()
  const reserve = useCreateReservation()
  const { activeByCouponId } = useActiveReservationMap(isAuthenticated)
  const canSubmit = useSubmitGuard(1000)
  const [seckillingId, setSeckillingId] = useState<string | null>(null)
  const [reservingId, setReservingId] = useState<string | null>(null)

  const bindingQueries = useQueries({
    queries: seckillCoupons.map((c) => ({
      queryKey: queryKeys.connector.bindingByCoupon(c.id),
      queryFn: () => connectorService.getBindingByCoupon(c.id),
      staleTime: staleTimes.connectorBindings,
      enabled: !!c.id,
    })),
  })

  const bindingLabelOf = (index: number): string => {
    const b = bindingQueries[index]?.data
    if (!b) return '未绑定外部商品'
    return `关联：${b.platform} · SKU ${b.externalSkuId}`
  }

  const requireLogin = () => {
    message.info('请先登录后再操作')
    navigate('/login', { state: { from: { pathname: '/seckill' } } })
  }

  const handleSeckill = (coupon: Coupon) => {
    if (!isAuthenticated || !user) {
      requireLogin()
      return
    }
    if (!canSubmit(seckill.isPending)) return

    setSeckillingId(coupon.id)
    seckill.mutate(
      { couponId: coupon.id },
      {
        onSuccess: () => {
          modal.success({
            title: '秒杀成功',
            content: '订单已受理。可查看同品参考价，或前往「我的订单」。',
            okText: '查看同品比价',
            cancelText: '我的订单',
            okCancel: true,
            onOk: () => navigate(`/coupons/${coupon.id}#price-compare`),
            onCancel: () => navigate('/orders'),
          })
        },
        onError: (err) => message.error(getErrorMessage(err, '秒杀失败，请稍后重试')),
        onSettled: () => setSeckillingId(null),
      }
    )
  }

  const handleReserve = (coupon: Coupon) => {
    if (!isAuthenticated || !user) {
      requireLogin()
      return
    }
    if (!canSubmit(reserve.isPending)) return

    setReservingId(coupon.id)
    reserve.mutate(coupon.id, {
      onSuccess: () => {
        message.success('预约成功！开售时系统将代发本站秒杀入队')
        navigate('/reservations')
      },
      onError: (err) => message.error(getErrorMessage(err, '预约失败')),
      onSettled: () => setReservingId(null),
    })
  }

  // 取首张有时间窗的券做页头提示
  const headerCoupon = seckillCoupons.find((c) => c.seckillStartAt && c.seckillEndAt)

  return (
    <div className="seckill-page">
      <div className="container">
        <div className="page-header">
          <Title level={2}>
            <ThunderboltOutlined /> 秒杀专区
          </Title>
          <Text type="secondary">
            预约帮抢 = 到点代发本站秒杀入队 · 不是跨平台代下单
          </Text>
        </div>

        <Card style={{ margin: '24px 0' }}>
          <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
            <SeckillCountdown
              title="活动说明"
              startTime={headerCoupon?.seckillStartAt}
              endTime={headerCoupon?.seckillEndAt}
            />
            {isAuthenticated && (
              <Button icon={<UnorderedListOutlined />} onClick={() => navigate('/reservations')}>
                我的预约
              </Button>
            )}
          </Space>
          <Text type="secondary" style={{ display: 'block', marginTop: 12 }}>
            开售前点「预约帮抢」；开售后点「立即秒杀」。弱网请勿连点，受理中请到订单/预约页确认。
          </Text>
        </Card>

        <div className="seckill-products">
          {isLoading ? (
            <div style={{ textAlign: 'center', padding: 50 }}>
              <Spin size="large" />
            </div>
          ) : isError ? (
            <Result
              status="error"
              title="秒杀列表加载失败"
              subTitle={getErrorMessage(error, '请检查网络后重试')}
              extra={
                <Button
                  type="primary"
                  icon={<ReloadOutlined />}
                  loading={isFetching}
                  onClick={() => void refetch()}
                >
                  重新加载
                </Button>
              }
            />
          ) : seckillCoupons.length > 0 ? (
            <Row gutter={[16, 16]}>
              {seckillCoupons.map((coupon, idx) => (
                <Col xs={24} sm={12} lg={8} key={coupon.id}>
                  <CouponCard
                    coupon={coupon}
                    showActions
                    seckillLoading={seckillingId === coupon.id}
                    reserveLoading={reservingId === coupon.id}
                    alreadyReserved={activeByCouponId.has(String(coupon.id))}
                    bindingLabel={bindingLabelOf(idx)}
                    onSeckill={handleSeckill}
                    onReserve={handleReserve}
                    onViewReservations={() => navigate('/reservations')}
                  />
                </Col>
              ))}
            </Row>
          ) : (
            <Empty
              description="暂无秒杀活动，稍后再来看看"
              image={Empty.PRESENTED_IMAGE_SIMPLE}
            >
              <Button icon={<ReloadOutlined />} loading={isFetching} onClick={() => void refetch()}>
                刷新
              </Button>
            </Empty>
          )}
        </div>
      </div>
    </div>
  )
}

export default Seckill
