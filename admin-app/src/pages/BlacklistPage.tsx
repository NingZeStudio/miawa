import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Table,
  Button,
  Form,
  Input,
  Popconfirm,
  message,
  Card,
  Space,
  Tag,
  Modal,
  Empty,
  Typography,
  Row,
  Col,
  Statistic,
  Flex,
  Pagination,
  Badge,
  Tooltip,
  theme,
} from 'antd'
import {
  DeleteOutlined,
  PlusOutlined,
  SearchOutlined,
  StopOutlined,
  GlobalOutlined,
  RobotOutlined,
  UserOutlined,
  TeamOutlined,
  ExclamationCircleOutlined,
  SafetyCertificateOutlined,
  ReloadOutlined,
} from '@ant-design/icons'
import { getBlacklistPage, addBlacklist, removeBlacklist } from '@/api/blacklist'
import { getFirewallStatus } from '@/api/firewall'
import type { BlacklistItem, BlacklistStats, FirewallStatus } from '@/types'
import { useBreakpoint } from '@/hooks/useBreakpoint'
import dayjs from 'dayjs'

const { Title, Text } = Typography

type SourceType = 'manual' | 'external' | 'auto' | 'local' | string

type FilterType = 'all' | SourceType

const SOURCE_META: Record<
  SourceType,
  { label: string; color: string; icon: React.ReactNode }
> = {
  manual: { label: '手动', color: 'blue', icon: <UserOutlined /> },
  external: { label: '外部同步', color: 'orange', icon: <GlobalOutlined /> },
  auto: { label: '自动封禁', color: 'red', icon: <RobotOutlined /> },
  // 后端自动封禁（流量超限/频率超限）写入的 source 是 local
  local: { label: '自动封禁', color: 'red', icon: <RobotOutlined /> },
}

function getSourceMeta(source: SourceType) {
  return SOURCE_META[source] || { label: source || '未知', color: 'default', icon: <TeamOutlined /> }
}

// 与后端一致的轻量校验：IPv4/IPv6 或 CIDR 网段（权威校验在服务端）
const IP_OR_CIDR_RE = /^(?:\d{1,3}(?:\.\d{1,3}){3})(?:\/\d{1,2})?$|^[0-9A-Fa-f:]+(?:\/\d{1,3})?$/

const DEFAULT_STATS: BlacklistStats = { all: 0, manual: 0, external: 0, local: 0, auto: 0 }

