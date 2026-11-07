import { lazy, Suspense, useState } from 'react'
import type { ReactNode } from 'react'
import { App, Layout, Menu, Input, Button, Avatar, Dropdown, Badge, Spin } from 'antd'
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
  SettingOutlined,
  ClockCircleOutlined,
  BellOutlined,
} from '@ant-design/icons'
import { Link, useNavigate, useLocation, createSearchParams } from 'react-router-dom'
import { useSelector, useDispatch } from 'react-redux'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { RootState, AppDispatch } from '../store'
import { logout, selectIsAdmin } from '../store/slices/authSlice'
import { preloadRoute } from '../lib/routePreload'
import { notificationService } from '../services/notificationService'
import { notifyError } from '../lib/notifyError'
import { COUPON_LIST_PATH, COUPON_SEARCH_PARAM, headerSearchKeyword } from '../lib/couponSearch'
import type { MenuProps } from 'antd'

/** 通知面板 chunk 加载器：铃铛 hover/focus 时预取，点开下拉时 lazy 复用同一个 Promise */
const loadNotificationPanel = () => import('./NotificationPanel')

/**
 * 通知面板按需加载（List/Pagination/Select/Typography 等不进首屏入口包）。
 * chunk 加载失败（弱网、发版后旧 chunk 404）时降级为提示文案，而不是把异常抛到根组件导致整页白屏。
 */
const NotificationPanel = lazy(() =>
  loadNotificationPanel().catch((err: unknown) => {
    console.warn('[notification] 通知面板加载失败', err)
    return {
      default: () => (
        <div style={{ padding: '12px 8px', color: 'rgba(0,0,0,0.45)' }}>通知加载失败，请刷新页面重试</div>
      ),
    }
  })
)

/** 预取通知面板 chunk；失败只记录日志，真正打开时 lazy 会走上面的降级 */
function preloadNotificationPanel() {
  void loadNotificationPanel().catch((err) => console.warn('[preload] 通知面板 chunk 预取失败', err))
}

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
  const { message } = App.useApp()

  // 顶部搜索框受控：在券列表页跟随 URL ?search=（列表页内改/清关键字时同步），离开列表页清空。
  // 用「渲染期比对上一次 URL 关键字」的方式同步，而不是 effect：用户输入未提交时不会被覆盖，URL 变化时立即对齐。
  const urlKeyword = headerSearchKeyword(location.pathname, location.search)
  const [searchInput, setSearchInput] = useState(urlKeyword)
  const [syncedUrlKeyword, setSyncedUrlKeyword] = useState(urlKeyword)
  if (syncedUrlKeyword !== urlKeyword) {
    setSyncedUrlKeyword(urlKeyword)
    setSearchInput(urlKeyword)
  }

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

  /** 退出：清本地会话 + 清空 react-query 缓存（预约/通知等 key 不含 userId，不清会让下一位登录用户看到上一位的数据） */
  const handleLogout = () => {
    dispatch(logout())
    qc.clear()
    navigate('/login')
  }

  /** 顶部搜索：编码关键字后跳到券列表，由列表页读取 ?search= 过滤 */
  const handleSearch = (value: string) => {
    const keyword = value.trim()
    navigate(
      keyword
        ? {
            pathname: COUPON_LIST_PATH,
            search: `?${createSearchParams({ [COUPON_SEARCH_PARAM]: keyword })}`,
          }
        : COUPON_LIST_PATH
    )
  }

  const openNoticePanel = () => {
    void refetchNotices()
  }

  const onMarkAllRead = async () => {
    try {
      await notificationService.markAllRead()
    } catch (err) {
      console.warn('[notification] 全部已读失败', err)
      notifyError(message, err, '标记已读失败，请稍后重试')
    } finally {
      void qc.invalidateQueries({ queryKey: ['notifications'] })
    }
  }

  const onClickNotice = async (id: string) => {
    try {
      await notificationService.markRead(id)
      void qc.invalidateQueries({ queryKey: ['notifications'] })
    } catch (err) {
      // 标记失败不阻断跳转，只记录日志
      console.warn('[notification] 标记已读失败', id, err)
    }
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
            icon: <SettingOutlined />,
            label: '管理',
            children: [
              {
                key: 'admin-coupons',
                icon: <TagsOutlined />,
                label: <NavLink to="/admin/coupons">优惠券</NavLink>,
              },
              {
                key: 'admin-connector',
                icon: <ApiOutlined />,
                label: <NavLink to="/admin/connector">Connector</NavLink>,
              },
            ],
          },
        ]
      : []),
  ]

  const selectedKeys = (() => {
    if (location.pathname.startsWith('/admin/coupons')) return ['admin-coupons']
    if (location.pathname.startsWith('/admin/connector')) return ['admin-connector']
    const seg = location.pathname.split('/')[1] || 'home'
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
            className="header-search"
            placeholder="搜索优惠券..."
            aria-label="搜索优惠券"
            allowClear
            style={{ width: 300 }}
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            onSearch={handleSearch}
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
                    <Suspense
                      fallback={
                        <div style={{ padding: 24, textAlign: 'center' }}>
                          <Spin />
                        </div>
                      }
                    >
                      <NotificationPanel
                        notices={notices}
                        onMarkAllRead={() => void onMarkAllRead()}
                        onClickNotice={(id) => void onClickNotice(id)}
                      />
                    </Suspense>
                  </div>
                )}
              >
                <Button
                  type="text"
                  style={{ color: 'white' }}
                  aria-label="站内通知"
                  onMouseEnter={preloadNotificationPanel}
                  onFocus={preloadNotificationPanel}
                >
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
                <button type="button" className="user-info" aria-label="用户菜单">
                  <Avatar icon={<UserOutlined />} />
                  <span className="username">{user?.username}</span>
                </button>
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
