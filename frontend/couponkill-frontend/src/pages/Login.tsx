import React from 'react'
import { Form, Input, Button, Card, Typography, App, Alert } from 'antd'
import { UserOutlined, LockOutlined } from '@ant-design/icons'
import { Link, useNavigate, useLocation } from 'react-router-dom'
import { useDispatch, useSelector } from 'react-redux'
import { loginUser, clearError } from '../store/slices/authSlice'
import type { RootState, AppDispatch } from '../store'
import { getErrorMessage } from '../lib/errorMessage'

const { Title, Text } = Typography

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
      message.error(getErrorMessage(err, '登录失败，请检查用户名和密码'))
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

            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 16 }}
              message="演示账号"
              description="用户名 demo，密码 demo1234"
            />

            <Form
              name="login"
              className="login-form"
              initialValues={{ username: 'demo', password: 'demo1234' }}
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
