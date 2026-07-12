import React from 'react'
import { Form, Input, Button, Card, Typography, App } from 'antd'
import { UserOutlined, LockOutlined, PhoneOutlined } from '@ant-design/icons'
import { Link, useNavigate } from 'react-router-dom'
import { useDispatch, useSelector } from 'react-redux'
import { registerUser, clearError } from '../store/slices/authSlice'
import type { RootState, AppDispatch } from '../store'

const { Title, Text } = Typography

interface RegisterFormData {
  username: string
  phone: string
  password: string
  confirmPassword: string
}

const Register: React.FC = () => {
  const navigate = useNavigate()
  const dispatch = useDispatch<AppDispatch>()
  const { message } = App.useApp()
  const { isLoading, error } = useSelector((state: RootState) => state.auth)

  React.useEffect(() => {
    dispatch(clearError())
  }, [dispatch])

  const onFinish = async (values: RegisterFormData) => {
    try {
      await dispatch(
        registerUser({
          username: values.username,
          password: values.password,
          phone: values.phone,
        })
      ).unwrap()
      message.success('注册成功！请登录')
      navigate('/login')
    } catch (err) {
      message.error((err as string) || '注册失败，请稍后重试')
    }
  }

  const validateConfirm = ({ getFieldValue }: { getFieldValue: (name: string) => string }) => ({
    validator(_: unknown, value: string) {
      if (!value || getFieldValue('password') === value) {
        return Promise.resolve()
      }
      return Promise.reject(new Error('两次输入的密码不一致!'))
    },
  })

  return (
    <div className="register-container">
      <div className="register-background">
        <div className="register-form-wrapper">
          <Card className="register-card">
            <div className="register-header">
              <Title level={2}>用户注册</Title>
              <Text type="secondary">创建您的账户，开始享受优惠</Text>
            </div>

            <Form
              name="register"
              className="register-form"
              onFinish={onFinish}
              size="large"
              layout="vertical"
              scrollToFirstError
            >
              <Form.Item
                name="username"
                rules={[
                  { required: true, message: '请输入用户名!' },
                  { min: 3, max: 20, message: '用户名长度应在3-20个字符之间!' },
                  { pattern: /^[a-zA-Z0-9_]+$/, message: '用户名只能包含字母、数字和下划线!' },
                ]}
              >
                <Input prefix={<UserOutlined />} placeholder="用户名" autoComplete="username" />
              </Form.Item>

              <Form.Item
                name="phone"
                rules={[
                  { required: true, message: '请输入手机号码!' },
                  { pattern: /^1[3-9]\d{9}$/, message: '请输入有效的手机号码!' },
                ]}
              >
                <Input prefix={<PhoneOutlined />} placeholder="手机号码" autoComplete="tel" />
              </Form.Item>

              <Form.Item
                name="password"
                rules={[
                  { required: true, message: '请输入密码!' },
                  { min: 6, message: '密码至少6个字符!' },
                ]}
                hasFeedback
              >
                <Input.Password prefix={<LockOutlined />} placeholder="密码" autoComplete="new-password" />
              </Form.Item>

              <Form.Item
                name="confirmPassword"
                dependencies={['password']}
                rules={[{ required: true, message: '请确认密码!' }, validateConfirm]}
                hasFeedback
              >
                <Input.Password prefix={<LockOutlined />} placeholder="确认密码" autoComplete="new-password" />
              </Form.Item>

              {error && (
                <div className="error-message">
                  <Text type="danger">{error}</Text>
                </div>
              )}

              <Form.Item>
                <Button type="primary" htmlType="submit" className="register-button" loading={isLoading} block>
                  注册
                </Button>
              </Form.Item>
            </Form>

            <div className="register-footer">
              <Text type="secondary">
                已有账户？ <Link to="/login">立即登录</Link>
              </Text>
            </div>
          </Card>
        </div>
      </div>
    </div>
  )
}

export default Register
