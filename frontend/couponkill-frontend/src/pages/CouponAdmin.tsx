import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  App,
  Button,
  Card,
  Col,
  DatePicker,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd'
import {
  CheckCircleOutlined,
  DeleteOutlined,
  PlusOutlined,
  ReloadOutlined,
  StopOutlined,
} from '@ant-design/icons'
import dayjs, { type Dayjs } from 'dayjs'
import { couponService } from '../services/couponService'
import { CouponStatus, CouponType } from '../types/api'
import type { Coupon } from '../types/api'
import { getErrorMessage } from '../lib/errorMessage'

const { Title, Paragraph, Text } = Typography
const { RangePicker } = DatePicker
const { Search } = Input

const FMT = 'YYYY-MM-DD HH:mm:ss'

type TypeFilter = 'all' | typeof CouponType.NORMAL | typeof CouponType.SECKILL
type StatusFilter = 'all' | typeof CouponStatus.VALID | typeof CouponStatus.INVALID

const CouponAdmin: React.FC = () => {
  const { message } = App.useApp()
  const [loading, setLoading] = useState(false)
  const [batching, setBatching] = useState(false)
  const [coupons, setCoupons] = useState<Coupon[]>([])
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([])
  const [keyword, setKeyword] = useState('')
  const [typeFilter, setTypeFilter] = useState<TypeFilter>('all')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [createForm] = Form.useForm()
  const [windowForm] = Form.useForm()
  const createType = Form.useWatch('type', createForm)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const list = await couponService.getAllCoupons()
      setCoupons(list ?? [])
    } catch (e) {
      message.error(getErrorMessage(e, '加载优惠券失败'))
    } finally {
      setLoading(false)
    }
  }, [message])

  useEffect(() => {
    void load()
  }, [load])

  const filtered = useMemo(() => {
    const q = keyword.trim().toLowerCase()
    return coupons.filter((c) => {
      if (typeFilter !== 'all' && c.type !== typeFilter) return false
      if (statusFilter !== 'all' && c.status !== statusFilter) return false
      if (!q) return true
      return (
        String(c.id).toLowerCase().includes(q) ||
        (c.name || '').toLowerCase().includes(q) ||
        (c.description || '').toLowerCase().includes(q)
      )
    })
  }, [coupons, keyword, typeFilter, statusFilter])

  // 筛选变化后去掉已不在结果集中的勾选
  useEffect(() => {
    const visible = new Set(filtered.map((c) => c.id))
    setSelectedRowKeys((prev) => prev.filter((k) => visible.has(String(k))))
  }, [filtered])

  const onCreate = async () => {
    try {
      const v = await createForm.validateFields()
      const range = v.seckillRange as [Dayjs, Dayjs] | undefined
      const created = await couponService.createCoupon({
        name: v.name.trim(),
        description: v.description?.trim() || undefined,
        type: v.type,
        faceValue: v.faceValue,
        minSpend: v.minSpend,
        validDays: v.validDays,
        perUserLimit: v.perUserLimit,
        totalStock: v.totalStock,
        seckillTotalStock: v.type === CouponType.SECKILL ? v.seckillTotalStock : undefined,
        seckillStartAt:
          v.type === CouponType.SECKILL && range?.[0] ? range[0].format(FMT) : undefined,
        seckillEndAt:
          v.type === CouponType.SECKILL && range?.[1] ? range[1].format(FMT) : undefined,
      })
      message.success(`已创建券 ${created.id}（全分片写）`)
      createForm.resetFields()
      createForm.setFieldsValue({
        type: CouponType.SECKILL,
        faceValue: 10,
        minSpend: 0,
        validDays: 7,
        perUserLimit: 1,
        totalStock: 32,
        seckillTotalStock: 32,
      })
      await load()
    } catch (e) {
      if ((e as { errorFields?: unknown }).errorFields) return
      message.error(getErrorMessage(e, '创建失败'))
    }
  }

  const onSaveWindow = async (id: string) => {
    try {
      const range = windowForm.getFieldValue(`range_${id}`) as [Dayjs, Dayjs] | undefined
      if (!range?.[0] || !range?.[1]) {
        message.warning('请选择秒杀起止时间')
        return
      }
      await couponService.updateSeckillWindow(id, range[0].format(FMT), range[1].format(FMT))
      message.success('时间窗已更新（全分片）')
      await load()
    } catch (e) {
      message.error(getErrorMessage(e, '更新时间窗失败'))
    }
  }

  const onToggleStatus = async (id: string, checked: boolean) => {
    try {
      await couponService.updateCouponStatus(
        id,
        checked ? CouponStatus.VALID : CouponStatus.INVALID
      )
      message.success(checked ? '已启用' : '已停用')
      await load()
    } catch (e) {
      message.error(getErrorMessage(e, '更新状态失败'))
    }
  }

  const runBatchStatus = async (status: number) => {
    const ids = selectedRowKeys.map(String)
    if (ids.length === 0) {
      message.warning('请先勾选要操作的券')
      return
    }
    setBatching(true)
    try {
      const results = await Promise.allSettled(
        ids.map((id) => couponService.updateCouponStatus(id, status))
      )
      const ok = results.filter((r) => r.status === 'fulfilled').length
      const fail = results.length - ok
      if (fail === 0) {
        message.success(`批量${status === CouponStatus.VALID ? '启用' : '停用'}成功：${ok} 张`)
      } else {
        message.warning(
          `批量${status === CouponStatus.VALID ? '启用' : '停用'}：成功 ${ok}，失败 ${fail}`
        )
      }
      setSelectedRowKeys([])
      await load()
    } finally {
      setBatching(false)
    }
  }

  const runBatchDelete = async () => {
    const ids = selectedRowKeys.map(String)
    if (ids.length === 0) {
      message.warning('请先勾选要删除的券')
      return
    }
    setBatching(true)
    try {
      const results = await Promise.allSettled(ids.map((id) => couponService.deleteCoupon(id)))
      const ok = results.filter((r) => r.status === 'fulfilled').length
      const fail = results.length - ok
      if (fail === 0) {
        message.success(`批量删除成功：${ok} 张（全分片）`)
      } else {
        message.warning(`批量删除：成功 ${ok}，失败 ${fail}`)
      }
      setSelectedRowKeys([])
      await load()
    } finally {
      setBatching(false)
    }
  }

  const onDelete = async (id: string) => {
    try {
      await couponService.deleteCoupon(id)
      message.success('已删除（全分片）')
      setSelectedRowKeys((prev) => prev.filter((k) => String(k) !== id))
      await load()
    } catch (e) {
      message.error(getErrorMessage(e, '删除失败'))
    }
  }

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 120,
      render: (id: string) => <Text code>{id}</Text>,
    },
    {
      title: '名称',
      dataIndex: 'name',
      ellipsis: true,
    },
    {
      title: '类型',
      dataIndex: 'type',
      width: 90,
      render: (t: number) =>
        t === CouponType.SECKILL ? <Tag color="red">秒抢</Tag> : <Tag>常驻</Tag>,
    },
    {
      title: '库存',
      width: 110,
      render: (_: unknown, r: Coupon) => (
        <Text>
          {r.remainingStock}/{r.totalStock}
        </Text>
      ),
    },
    {
      title: '秒杀窗',
      width: 360,
      render: (_: unknown, r: Coupon) => {
        if (r.type !== CouponType.SECKILL) return <Text type="secondary">—</Text>
        const initial =
          r.seckillStartAt && r.seckillEndAt
            ? [dayjs(r.seckillStartAt), dayjs(r.seckillEndAt)]
            : undefined
        return (
          <Space direction="vertical" size={4}>
            <RangePicker
              showTime
              format={FMT}
              defaultValue={initial as [Dayjs, Dayjs] | undefined}
              onChange={(v) => windowForm.setFieldValue(`range_${r.id}`, v)}
              style={{ width: '100%' }}
            />
            <Button size="small" type="link" onClick={() => void onSaveWindow(r.id)}>
              保存时间窗
            </Button>
          </Space>
        )
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (s: number, r: Coupon) => (
        <Switch
          checked={s === CouponStatus.VALID}
          checkedChildren="有效"
          unCheckedChildren="无效"
          onChange={(c) => void onToggleStatus(r.id, c)}
        />
      ),
    },
    {
      title: '操作',
      width: 90,
      render: (_: unknown, r: Coupon) => (
        <Popconfirm title={`确认删除券 ${r.id}？`} onConfirm={() => void onDelete(r.id)}>
          <Button size="small" danger icon={<DeleteOutlined />}>
            删除
          </Button>
        </Popconfirm>
      ),
    },
  ]

  return (
    <div className="page-container" style={{ padding: 24 }}>
      <Title level={3}>优惠券管理</Title>
      <Paragraph type="secondary">
        经网关 JWT + admin：创建 / 秒杀时间窗 / 启停 / 删除（全分片）。库存预热与扣减仍仅集群内部可达。
      </Paragraph>

      <Card
        title="创建优惠券"
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => void onCreate()}>
            创建
          </Button>
        }
        style={{ marginBottom: 16 }}
      >
        <Form
          form={createForm}
          layout="vertical"
          initialValues={{
            type: CouponType.SECKILL,
            faceValue: 10,
            minSpend: 0,
            validDays: 7,
            perUserLimit: 1,
            totalStock: 32,
            seckillTotalStock: 32,
          }}
        >
          <Row gutter={16}>
            <Col xs={24} md={8}>
              <Form.Item name="name" label="名称" rules={[{ required: true, message: '必填' }]}>
                <Input placeholder="例如：夏日秒杀券" maxLength={64} />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item name="type" label="类型" rules={[{ required: true }]}>
                <Select
                  options={[
                    { value: CouponType.NORMAL, label: '常驻' },
                    { value: CouponType.SECKILL, label: '秒抢' },
                  ]}
                />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item name="description" label="描述">
                <Input placeholder="可选" maxLength={200} />
              </Form.Item>
            </Col>
            <Col xs={12} md={4}>
              <Form.Item name="faceValue" label="面值" rules={[{ required: true }]}>
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={12} md={4}>
              <Form.Item name="minSpend" label="门槛" rules={[{ required: true }]}>
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={12} md={4}>
              <Form.Item name="validDays" label="有效天" rules={[{ required: true }]}>
                <InputNumber min={1} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={12} md={4}>
              <Form.Item name="perUserLimit" label="限领" rules={[{ required: true }]}>
                <InputNumber min={1} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={12} md={4}>
              <Form.Item name="totalStock" label="总库存" rules={[{ required: true }]}>
                <InputNumber min={1} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            {createType === CouponType.SECKILL && (
              <>
                <Col xs={12} md={4}>
                  <Form.Item name="seckillTotalStock" label="秒杀库存" rules={[{ required: true }]}>
                    <InputNumber min={0} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item
                    name="seckillRange"
                    label="秒杀时间窗"
                    rules={[{ required: true, message: '秒抢券必须设时间窗' }]}
                  >
                    <RangePicker showTime format={FMT} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </>
            )}
          </Row>
        </Form>
      </Card>

      <Card
        title={
          <Space>
            <span>券列表</span>
            <Text type="secondary" style={{ fontWeight: 400, fontSize: 13 }}>
              显示 {filtered.length}/{coupons.length}
              {selectedRowKeys.length > 0 ? ` · 已选 ${selectedRowKeys.length}` : ''}
            </Text>
          </Space>
        }
        extra={
          <Button icon={<ReloadOutlined />} onClick={() => void load()} loading={loading}>
            刷新
          </Button>
        }
      >
        <Space wrap style={{ marginBottom: 16 }} size="middle">
          <Search
            allowClear
            placeholder="搜索 ID / 名称 / 描述"
            style={{ width: 240 }}
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            onSearch={setKeyword}
          />
          <Select<TypeFilter>
            style={{ width: 120 }}
            value={typeFilter}
            onChange={setTypeFilter}
            options={[
              { value: 'all', label: '全部类型' },
              { value: CouponType.NORMAL, label: '常驻' },
              { value: CouponType.SECKILL, label: '秒抢' },
            ]}
          />
          <Select<StatusFilter>
            style={{ width: 120 }}
            value={statusFilter}
            onChange={setStatusFilter}
            options={[
              { value: 'all', label: '全部状态' },
              { value: CouponStatus.VALID, label: '有效' },
              { value: CouponStatus.INVALID, label: '无效' },
            ]}
          />
          <Button
            onClick={() => {
              setKeyword('')
              setTypeFilter('all')
              setStatusFilter('all')
            }}
          >
            重置筛选
          </Button>
          <Popconfirm
            title={`确认批量启用选中的 ${selectedRowKeys.length} 张券？`}
            disabled={selectedRowKeys.length === 0 || batching}
            onConfirm={() => void runBatchStatus(CouponStatus.VALID)}
          >
            <Button
              type="primary"
              icon={<CheckCircleOutlined />}
              disabled={selectedRowKeys.length === 0}
              loading={batching}
            >
              批量启用
            </Button>
          </Popconfirm>
          <Popconfirm
            title={`确认批量停用选中的 ${selectedRowKeys.length} 张券？`}
            disabled={selectedRowKeys.length === 0 || batching}
            onConfirm={() => void runBatchStatus(CouponStatus.INVALID)}
          >
            <Button
              danger
              icon={<StopOutlined />}
              disabled={selectedRowKeys.length === 0}
              loading={batching}
            >
              批量停用
            </Button>
          </Popconfirm>
          <Popconfirm
            title={`确认批量删除选中的 ${selectedRowKeys.length} 张券？`}
            description="将删除全部物理分片并清理缓存，不可恢复。"
            disabled={selectedRowKeys.length === 0 || batching}
            okText="确认删除"
            okButtonProps={{ danger: true }}
            onConfirm={() => void runBatchDelete()}
          >
            <Button
              danger
              type="primary"
              icon={<DeleteOutlined />}
              disabled={selectedRowKeys.length === 0}
              loading={batching}
            >
              批量删除
            </Button>
          </Popconfirm>
        </Space>

        <Form form={windowForm} component={false} />
        <Table
          rowKey="id"
          loading={loading || batching}
          dataSource={filtered}
          columns={columns}
          rowSelection={{
            selectedRowKeys,
            onChange: setSelectedRowKeys,
          }}
          pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 条` }}
          scroll={{ x: 1100 }}
        />
      </Card>
    </div>
  )
}

export default CouponAdmin
