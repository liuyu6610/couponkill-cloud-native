import { Layout, Menu, Input, Button, Badge, Avatar, Dropdown } from 'antd'
import {
  ShoppingCartOutlined,
  UserOutlined,
  LogoutOutlined,
  LoginOutlined,
  HomeOutlined,
  TagsOutlined,
  ThunderboltOutlined,
  OrderedListOutlined
} from '@ant-design/icons'
import { Link, useNavigate, useLocation } from 'react-router-dom'
import { useSelector, useDispatch } from 'react-redux'
import type { RootState, AppDispatch } from '../store'
import { logoutUser } from '../store/slices/authSlice'
import type { MenuProps } from 'antd'

const { Header: AntHeader } = Layout
const { Search } = Input

const Header: React.FC = () => {
  const navigate = useNavigate()
  const location = useLocation()
  const dispatch = useDispatch<AppDispatch>()
  const { isAuthenticated, user } = useSelector((state: RootState) => state.auth)

  const handleLogout = () => {
    dispatch(logoutUser())
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
      label: <Link to="/">首页</Link>,
    },
    {
      key: 'coupons',
      icon: <TagsOutlined />,
      label: <Link to="/coupons">优惠券</Link>,
    },
    {
      key: 'seckill',
      icon: <ThunderboltOutlined />,
      label: <Link to="/seckill">秒杀专区</Link>,
    },
  ]

  const selectedKeys = [location.pathname.split('/')[1] || 'home']

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
              <Badge count={0} showZero={false}>
                <Button
                  type="text"
                  icon={<ShoppingCartOutlined />}
                  style={{ color: 'white' }}
                  onClick={() => navigate('/orders')}
                >
                  购物车
                </Button>
              </Badge>

              <Dropdown
                menu={{ items: userMenuItems }}
                placement="bottomRight"
                trigger={['click']}
              >
                <div className="user-info">
                  <Avatar icon={<UserOutlined />} src={user?.avatar} />
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
