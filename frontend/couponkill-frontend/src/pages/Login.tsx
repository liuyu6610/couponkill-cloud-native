import React from 'react'
import { Form, Input, Button, Card, Typography, Divider, Space, App } from 'antd'
import { UserOutlined, LockOutlined, GoogleOutlined, GithubOutlined } from '@ant-design/icons'
import { Link, useNavigate, useLocation } from 'react-router-dom'
import { useDispatch, useSelector } from 'react-redux'
import { loginUser, clearError } from '../store/slices/authSlice'
import type { RootState, AppDispatch } from '../store'

const { Title, Text } = Typography

interface LoginFormData {
  username: string
  password: string
  remember?: boolean
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
    // 清除之前的错误信息
    dispatch(clearError())
  }, [dispatch])

  React.useEffect(() => {
    // 已登录则重定向到之前的页面
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
      message.error((err as string) || '登录失败，请检查用户名和密码')
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

            <Form
              name="login"
              className="login-form"
              initialValues={{ remember: true }}
              onFinish={onFinish}
              size="large"
              layout="vertical"
            >
              <Form.Item
                name="username"
                rules={[
                  { required: true, message: '请输入用户名!' },
                  { min: 3, message: '用户名至少3个字符!' }
                ]}
              >
                <Input
                  prefix={<UserOutlined />}
                  placeholder="用户名或邮箱"
                  autoComplete="username"
                />
              </Form.Item>

              <Form.Item
                name="password"
                rules={[
                  { required: true, message: '请输入密码!' },
                  { min: 6, message: '密码至少6个字符!' }
                ]}
              >
                <Input.Password
                  prefix={<LockOutlined />}
                  placeholder="密码"
                  autoComplete="current-password"
                />
              </Form.Item>

              <Form.Item>
                <div className="form-actions">
                  <Form.Item name="remember" valuePropName="checked" noStyle>
                    <label>记住我</label>
                  </Form.Item>
                  <Link to="/forgot-password" className="forgot-password">
                    忘记密码？
                  </Link>
                </div>
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

            <Divider>其他登录方式</Divider>

            <div className="social-login">
              <Space size="middle">
                <Button icon={<GoogleOutlined />} size="large">
                  Google
                </Button>
                <Button icon={<GithubOutlined />} size="large">
                  GitHub
                </Button>
              </Space>
            </div>

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
