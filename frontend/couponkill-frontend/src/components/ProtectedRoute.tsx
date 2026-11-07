import React from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { useSelector } from 'react-redux'
import { Result, Spin } from 'antd'
import type { RootState } from '../store'
import { selectIsAdmin } from '../store/slices/authSlice'

interface ProtectedRouteProps {
  children: React.ReactNode
  /** 需要 admin 角色（管理台页面） */
  requireAdmin?: boolean
}

const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ children, requireAdmin = false }) => {
  const location = useLocation()
  const { isAuthenticated, isLoading } = useSelector((state: RootState) => state.auth)
  const isAdmin = useSelector(selectIsAdmin)

  if (isLoading) {
    return (
      <div className="loading-container">
        <Spin size="large" />
      </div>
    )
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  if (requireAdmin && !isAdmin) {
    return (
      <Result
        status="403"
        title="需要管理员权限"
        subTitle="管理台仅对 admin 角色开放。本地演示管理员账号见后端种子数据（CK_SEED_DEMO=true）。"
      />
    )
  }

  return <>{children}</>
}

export default ProtectedRoute
