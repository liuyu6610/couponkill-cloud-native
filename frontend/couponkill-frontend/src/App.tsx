import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom'
import { Layout, Spin } from 'antd'
import { useSelector } from 'react-redux'
import type { RootState } from './store'
import Header from './components/Header'
import Footer from './components/Footer'
import Login from './pages/Login'
import Register from './pages/Register'
import Home from './pages/Home'
import CouponList from './pages/CouponList'
import CouponDetail from './pages/CouponDetail'
import Seckill from './pages/Seckill'
import OrderList from './pages/OrderList'
import OrderDetail from './pages/OrderDetail'
import UserCenter from './pages/UserCenter'
import ProtectedRoute from './components/ProtectedRoute'
import './App.css'

const { Content } = Layout

function App() {
  const { isLoading } = useSelector((state: RootState) => state.auth)

  if (isLoading) {
    return (
      <div className="loading-container">
        <Spin size="large" />
      </div>
    )
  }

  return (
    <Router>
      <Layout className="app-layout">
        <Header />
        <Content className="app-content">
          <Routes>
            {/* 公开路由 */}
            <Route path="/login" element={<Login />} />
            <Route path="/register" element={<Register />} />

            {/* 首页 */}
            <Route path="/" element={<Home />} />

            {/* 优惠券相关 */}
            <Route path="/coupons" element={<CouponList />} />
            <Route path="/coupons/:id" element={<CouponDetail />} />

            {/* 秒杀专区 */}
            <Route path="/seckill" element={<Seckill />} />

            {/* 订单相关 */}
            <Route path="/orders" element={
              <ProtectedRoute>
                <OrderList />
              </ProtectedRoute>
            } />
            <Route path="/orders/:id" element={
              <ProtectedRoute>
                <OrderDetail />
              </ProtectedRoute>
            } />

            {/* 用户中心 */}
            <Route path="/user/*" element={
              <ProtectedRoute>
                <UserCenter />
              </ProtectedRoute>
            } />

            {/* 默认重定向 */}
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Content>
        <Footer />
      </Layout>
    </Router>
  )
}

export default App
