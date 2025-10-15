import React, { useEffect, useState } from 'react'
import { Row, Col, Typography, Input, Select, Pagination, Empty, Spin } from 'antd'
import { useDispatch, useSelector } from 'react-redux'
import { useSearchParams } from 'react-router-dom'
import { fetchCoupons, setFilters, setPagination } from '../store/slices/couponSlice'
import type { RootState } from '../store'
import CouponCard from '../components/CouponCard'

const { Title, Text } = Typography
const { Search } = Input
const { Option } = Select

const CouponList: React.FC = () => {
  const dispatch = useDispatch()
  const [searchParams, setSearchParams] = useSearchParams()
  const { coupons, isLoading, filters, pagination } = useSelector((state: RootState) => state.coupon)

  const [searchText, setSearchText] = useState(filters.search || '')
  const [typeFilter, setTypeFilter] = useState(filters.type || '')
  const [statusFilter, setStatusFilter] = useState(filters.status || '')

  useEffect(() => {
    // 从URL参数初始化筛选条件
    const search = searchParams.get('search') || ''
    const type = searchParams.get('type') || ''
    const status = searchParams.get('status') || ''

    if (search || type || status) {
      dispatch(setFilters({ search, type, status }) as any)
      setSearchText(search)
      setTypeFilter(type)
      setStatusFilter(status)
    }

    // 获取优惠券列表
    dispatch(fetchCoupons({
      page: pagination.page,
      size: pagination.size,
      search,
      type,
      status
    }) as any)
  }, [dispatch, searchParams, pagination.page, pagination.size])

  const handleSearch = (value: string) => {
    setSearchText(value)
    const newFilters = { ...filters, search: value, page: 1 }
    dispatch(setFilters(newFilters) as any)

    // 更新URL参数
    const params = new URLSearchParams()
    if (value) params.set('search', value)
    if (typeFilter) params.set('type', typeFilter)
    if (statusFilter) params.set('status', statusFilter)
    setSearchParams(params)

    dispatch(fetchCoupons({
      page: 1,
      size: pagination.size,
      search: value,
      type: typeFilter,
      status: statusFilter
    }) as any)
  }

  const handleTypeFilter = (value: string) => {
    setTypeFilter(value)
    const newFilters = { ...filters, type: value, page: 1 }
    dispatch(setFilters(newFilters) as any)

    const params = new URLSearchParams()
    if (searchText) params.set('search', searchText)
    if (value) params.set('type', value)
    if (statusFilter) params.set('status', statusFilter)
    setSearchParams(params)

    dispatch(fetchCoupons({
      page: 1,
      size: pagination.size,
      search: searchText,
      type: value,
      status: statusFilter
    }) as any)
  }

  const handleStatusFilter = (value: string) => {
    setStatusFilter(value)
    const newFilters = { ...filters, status: value, page: 1 }
    dispatch(setFilters(newFilters) as any)

    const params = new URLSearchParams()
    if (searchText) params.set('search', searchText)
    if (typeFilter) params.set('type', typeFilter)
    if (value) params.set('status', value)
    setSearchParams(params)

    dispatch(fetchCoupons({
      page: 1,
      size: pagination.size,
      search: searchText,
      type: typeFilter,
      status: value
    }) as any)
  }

  const handlePageChange = (page: number, size: number) => {
    dispatch(setPagination({ page, size }) as any)
    dispatch(fetchCoupons({
      page,
      size,
      search: searchText,
      type: typeFilter,
      status: statusFilter
    }) as any)
  }

  return (
    <div className="coupon-list-page">
      <div className="container">
        {/* 页面标题 */}
        <div className="page-header">
          <Title level={2}>优惠券列表</Title>
          <Text type="secondary">发现更多优质优惠，总有一款适合你</Text>
        </div>

        {/* 搜索和筛选 */}
        <div className="filters-section">
          <Row gutter={[16, 16]} align="middle">
            <Col xs={24} sm={8}>
              <Search
                placeholder="搜索优惠券..."
                value={searchText}
                onChange={(e) => setSearchText(e.target.value)}
                onSearch={handleSearch}
                allowClear
                size="large"
              />
            </Col>
            <Col xs={24} sm={8}>
              <Select
                placeholder="选择类型"
                value={typeFilter}
                onChange={handleTypeFilter}
                style={{ width: '100%' }}
                size="large"
                allowClear
              >
                <Option value="DISCOUNT">折扣券</Option>
                <Option value="CASH">现金券</Option>
                <Option value="PERCENTAGE">百分比券</Option>
              </Select>
            </Col>
            <Col xs={24} sm={8}>
              <Select
                placeholder="选择状态"
                value={statusFilter}
                onChange={handleStatusFilter}
                style={{ width: '100%' }}
                size="large"
                allowClear
              >
                <Option value="ACTIVE">进行中</Option>
                <Option value="INACTIVE">未开始</Option>
                <Option value="EXPIRED">已过期</Option>
              </Select>
            </Col>
          </Row>
        </div>

        {/* 优惠券列表 */}
        <div className="coupon-list-section">
          {isLoading ? (
            <div className="loading-container">
              <Spin size="large" />
            </div>
          ) : coupons.length > 0 ? (
            <>
              <Row gutter={[16, 16]}>
                {coupons.map((coupon) => (
                  <Col xs={24} sm={12} lg={8} xl={6} key={coupon.id}>
                    <CouponCard coupon={coupon} showActions={true} />
                  </Col>
                ))}
              </Row>

              {/* 分页 */}
              <div className="pagination-container">
                <Pagination
                  current={pagination.page}
                  total={pagination.total}
                  pageSize={pagination.size}
                  onChange={handlePageChange}
                  showSizeChanger
                  showQuickJumper
                  showTotal={(total, range) =>
                    `第 ${range[0]}-${range[1]} 条，共 ${total} 条`
                  }
                />
              </div>
            </>
          ) : (
            <Empty
              description="暂无优惠券数据"
              image={Empty.PRESENTED_IMAGE_SIMPLE}
            />
          )}
        </div>
      </div>
    </div>
  )
}

export default CouponList