export function BlacklistPage() {
  const [items, setItems] = useState<BlacklistItem[]>([])
  const [total, setTotal] = useState(0)
  const [stats, setStats] = useState<BlacklistStats>(DEFAULT_STATS)
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [filter, setFilter] = useState<FilterType>('all')
  const [searchInput, setSearchInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [fwStatus, setFwStatus] = useState<FirewallStatus | null>(null)
  const [form] = Form.useForm()
  const { isMobile } = useBreakpoint()
  const {
    token: { colorBgContainer, colorBorder },
  } = theme.useToken()

  const debounceRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const mountedRef = useRef(true)
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  // 搜索输入防抖后触发服务端查询（回到第 1 页）
  useEffect(() => {
    debounceRef.current = setTimeout(() => {
      setKeyword(searchInput.trim())
      setPage(1)
    }, 400)
    return () => clearTimeout(debounceRef.current)
  }, [searchInput])

  const loadFirewallStatus = useCallback(async () => {
    try {
      const status = await getFirewallStatus()
      if (mountedRef.current) setFwStatus(status)
    } catch {
      // 防火墙状态加载失败不打断页面
    }
  }, [])

  const loadBlacklist = useCallback(async () => {
    setLoading(true)
    try {
      const data = await getBlacklistPage({ page, pageSize, source: filter, keyword })
      if (!mountedRef.current) return
      setItems(data.items || [])
      setTotal(data.total)
      setStats(data.stats || DEFAULT_STATS)
    } catch {
      if (mountedRef.current) message.error('加载黑名单失败')
    } finally {
      if (mountedRef.current) setLoading(false)
    }
  }, [page, pageSize, filter, keyword])

  useEffect(() => {
    // 数据加载延后到定时器回调：effect 体内同步触发会 setState 的函数
    // 触发 react-hooks/set-state-in-effect 告警（级联渲染风险）
    const timer = setTimeout(() => {
      void loadBlacklist()
    }, 0)
    return () => clearTimeout(timer)
  }, [loadBlacklist])

  useEffect(() => {
    const timer = setTimeout(() => {
      void loadFirewallStatus()
    }, 0)
    return () => clearTimeout(timer)
  }, [loadFirewallStatus])

  const handleAdd = async (values: { ip: string; reason: string }) => {
    try {
      await addBlacklist(values)
      message.success('添加成功')
      form.resetFields()
      setModalOpen(false)
      loadBlacklist()
      loadFirewallStatus()
    } catch (error) {
      const err = error as { response?: { data?: { error?: { message?: string } } } }
      message.error(err.response?.data?.error?.message || '添加失败')
    }
  }

  const handleRemove = async (ip: string) => {
    try {
      await removeBlacklist(ip)
      message.success('移除成功')
      loadBlacklist()
      loadFirewallStatus()
    } catch {
      message.error('移除失败')
    }
  }

  const columns = [
    {
      title: 'IP',
      dataIndex: 'ip',
      key: 'ip',
      width: 180,
      render: (ip: string) => <Text copyable={{ text: ip }}>{ip}</Text>,
    },
    {
      title: '来源',
      dataIndex: 'source',
      key: 'source',
      width: 120,
      render: (source: SourceType) => {
        const meta = getSourceMeta(source)
        return (
          <Tag icon={meta.icon} color={meta.color}>
            {meta.label}
          </Tag>
        )
      },
    },
    {
      title: '原因',
      dataIndex: 'reason',
      key: 'reason',
      ellipsis: true,
    },
    {
      title: '添加时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 170,
      render: (time: string) => dayjs(time).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      align: 'center' as const,
      render: (_: unknown, record: BlacklistItem) => (
        <Popconfirm
          title={`确定要移除 ${record.ip} 吗？`}
          onConfirm={() => handleRemove(record.ip)}
          okText="确定"
          cancelText="取消"
        >
          <Button type="link" danger icon={<DeleteOutlined />} size="small">
            移除
          </Button>
        </Popconfirm>
      ),
    },
  ]

  const renderMobileCard = (item: BlacklistItem) => {
    const meta = getSourceMeta(item.source)
    return (
      <Card
        key={item.ip}
        size="small"
        style={{ marginBottom: 12, background: colorBgContainer }}
        styles={{ body: { padding: 12 } }}
      >
        <Flex justify="space-between" align="start">
          <div style={{ minWidth: 0, flex: 1 }}>
            <Flex gap={8} align="center" wrap>
              <Text strong style={{ fontSize: 15 }}>
                {item.ip}
              </Text>
              <Tag icon={meta.icon} color={meta.color}>
                {meta.label}
              </Tag>
            </Flex>
            <div style={{ marginTop: 8, color: '#595959' }}>
              <Text type="secondary" style={{ fontSize: 13 }}>
                原因：
              </Text>
              <Text style={{ fontSize: 13 }}>{item.reason}</Text>
            </div>
            <div style={{ marginTop: 6 }}>
              <Text type="secondary" style={{ fontSize: 12 }}>
                {dayjs(item.created_at).format('YYYY-MM-DD HH:mm:ss')}
              </Text>
            </div>
          </div>
          <Popconfirm
            title={`确定要移除 ${item.ip} 吗？`}
            onConfirm={() => handleRemove(item.ip)}
            okText="确定"
            cancelText="取消"
          >
            <Button type="text" danger icon={<DeleteOutlined />} size="small" />
          </Popconfirm>
        </Flex>
      </Card>
    )
  }

  const statCards = [
    { key: 'all', title: '全部', value: stats.all ?? 0, color: 'default' },
    { key: 'manual', title: '手动', value: stats.manual ?? 0, color: 'blue' },
    { key: 'external', title: '外部', value: stats.external ?? 0, color: 'orange' },
    { key: 'auto', title: '自动', value: stats.auto ?? 0, color: 'red' },
  ]

  const fwCard = (
    <Card
      size="small"
      style={{ marginBottom: 16 }}
      styles={{ body: { padding: isMobile ? 12 : 16 } }}
      title={
        <Space>
          <SafetyCertificateOutlined />
          <span>防火墙状态</span>
          {fwStatus && (
            <Badge
              status={fwStatus.settings.enabled ? 'success' : 'default'}
              text={fwStatus.settings.enabled ? '频率限制运行中' : '频率限制已禁用'}
            />
          )}
        </Space>
      }
      extra={
        <Button icon={<ReloadOutlined />} size="small" onClick={loadFirewallStatus}>
          刷新
        </Button>
      }
    >
      {fwStatus ? (
        <Row gutter={[12, 12]}>
          <Col xs={12} sm={8} md={4}>
            <Statistic title="每分钟请求上限" value={fwStatus.settings.per_minute} />
          </Col>
          <Col xs={12} sm={8} md={4}>
            <Statistic title="自动封禁阈值" value={fwStatus.settings.ban_threshold} />
          </Col>
          <Col xs={12} sm={8} md={4}>
            <Statistic title="白名单条目" value={fwStatus.whitelist_count} />
          </Col>
          <Col xs={12} sm={8} md={4}>
            <Statistic title="网段封禁" value={fwStatus.cidr_ban_count} />
          </Col>
          <Col xs={12} sm={8} md={4}>
            <Statistic title="活跃请求 IP" value={fwStatus.tracked_ips} />
          </Col>
          <Col xs={12} sm={8} md={4}>
            <Statistic title="近 30 分钟违规 IP" value={fwStatus.active_strikes} />
          </Col>
        </Row>
      ) : (
        <Text type="secondary">正在加载防火墙状态…</Text>
      )}
    </Card>
  )

  const pagination = isMobile ? (
    <Pagination
      current={page}
      pageSize={pageSize}
      total={total}
      onChange={(p, ps) => {
        setPage(p)
        setPageSize(ps)
      }}
      showSizeChanger
      pageSizeOptions={[10, 20, 50]}
      showTotal={(t) => `共 ${t} 条`}
      style={{ marginTop: 16, textAlign: 'center' }}
    />
  ) : null

  return (
    <div>
      <Title level={4} style={{ marginTop: 0, marginBottom: 16 }}>
        <StopOutlined style={{ marginRight: 8 }} />
        黑名单管理
      </Title>

      {fwCard}

      <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
        {statCards.map((s) => (
          <Col xs={12} sm={12} md={6} key={s.key}>
            <Card
              size="small"
              style={{
                cursor: 'pointer',
                borderColor: filter === s.key ? undefined : colorBorder,
              }}
              styles={{
                body: {
                  padding: isMobile ? 12 : 16,
                  background: filter === s.key ? '#e6f4ff' : undefined,
                },
              }}
              onClick={() => {
                setFilter(s.key as FilterType)
                setPage(1)
              }}
            >
              <Statistic
                title={s.title}
                value={s.value}
                valueStyle={{ color: s.color === 'default' ? '#262626' : undefined }}
              />
            </Card>
          </Col>
        ))}
      </Row>

      <Card
        style={{ marginBottom: 16 }}
        styles={{ body: { padding: isMobile ? 12 : 16 } }}
      >
        <Flex
          gap={12}
          vertical={isMobile}
          justify="space-between"
          align={isMobile ? 'stretch' : 'center'}
        >
          <Input
            placeholder="搜索 IP 或原因"
            prefix={<SearchOutlined />}
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            allowClear
            style={{ flex: 1, maxWidth: isMobile ? '100%' : 320 }}
          />
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => setModalOpen(true)}
            block={isMobile}
          >
            添加黑名单
          </Button>
        </Flex>
      </Card>

      {isMobile ? (
        <div>
          {items.length === 0 && !loading ? (
            <Empty description="暂无数据" style={{ marginTop: 40 }} />
          ) : (
            items.map(renderMobileCard)
          )}
          {pagination}
        </div>
      ) : (
        <Table
          columns={columns}
          dataSource={items}
          rowKey="ip"
          loading={loading}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            pageSizeOptions: [10, 20, 50],
            showTotal: (t) => `共 ${t} 条`,
            onChange: (p, ps) => {
              setPage(p)
              setPageSize(ps)
            },
          }}
          scroll={{ x: true }}
          size="middle"
          locale={{ emptyText: <Empty description="暂无黑名单数据" /> }}
        />
      )}

      <Modal
        title="添加黑名单"
        open={modalOpen}
        onCancel={() => {
          setModalOpen(false)
          form.resetFields()
        }}
        footer={null}
        destroyOnClose
      >
        <Form form={form} layout="vertical" onFinish={handleAdd} style={{ marginTop: 16 }}>
          <Form.Item
            name="ip"
            label="IP / 网段"
            rules={[
              { required: true, message: '请输入 IP 地址或网段' },
              {
                pattern: IP_OR_CIDR_RE,
                message: '格式无效，支持 IPv4/IPv6 地址或 CIDR 网段（如 1.2.3.4、10.0.0.0/8）',
              },
            ]}
          >
            <Input placeholder="例如：192.168.1.1 或 10.0.0.0/8" />
          </Form.Item>
          <Form.Item
            name="reason"
            label="封禁原因"
            rules={[{ required: true, message: '请输入封禁原因' }]}
          >
            <Input.TextArea rows={3} placeholder="请输入封禁原因" />
          </Form.Item>
          <Form.Item style={{ marginBottom: 0 }}>
            <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
              <Button
                onClick={() => {
                  setModalOpen(false)
                  form.resetFields()
                }}
              >
                取消
              </Button>
              <Button type="primary" htmlType="submit" icon={<PlusOutlined />}>
                添加
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>

      {Boolean(stats.external) && (
        <div
          style={{
            marginTop: 16,
            padding: 12,
            background: '#fffbe6',
            border: '1px solid #ffe58f',
            borderRadius: 8,
            display: 'flex',
            gap: 8,
            alignItems: 'flex-start',
          }}
        >
          <Tooltip title="外部黑名单由 external_blacklist_url 定时同步生成">
            <ExclamationCircleOutlined style={{ color: '#faad14', marginTop: 3 }} />
          </Tooltip>
          <Text type="secondary" style={{ fontSize: 13 }}>
            来源为“外部同步”的记录由外部黑名单 URL 同步生成，移除后下次同步可能会重新出现。
          </Text>
        </div>
      )}
    </div>
  )
}
