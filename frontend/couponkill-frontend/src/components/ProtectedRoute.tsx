import React from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { useSelector } from 'react-redux'
import { Spin } from 'antd'
import type { RootState } from '../store'

interface ProtectedRouteProps {
  children: React.ReactNode
}

const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ children }) => {
  const location = useLocation()
  const { isAuthenticated, isLoading } = useSelector((state: RootState) => state.auth)

  // 如果还在加载中，显示加载状态
  if (isLoading) {
    return (
      <div className="loading-container">
        <Spin size="large" />
      </div>
    )
  }

  // 如果未认证，重定向到登录页
  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  // 已认证，返回子组件
  return <>{children}</>
}

export default ProtectedRoute
