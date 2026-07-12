import React, { useMemo, useState } from 'react'
import { Row, Col, Typography, Input, Select, Empty, Spin, App } from 'antd'
import { useSelector } from 'react-redux'
import { useNavigate } from 'react-router-dom'
import type { RootState } from '../store'
import CouponCard from '../components/CouponCard'
import { useAvailableCoupons } from '../hooks/useCoupons'
import { useSeckill } from '../hooks/useSeckill'
import { CouponType } from '../types/api'
import type { Coupon } from '../types/api'

const { Title, Text } = Typography
const { Search } = Input

const CouponList: React.FC = () => {
  const navigate = useNavigate()
  const { message } = App.useApp()
  const { isAuthenticated, user } = useSelector((state: RootState) => state.auth)

  const { data: coupons = [], isLoading } = useAvailableCoupons()
  const seckill = useSeckill()
  const [seckillingId, setSeckillingId] = useState<string | null>(null)

  const [keyword, setKeyword] = useState('')
  const [typeFilter, setTypeFilter] = useState<number | ''>('')

  const filtered = useMemo(() => {
    return coupons.filter((c) => {
      const matchKeyword = keyword ? c.name.includes(keyword) : true
      const matchType = typeFilter === '' ? true : c.type === typeFilter
      return matchKeyword && matchType
    })
  }, [coupons, keyword, typeFilter])

  const handleSeckill = (coupon: Coupon) => {
    if (!isAuthenticated || !user) {
      message.info('请先登录后再参与秒杀')
      navigate('/login', { state: { from: { pathname: '/coupons' } } })
      return
    }
    if (seckill.isPending) return // 防抖：进行中禁止重复提交
    setSeckillingId(coupon.id)
    seckill.mutate(
      { couponId: coupon.id },
      {
        onSuccess: () => message.success('秒杀成功！可在“我的订单”中查看'),
        onError: (err) => message.error((err as Error).message || '秒杀失败，请稍后重试'),
        onSettled: () => setSeckillingId(null),
      }
    )
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
                allowClear
                size="large"
                onSearch={setKeyword}
                onChange={(e) => !e.target.value && setKeyword('')}
              />
            </Col>
            <Col xs={24} sm={12}>
              <Select
                placeholder="选择类型"
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
          ) : filtered.length > 0 ? (
            <Row gutter={[16, 16]}>
              {filtered.map((coupon) => (
                <Col xs={24} sm={12} lg={8} xl={6} key={coupon.id}>
                  <CouponCard
                    coupon={coupon}
                    showActions
                    seckillLoading={seckillingId === coupon.id}
                    onSeckill={handleSeckill}
                  />
                </Col>
              ))}
            </Row>
          ) : (
            <Empty description="暂无优惠券数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
          )}
        </div>
      </div>
    </div>
  )
}

export default CouponList
