import type { ReactNode } from 'react'
import { Layout, Menu, Input, Button, Avatar, Dropdown, Badge, List, Typography, Space } from 'antd'
import {
  ShoppingCartOutlined,
  UserOutlined,
  LogoutOutlined,
  LoginOutlined,
  HomeOutlined,
  TagsOutlined,
  ThunderboltOutlined,
  OrderedListOutlined,
  ApiOutlined,
  ClockCircleOutlined,
  BellOutlined,
} from '@ant-design/icons'
import { Link, useNavigate, useLocation } from 'react-router-dom'
import { useSelector, useDispatch } from 'react-redux'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { RootState, AppDispatch } from '../store'
import { logout, selectIsAdmin } from '../store/slices/authSlice'
import { preloadRoute } from '../lib/routePreload'
import { notificationService } from '../services/notificationService'
import type { MenuProps } from 'antd'

const { Text } = Typography

/** 导航 Link：hover/focus 时预取对应懒加载 chunk */
function NavLink({ to, children }: { to: string; children: ReactNode }) {
  return (
    <Link to={to} onMouseEnter={() => preloadRoute(to)} onFocus={() => preloadRoute(to)}>
      {children}
    </Link>
  )
}

const { Header: AntHeader } = Layout
const { Search } = Input

const Header: React.FC = () => {
  const navigate = useNavigate()
  const location = useLocation()
  const dispatch = useDispatch<AppDispatch>()
  const { isAuthenticated, user } = useSelector((state: RootState) => state.auth)
  const isAdmin = useSelector(selectIsAdmin)
  const qc = useQueryClient()

  const { data: unread = 0 } = useQuery({
    queryKey: ['notifications', 'unread'],
    queryFn: () => notificationService.unreadCount(),
    enabled: isAuthenticated,
    refetchInterval: 30_000,
  })

  const { data: notices = [], refetch: refetchNotices } = useQuery({
    queryKey: ['notifications', 'mine'],
    queryFn: () => notificationService.listMine(15),
    enabled: isAuthenticated,
    staleTime: 15_000,
  })

  const handleLogout = () => {
    dispatch(logout())
    navigate('/login')
  }

  const openNoticePanel = () => {
    void refetchNotices()
  }

  const onMarkAllRead = async () => {
    await notificationService.markAllRead()
    void qc.invalidateQueries({ queryKey: ['notifications'] })
  }

  const onClickNotice = async (id: string) => {
    await notificationService.markRead(id)
    void qc.invalidateQueries({ queryKey: ['notifications'] })
    navigate('/reservations')
  }

  const userMenuItems: MenuProps['items'] = [
    {
      key: 'profile',
      label: '个人中心',
      icon: <UserOutlined />,
      onClick: () => navigate('/user/profile'),
    },
    {
      key: 'orders',
      label: '我的订单',
      icon: <OrderedListOutlined />,
      onClick: () => navigate('/orders'),
    },
    {
      key: 'reservations',
      label: '我的预约',
      icon: <ClockCircleOutlined />,
      onClick: () => navigate('/reservations'),
    },
    {
      type: 'divider',
    },
    {
      key: 'logout',
      label: '退出登录',
      icon: <LogoutOutlined />,
      onClick: handleLogout,
    },
  ]

  const menuItems = [
    {
      key: 'home',
      icon: <HomeOutlined />,
      label: <NavLink to="/">首页</NavLink>,
    },
    {
      key: 'coupons',
      icon: <TagsOutlined />,
      label: <NavLink to="/coupons">优惠券</NavLink>,
    },
    {
      key: 'seckill',
      icon: <ThunderboltOutlined />,
      label: <NavLink to="/seckill">秒杀专区</NavLink>,
    },
    ...(isAuthenticated
      ? [
          {
            key: 'reservations',
            icon: <ClockCircleOutlined />,
            label: <NavLink to="/reservations">我的预约</NavLink>,
          },
        ]
      : []),
    ...(isAdmin
      ? [
          {
            key: 'admin',
            icon: <ApiOutlined />,
            label: <NavLink to="/admin/connector">Connector</NavLink>,
          },
        ]
      : []),
  ]

  const selectedKeys = (() => {
    const seg = location.pathname.split('/')[1] || 'home'
    if (seg === 'admin') return ['admin']
    return [seg]
  })()

  return (
    <AntHeader className="app-header">
      <div className="header-container">
        <div className="header-left">
          <div className="logo">
            <Link to="/">
              <span className="logo-text">CouponKill</span>
            </Link>
          </div>
          <Menu
            theme="dark"
            mode="horizontal"
            selectedKeys={selectedKeys}
            items={menuItems}
            className="main-menu"
          />
        </div>

        <div className="header-center">
          <Search
            placeholder="搜索优惠券..."
            allowClear
            style={{ width: 300 }}
            onSearch={(value) => navigate(`/coupons?search=${value}`)}
          />
        </div>

        <div className="header-right">
          {isAuthenticated ? (
            <>
              <Dropdown
                trigger={['click']}
                placement="bottomRight"
                onOpenChange={(open) => {
                  if (open) openNoticePanel()
                }}
                dropdownRender={() => (
                  <div
                    style={{
                      width: 320,
                      background: '#fff',
                      borderRadius: 8,
                      boxShadow: '0 6px 16px rgba(0,0,0,0.12)',
                      padding: 8,
                    }}
                  >
                    <Space style={{ width: '100%', justifyContent: 'space-between', padding: '4px 8px' }}>
                      <Text strong>站内通知</Text>
                      <Button type="link" size="small" onClick={() => void onMarkAllRead()}>
                        全部已读
                      </Button>
                    </Space>
                    <List
                      size="small"
                      locale={{ emptyText: '暂无通知' }}
                      dataSource={notices}
                      renderItem={(item) => (
                        <List.Item
                          style={{
                            cursor: 'pointer',
                            background: item.readFlag ? undefined : '#f0f5ff',
                            padding: '8px 12px',
                          }}
                          onClick={() => void onClickNotice(item.id)}
                        >
                          <List.Item.Meta
                            title={item.title}
                            description={
                              <Text type="secondary" style={{ fontSize: 12 }}>
                                {item.content || item.type}
                              </Text>
                            }
                          />
                        </List.Item>
                      )}
                    />
                  </div>
                )}
              >
                <Button type="text" style={{ color: 'white' }} aria-label="站内通知">
                  <Badge count={unread} size="small" offset={[2, 0]}>
                    <BellOutlined style={{ color: 'white', fontSize: 16 }} />
                  </Badge>
                </Button>
              </Dropdown>

              <Button
                type="text"
                icon={<ShoppingCartOutlined />}
                style={{ color: 'white' }}
                onMouseEnter={() => preloadRoute('/orders')}
                onClick={() => navigate('/orders')}
              >
                我的订单
              </Button>

              <Dropdown
                menu={{ items: userMenuItems }}
                placement="bottomRight"
                trigger={['click']}
              >
                <div className="user-info">
                  <Avatar icon={<UserOutlined />} />
                  <span className="username">{user?.username}</span>
                </div>
              </Dropdown>
            </>
          ) : (
            <div className="auth-buttons">
              <Button
                type="text"
                icon={<LoginOutlined />}
                style={{ color: 'white' }}
                onClick={() => navigate('/login')}
              >
                登录
              </Button>
            </div>
          )}
        </div>
      </div>
    </AntHeader>
  )
}

export default Header
