import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom'
import { Layout, Spin } from 'antd'
import { useSelector } from 'react-redux'
import { lazy, Suspense } from 'react'
import type { RootState } from './store'
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
const MyReservations = lazy(loadMyReservations)

const { Content } = Layout

function PageFallback() {
  return (
    <div className="loading-container">
      <Spin size="large" />
    </div>
  )
}

function App() {
  const { isLoading } = useSelector((state: RootState) => state.auth)

  if (isLoading) {
    return <PageFallback />
  }

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
