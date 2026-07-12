import React, { useCallback, useEffect, useState } from 'react'
import {
  App,
  Badge,
  Button,
  Card,
  Col,
  Descriptions,
  Form,
  Input,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd'
import {
  ApiOutlined,
  CloudSyncOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
} from '@ant-design/icons'
import { connectorService } from '../services/connectorService'
import { PlatformType } from '../types/api'
import type {
  ConnectorPlatformInfo,
  PlatformProductSnapshot,
  PlatformSkuBinding,
  PlatformStockSnapshot,
} from '../types/api'
import { ApiError } from '../lib/apiClient'

const { Title, Text, Paragraph } = Typography

const PLATFORM_OPTIONS = [
  { value: PlatformType.MOCK, label: 'MOCK（本地联调）' },
  { value: PlatformType.JD, label: '京东 JD' },
  { value: PlatformType.TB, label: '淘宝 TB（二期预留）', disabled: true },
  { value: PlatformType.PDD, label: '拼多多 PDD（二期预留）', disabled: true },
]

const statusColor = (status?: string) => {
  switch (status) {
    case 'UP':
      return 'success'
    case 'DOWN':
      return 'error'
    case 'DISABLED':
      return 'default'
    case 'CONFIGURED':
      return 'processing'
    case 'SUCCESS':
      return 'success'
    case 'FAIL':
      return 'error'
    case 'SKIP':
      return 'warning'
    default:
      return 'processing'
  }
}

const ConnectorAdmin: React.FC = () => {
  const { message } = App.useApp()
  const [platforms, setPlatforms] = useState<ConnectorPlatformInfo[]>([])
  const [bindings, setBindings] = useState<PlatformSkuBinding[]>([])
  const [loadingHealth, setLoadingHealth] = useState(false)
  const [loadingBindings, setLoadingBindings] = useState(false)
  const [syncingId, setSyncingId] = useState<string | null>(null)
  const [syncingAll, setSyncingAll] = useState(false)
  const [forceSync, setForceSync] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [probing, setProbing] = useState(false)
  const [probeStock, setProbeStock] = useState<PlatformStockSnapshot | null>(null)
  const [probeProduct, setProbeProduct] = useState<PlatformProductSnapshot | null>(null)

  const [bindForm] = Form.useForm()
  const [probeForm] = Form.useForm()

  const loadPlatforms = useCallback(async () => {
    setLoadingHealth(true)
    try {
      // platforms 含 JD 配置态；失败则退回 health
      try {
        setPlatforms(await connectorService.getPlatforms())
      } catch {
        const health = await connectorService.getHealth()
        setPlatforms(health)
      }
    } catch (e) {
      message.error((e as Error).message || '加载平台健康失败')
    } finally {
      setLoadingHealth(false)
    }
  }, [message])

  const loadBindings = useCallback(async () => {
    setLoadingBindings(true)
    try {
      const list = await connectorService.listBindings()
      setBindings(
        (list || []).map((b) => ({
          ...b,
          id: String(b.id),
          couponId: String(b.couponId),
        }))
      )
    } catch (e) {
      if (e instanceof ApiError && e.code === 401) {
        message.warning('请先登录后再管理绑定')
      } else {
        message.error((e as Error).message || '加载绑定失败')
      }
    } finally {
      setLoadingBindings(false)
    }
  }, [message])

  useEffect(() => {
    void loadPlatforms()
    void loadBindings()
  }, [loadPlatforms, loadBindings])

  const onCreateBinding = async () => {
    try {
      const values = await bindForm.validateFields()
      setSubmitting(true)
      await connectorService.createOrUpdateBinding({
        platform: values.platform,
        externalSkuId: values.externalSkuId.trim(),
        couponId: values.couponId,
        syncEnabled: values.syncEnabled ?? true,
      })
      message.success('绑定已保存')
      bindForm.resetFields(['externalSkuId', 'couponId'])
      await loadBindings()
    } catch (e) {
      if ((e as { errorFields?: unknown }).errorFields) return
      message.error((e as Error).message || '保存失败')
    } finally {
      setSubmitting(false)
    }
  }

  const onSyncOne = async (id: string) => {
    setSyncingId(id)
    try {
      const updated = await connectorService.syncOne(id, forceSync)
      message.success(
        updated.lastSyncStatus === 'SUCCESS'
          ? `同步成功，Redis库存=${updated.lastStock}${forceSync ? '（强制覆盖）' : '（安全合并）'}${updated.lastError ? ' · ' + updated.lastError : ''}`
          : `同步结束：${updated.lastSyncStatus} ${updated.lastError || ''}`
      )
      await loadBindings()
    } catch (e) {
      message.error((e as Error).message || '同步失败')
    } finally {
      setSyncingId(null)
    }
  }

  const onSyncAll = async () => {
    setSyncingAll(true)
    try {
      const r = await connectorService.syncAll(forceSync)
      message.success(
        `同步完成：成功 ${r.syncedOk}，失败 ${r.failed ?? 0}，跳过 ${r.skipped ?? 0}` +
          (forceSync ? '（强制）' : '（安全合并）')
      )
      await loadBindings()
    } catch (e) {
      message.error((e as Error).message || '批量同步失败')
    } finally {
      setSyncingAll(false)
    }
  }

  const onProbe = async () => {
    try {
      const values = await probeForm.validateFields()
      setProbing(true)
      setProbeStock(null)
      setProbeProduct(null)
      const platform = values.platform as string
      const skuId = String(values.skuId).trim()
      const [stock, product] = await Promise.allSettled([
        connectorService.probeStock(platform, skuId),
        connectorService.probeProduct(platform, skuId),
      ])
      if (stock.status === 'fulfilled') {
        setProbeStock(stock.value)
      } else {
        message.error(`库存探测失败：${(stock.reason as Error).message}`)
      }
      if (product.status === 'fulfilled') {
        setProbeProduct(product.value)
      } else {
        message.warning(`商品探测失败：${(product.reason as Error).message}`)
      }
      if (stock.status === 'fulfilled' || product.status === 'fulfilled') {
        message.success('探测完成（仅读平台，未写入 Redis）')
      }
    } catch (e) {
      if ((e as { errorFields?: unknown }).errorFields) return
      message.error((e as Error).message || '探测失败')
    } finally {
      setProbing(false)
    }
  }

  const jdInfo = platforms.find((p) => p.platform === PlatformType.JD)

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 72 },
    {
      title: '平台',
      dataIndex: 'platform',
      width: 88,
      render: (v: string) => <Tag>{v}</Tag>,
    },
    { title: '外部 SKU', dataIndex: 'externalSkuId', ellipsis: true },
    { title: '本地券 ID', dataIndex: 'couponId', width: 100 },
    {
      title: '同步',
      dataIndex: 'syncEnabled',
      width: 72,
      render: (v: boolean) => (v ? <Badge status="success" text="开" /> : <Badge status="default" text="关" />),
    },
    {
      title: 'Redis库存',
      dataIndex: 'lastStock',
      width: 100,
      render: (v: number | null | undefined) => (v == null ? '-' : v),
    },
    {
      title: '状态',
      dataIndex: 'lastSyncStatus',
      width: 100,
      render: (v: string) => (v ? <Tag color={statusColor(v)}>{v}</Tag> : '-'),
    },
    {
      title: '上次同步',
      dataIndex: 'lastSyncAt',
      width: 180,
      render: (v: string) => v || '-',
    },
    {
      title: '错误',
      dataIndex: 'lastError',
      ellipsis: true,
      render: (v: string) => v || '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 110,
      fixed: 'right' as const,
      render: (_: unknown, row: PlatformSkuBinding) => (
        <Button
          type="link"
          size="small"
          icon={<CloudSyncOutlined />}
          loading={syncingId === row.id}
          onClick={() => void onSyncOne(row.id)}
        >
          同步
        </Button>
      ),
    },
  ]

  return (
    <div className="container" style={{ padding: '24px 16px 48px' }}>
      <div className="page-header" style={{ marginBottom: 24 }}>
        <Title level={2}>
          <ApiOutlined /> Connector 管理
        </Title>
        <Text type="secondary">
          旁路同步外部电商库存到本地秒杀 Redis；不进入秒杀热路径。绑定/同步需登录；健康检查可匿名。
        </Text>
      </div>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={10}>
          <Card
            title="平台健康"
            extra={
              <Button icon={<ReloadOutlined />} size="small" loading={loadingHealth} onClick={() => void loadPlatforms()}>
                刷新
              </Button>
            }
            loading={loadingHealth}
          >
            <Space direction="vertical" style={{ width: '100%' }} size="middle">
              {platforms.map((p) => (
                <Card key={String(p.platform)} size="small" type="inner" title={<Tag color={statusColor(p.status)}>{p.platform}</Tag>}>
                  <Paragraph style={{ marginBottom: 0 }}>
                    状态：<Tag color={statusColor(p.status)}>{p.status}</Tag>
                    <br />
                    {p.message}
                  </Paragraph>
                </Card>
              ))}
              {!platforms.length && <Text type="secondary">暂无平台信息</Text>}
            </Space>
          </Card>
        </Col>

        <Col xs={24} lg={14}>
          <Card title="京东密钥就绪态（不含明文密钥）">
            {jdInfo ? (
              <Descriptions column={1} size="small" bordered>
                <Descriptions.Item label="enabled">{String(jdInfo.jdEnabled ?? false)}</Descriptions.Item>
                <Descriptions.Item label="credentials">
                  {jdInfo.jdCredentialsConfigured ? (
                    <Tag color="success">已配置三件套</Tag>
                  ) : (
                    <Tag>未配置 — 设置 JD_CONNECTOR_ENABLED + JD_APP_KEY / SECRET / ACCESS_TOKEN 后重启 connector</Tag>
                  )}
                </Descriptions.Item>
                <Descriptions.Item label="serverUrl">{jdInfo.jdServerUrl || '-'}</Descriptions.Item>
                <Descriptions.Item label="defaultArea">{jdInfo.jdDefaultArea || '-'}</Descriptions.Item>
                <Descriptions.Item label="appKey">{jdInfo.jdAppKeyMasked || '（空）'}</Descriptions.Item>
                <Descriptions.Item label="health">
                  <Tag color={statusColor(jdInfo.status)}>{jdInfo.status}</Tag> {jdInfo.message}
                </Descriptions.Item>
              </Descriptions>
            ) : (
              <Text type="secondary">加载中或 JD 平台未注册</Text>
            )}
            <Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
              默认同步为「安全合并」：不会把 Redis 库存抬高（防秒杀扣减被虚增）。需要校准抬高时打开「强制覆盖」。
              JD 状态 CONFIGURED 表示密钥已配但未探活，请用下方探测验证。
            </Paragraph>
          </Card>
        </Col>
      </Row>

      <Card title="平台探测（只读，不写 Redis）" style={{ marginTop: 16 }}>
        <Form
          form={probeForm}
          layout="inline"
          initialValues={{ platform: PlatformType.JD }}
          style={{ marginBottom: 16, rowGap: 12 }}
        >
          <Form.Item name="platform" label="平台" rules={[{ required: true }]}>
            <Select
              style={{ width: 160 }}
              options={[
                { value: PlatformType.MOCK, label: 'MOCK' },
                { value: PlatformType.JD, label: 'JD' },
                { value: PlatformType.TB, label: 'TB（未实现）' },
                { value: PlatformType.PDD, label: 'PDD（未实现）' },
              ]}
            />
          </Form.Item>
          <Form.Item name="skuId" label="SKU" rules={[{ required: true, message: '填写 skuId' }]}>
            <Input placeholder="如 mock-sku:888 或京东 skuId" style={{ width: 240 }} allowClear />
          </Form.Item>
          <Form.Item>
            <Button type="primary" icon={<SearchOutlined />} loading={probing} onClick={() => void onProbe()}>
              探测库存/商品
            </Button>
          </Form.Item>
        </Form>
        <Row gutter={16}>
          <Col xs={24} md={12}>
            <Card size="small" title="库存快照" type="inner">
              {probeStock ? (
                <Descriptions column={1} size="small">
                  <Descriptions.Item label="qty">{probeStock.stockQty ?? 'null'}</Descriptions.Item>
                  <Descriptions.Item label="available">{String(probeStock.stockAvailable)}</Descriptions.Item>
                  <Descriptions.Item label="state">{probeStock.stockStateDesc || '-'}</Descriptions.Item>
                  <Descriptions.Item label="area">{probeStock.area || '-'}</Descriptions.Item>
                </Descriptions>
              ) : (
                <Text type="secondary">尚未探测</Text>
              )}
            </Card>
          </Col>
          <Col xs={24} md={12}>
            <Card size="small" title="商品快照" type="inner">
              {probeProduct ? (
                <Descriptions column={1} size="small">
                  <Descriptions.Item label="title">{probeProduct.title || '-'}</Descriptions.Item>
                  <Descriptions.Item label="price">{probeProduct.price ?? '-'}</Descriptions.Item>
                  <Descriptions.Item label="onSale">{String(probeProduct.onSale)}</Descriptions.Item>
                  <Descriptions.Item label="raw">{probeProduct.rawStatus || '-'}</Descriptions.Item>
                </Descriptions>
              ) : (
                <Text type="secondary">尚未探测</Text>
              )}
            </Card>
          </Col>
        </Row>
      </Card>

      <Card
        title="SKU ↔ 本地券绑定"
        style={{ marginTop: 16 }}
        extra={
          <Space>
            <span>
              强制覆盖{' '}
              <Switch
                checked={forceSync}
                onChange={setForceSync}
                checkedChildren="校准"
                unCheckedChildren="安全"
              />
            </span>
            <Button icon={<ReloadOutlined />} onClick={() => void loadBindings()} loading={loadingBindings}>
              刷新
            </Button>
            <Button type="primary" ghost icon={<CloudSyncOutlined />} loading={syncingAll} onClick={() => void onSyncAll()}>
              同步全部启用
            </Button>
          </Space>
        }
      >
        <Form
          form={bindForm}
          layout="inline"
          initialValues={{ platform: PlatformType.MOCK, syncEnabled: true, couponId: '1001' }}
          style={{ marginBottom: 16, rowGap: 12 }}
        >
          <Form.Item name="platform" label="平台" rules={[{ required: true }]}>
            <Select style={{ width: 200 }} options={PLATFORM_OPTIONS} />
          </Form.Item>
          <Form.Item
            name="externalSkuId"
            label="外部 SKU"
            rules={[{ required: true, message: '必填' }]}
          >
            <Input placeholder="mock-sku:888 或 JD skuId" style={{ width: 220 }} />
          </Form.Item>
          <Form.Item name="couponId" label="券 ID" rules={[{ required: true, message: '必填' }]}>
            <Input placeholder="如 1001" style={{ width: 140 }} />
          </Form.Item>
          <Form.Item name="syncEnabled" label="启用同步" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item>
            <Button type="primary" icon={<PlusOutlined />} loading={submitting} onClick={() => void onCreateBinding()}>
              保存绑定
            </Button>
          </Form.Item>
        </Form>

        <Table
          rowKey="id"
          size="middle"
          loading={loadingBindings}
          columns={columns}
          dataSource={bindings}
          scroll={{ x: 1100 }}
          pagination={{ pageSize: 10 }}
        />
      </Card>
    </div>
  )
}

export default ConnectorAdmin
