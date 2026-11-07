import React from 'react'
import { Form, Input, Button, Card, Typography, App, Alert } from 'antd'
import { UserOutlined, LockOutlined } from '@ant-design/icons'
import { Link, useNavigate, useLocation } from 'react-router-dom'
import { useDispatch, useSelector } from 'react-redux'
import { loginUser, clearError } from '../store/slices/authSlice'
import type { RootState, AppDispatch } from '../store'
import { SESSION_EXPIRED_FLAG } from '../lib/apiClient'

const { Title, Text } = Typography

/**
 * 是否展示并预填演示账号（demo / demo1234）。
 * 仅 `npm run dev` 默认开启；生产构建需显式设置 VITE_SHOW_DEMO_ACCOUNT=true（如公开演示站），
 * 避免对外部署时把种子账号口令直接写在登录页上。
 */
const SHOW_DEMO_ACCOUNT =
  import.meta.env.DEV || import.meta.env.VITE_SHOW_DEMO_ACCOUNT === 'true'

const DEMO_INITIAL_VALUES = { username: 'demo', password: 'demo1234' }

interface LoginFormData {
  username: string
  password: string
}

const Login: React.FC = () => {
  const navigate = useNavigate()
  const location = useLocation()
  const dispatch = useDispatch<AppDispatch>()
  const { message } = App.useApp()
  const { isLoading, error, isAuthenticated } = useSelector((state: RootState) => state.auth)

  // 从state中获取重定向路径
  const from = (location.state as { from?: { pathname?: string } } | null)?.from?.pathname || '/'

  // 因会话过期（401）被 apiClient 踢回登录页时给一次性提示；标记在 effect 里清除（StrictMode 下初始化器会跑两次，不能在其中清）
  const [sessionExpired] = React.useState(() => {
    try {
      return sessionStorage.getItem(SESSION_EXPIRED_FLAG) === '1'
    } catch {
      return false
    }
  })
  React.useEffect(() => {
    if (!sessionExpired) return
    try {
      sessionStorage.removeItem(SESSION_EXPIRED_FLAG)
    } catch {
      // sessionStorage 不可用时忽略，最多下次再提示一次
    }
  }, [sessionExpired])

  React.useEffect(() => {
    dispatch(clearError())
  }, [dispatch])

  React.useEffect(() => {
    if (isAuthenticated) {
      navigate(from, { replace: true })
    }
  }, [navigate, from, isAuthenticated])

  const onFinish = async (values: LoginFormData) => {
    try {
      await dispatch(loginUser({ username: values.username, password: values.password })).unwrap()
      message.success('登录成功！')
      navigate(from, { replace: true })
    } catch (err) {
      // thunk 以 rejectWithValue 抛出的是已解析好的服务端文案（字符串）
      message.error(typeof err === 'string' && err ? err : '登录失败，请检查用户名和密码')
    }
  }

  return (
    <div className="login-container">
      <div className="login-background">
        <div className="login-form-wrapper">
          <Card className="login-card">
            <div className="login-header">
              <Title level={2}>欢迎登录</Title>
              <Text type="secondary">请输入您的账户信息</Text>
            </div>

            {sessionExpired && (
              <Alert
                type="warning"
                showIcon
                style={{ marginBottom: 16 }}
                message="登录已过期，请重新登录"
              />
            )}

            {SHOW_DEMO_ACCOUNT && (
              <Alert
                type="info"
                showIcon
                style={{ marginBottom: 16 }}
                message="演示账号"
                description="用户名 demo，密码 demo1234"
              />
            )}

            <Form
              name="login"
              className="login-form"
              initialValues={SHOW_DEMO_ACCOUNT ? DEMO_INITIAL_VALUES : undefined}
              onFinish={onFinish}
              size="large"
              layout="vertical"
            >
              <Form.Item
                name="username"
                rules={[
                  { required: true, message: '请输入用户名!' },
                  { min: 3, message: '用户名至少3个字符!' },
                ]}
              >
                <Input
                  prefix={<UserOutlined />}
                  placeholder="用户名"
                  autoComplete="username"
                />
              </Form.Item>

              <Form.Item
                name="password"
                rules={[
                  { required: true, message: '请输入密码!' },
                  { min: 6, message: '密码至少6个字符!' },
                ]}
              >
                <Input.Password
                  prefix={<LockOutlined />}
                  placeholder="密码"
                  autoComplete="current-password"
                />
              </Form.Item>

              {error && (
                <div className="error-message">
                  <Text type="danger">{error}</Text>
                </div>
              )}

              <Form.Item>
                <Button
                  type="primary"
                  htmlType="submit"
                  className="login-button"
                  loading={isLoading}
                  block
                >
                  登录
                </Button>
              </Form.Item>
            </Form>

            <div className="login-footer">
              <Text type="secondary">
                还没有账户？ <Link to="/register">立即注册</Link>
              </Text>
            </div>
          </Card>
        </div>
      </div>
    </div>
  )
}

export default Login
