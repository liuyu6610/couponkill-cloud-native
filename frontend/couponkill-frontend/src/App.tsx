import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom'
import { Layout, Spin } from 'antd'
import { lazy, Suspense } from 'react'
import Header from './components/Header'
import Footer from './components/Footer'
import ProtectedRoute from './components/ProtectedRoute'
import {
  loadLogin,
  loadRegister,
  loadHome,
  loadCouponList,
  loadCouponDetail,
  loadSeckill,
  loadOrderList,
  loadOrderDetail,
  loadUserCenter,
  loadConnectorAdmin,
  loadCouponAdmin,
  loadMyReservations,
} from './lib/routePreload'
import './App.css'

// 路由级懒加载，配合 vite manualChunks 拆分首屏包体
const Login = lazy(loadLogin)
const Register = lazy(loadRegister)
const Home = lazy(loadHome)
const CouponList = lazy(loadCouponList)
const CouponDetail = lazy(loadCouponDetail)
const Seckill = lazy(loadSeckill)
const OrderList = lazy(loadOrderList)
const OrderDetail = lazy(loadOrderDetail)
const UserCenter = lazy(loadUserCenter)
const ConnectorAdmin = lazy(loadConnectorAdmin)
const CouponAdmin = lazy(loadCouponAdmin)
const MyReservations = lazy(loadMyReservations)

const { Content } = Layout

function PageFallback() {
  return (
    <div className="loading-container">
      <Spin size="large" />
    </div>
  )
}

/**
 * 应用根组件：路由 + 布局。
 * 注意：auth.isLoading 只表示登录/注册请求进行中，不能在这里拿它整体替换路由树，
 * 否则提交登录/注册时整个 Router 被卸载，表单内容与错误提示丢失，注册成功后的 navigate 也会落空。
 */
function App() {
  return (
    <Router>
      <Layout className="app-layout">
        <Header />
        <Content className="app-content">
          <Suspense fallback={<PageFallback />}>
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route path="/register" element={<Register />} />
              <Route path="/" element={<Home />} />
              <Route path="/coupons" element={<CouponList />} />
              <Route path="/coupons/:id" element={<CouponDetail />} />
              <Route path="/seckill" element={<Seckill />} />
              <Route
                path="/reservations"
                element={
                  <ProtectedRoute>
                    <MyReservations />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/orders"
                element={
                  <ProtectedRoute>
                    <OrderList />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/orders/:id"
                element={
                  <ProtectedRoute>
                    <OrderDetail />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/user/*"
                element={
                  <ProtectedRoute>
                    <UserCenter />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/admin/connector"
                element={
                  <ProtectedRoute requireAdmin>
                    <ConnectorAdmin />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/admin/coupons"
                element={
                  <ProtectedRoute requireAdmin>
                    <CouponAdmin />
                  </ProtectedRoute>
                }
              />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </Suspense>
        </Content>
        <Footer />
      </Layout>
    </Router>
  )
}

export default App
