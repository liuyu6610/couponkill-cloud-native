import type { ReactNode } from 'react'
import { Layout, Menu, Input, Button, Avatar, Dropdown } from 'antd'
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
} from '@ant-design/icons'
import { Link, useNavigate, useLocation } from 'react-router-dom'
import { useSelector, useDispatch } from 'react-redux'
import type { RootState, AppDispatch } from '../store'
import { logout, selectIsAdmin } from '../store/slices/authSlice'
import { preloadRoute } from '../lib/routePreload'
import type { MenuProps } from 'antd'

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

  const handleLogout = () => {
    dispatch(logout())
    navigate('/login')
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
