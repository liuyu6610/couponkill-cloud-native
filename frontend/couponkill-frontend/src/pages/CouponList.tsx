import React, { useEffect, useMemo, useState } from 'react'
import { Row, Col, Typography, Input, Select, Empty, Spin, App, Button, Result } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { useSelector } from 'react-redux'
import { useNavigate, useSearchParams } from 'react-router-dom'
import type { RootState } from '../store'
import CouponCard from '../components/CouponCard'
import { useAvailableCoupons } from '../hooks/useCoupons'
import { useSeckill, seckillStageText } from '../hooks/useSeckill'
import { useActiveReservationMap, useCreateReservation } from '../hooks/useReservations'
import { useSubmitGuard } from '../hooks/useSubmitGuard'
import { getErrorMessage } from '../lib/errorMessage'
import { notifyError } from '../lib/notifyError'
import { CouponType } from '../types/api'
import type { Coupon } from '../types/api'

const { Title, Text } = Typography
const { Search } = Input

const CouponList: React.FC = () => {
  const navigate = useNavigate()
  const { message, modal } = App.useApp()
  const { isAuthenticated, user } = useSelector((state: RootState) => state.auth)

  const {
    data: coupons = [],
    isLoading,
    isError,
    error,
    refetch,
    isFetching,
  } = useAvailableCoupons()
  const seckill = useSeckill()
  const reserve = useCreateReservation()
  const { activeByCouponId } = useActiveReservationMap(isAuthenticated)
  const canSubmit = useSubmitGuard(1000)
  const [seckillingId, setSeckillingId] = useState<string | null>(null)
  const [reservingId, setReservingId] = useState<string | null>(null)

  // 关键字以 URL ?search= 为单一真源：顶部导航搜索跳过来能直接生效，刷新/分享也保留筛选
  const [searchParams, setSearchParams] = useSearchParams()
  const keyword = (searchParams.get('search') ?? '').trim()
  const [searchInput, setSearchInput] = useState(keyword)
  const [typeFilter, setTypeFilter] = useState<number | ''>('')

  // URL 变化（如在顶部再次搜索）时同步输入框
  useEffect(() => {
    setSearchInput(keyword)
  }, [keyword])

  /** 写回 URL；replace 避免每次搜索都堆一条历史记录 */
  const applyKeyword = (value: string) => {
    const next = new URLSearchParams(searchParams)
    const v = value.trim()
    if (v) next.set('search', v)
    else next.delete('search')
    setSearchParams(next, { replace: true })
  }

  const filtered = useMemo(() => {
    const kw = keyword.toLowerCase()
    return coupons.filter((c) => {
      const matchKeyword = kw ? (c.name ?? '').toLowerCase().includes(kw) : true
      const matchType = typeFilter === '' ? true : c.type === typeFilter
      return matchKeyword && matchType
    })
  }, [coupons, keyword, typeFilter])

  const requireLogin = (path = '/coupons') => {
    message.info('请先登录后再操作')
    navigate('/login', { state: { from: { pathname: path } } })
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
        onSuccess: ({ orderId }) => {
          modal.success({
            title: '秒杀成功',
            content: '订单已生成。可查看同品参考价，或前往「我的订单」。',
            okText: '查看同品比价',
            cancelText: '我的订单',
            okCancel: true,
            onOk: () => navigate(`/coupons/${coupon.id}#price-compare`),
            onCancel: () => navigate(orderId ? `/orders/${orderId}` : '/orders'),
          })
        },
        onError: (err) => notifyError(message, err, '秒杀失败，请稍后重试'),
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
      onError: (err) => notifyError(message, err, '预约失败'),
      onSettled: () => setReservingId(null),
    })
  }

  return (
    <div className="coupon-list-page">
      <div className="container">
        <div className="page-header">
          <Title level={2}>优惠券列表</Title>
          <Text type="secondary">发现更多优质优惠，总有一款适合你</Text>
        </div>

        <div className="filters-section" style={{ margin: '24px 0' }}>
          <Row gutter={[16, 16]} align="middle">
            <Col xs={24} sm={12}>
              <Search
                placeholder="搜索优惠券名称..."
                aria-label="搜索优惠券名称"
                allowClear
                size="large"
                value={searchInput}
                onSearch={applyKeyword}
                onChange={(e) => {
                  setSearchInput(e.target.value)
                  if (!e.target.value) applyKeyword('')
                }}
              />
            </Col>
            <Col xs={24} sm={12}>
              <Select
                placeholder="选择类型"
                aria-label="按类型筛选"
                value={typeFilter === '' ? undefined : typeFilter}
                onChange={(v) => setTypeFilter(v ?? '')}
                style={{ width: '100%' }}
                size="large"
                allowClear
                options={[
                  { value: CouponType.NORMAL, label: '常驻优惠券' },
                  { value: CouponType.SECKILL, label: '秒杀优惠券' },
                ]}
              />
            </Col>
          </Row>
        </div>

        <div className="coupon-list-section">
          {isLoading ? (
            <div className="loading-container" style={{ textAlign: 'center', padding: 50 }}>
              <Spin size="large" />
            </div>
          ) : isError ? (
            <Result
              status="error"
              title="优惠券列表加载失败"
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
          ) : filtered.length > 0 ? (
            <Row gutter={[16, 16]}>
              {filtered.map((coupon) => (
                <Col xs={24} sm={12} lg={8} xl={6} key={coupon.id}>
                  <CouponCard
                    coupon={coupon}
                    showActions
                    seckillLoading={seckillingId === coupon.id}
                    seckillLoadingText={seckillStageText(seckill.stage)}
                    reserveLoading={reservingId === coupon.id}
                    alreadyReserved={activeByCouponId.has(String(coupon.id))}
                    onSeckill={handleSeckill}
                    onReserve={handleReserve}
                    onViewReservations={() => navigate('/reservations')}
                  />
                </Col>
              ))}
            </Row>
          ) : (
            <Empty
              description={keyword || typeFilter !== '' ? '没有符合筛选条件的优惠券' : '暂无优惠券数据'}
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

export default CouponList
