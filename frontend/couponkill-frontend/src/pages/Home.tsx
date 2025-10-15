import React from 'react'
import { Card, Col, Row, Typography, Button } from 'antd'
import { useNavigate } from 'react-router-dom'
import { useSelector } from 'react-redux'
import type { RootState } from '../store'

const { Title, Paragraph } = Typography

const Home: React.FC = () => {
  const navigate = useNavigate()
  const { isAuthenticated } = useSelector((state: RootState) => state.auth)

  const handleStart = () => {
    if (isAuthenticated) {
      navigate('/coupons')
    } else {
      navigate('/login')
    }
  }

  return (
    <div className="home-page">
      <div className="hero-section">
        <Title level={2}>欢迎来到云秒杀平台</Title>
        <Paragraph>体验极致的秒杀购物乐趣，海量优惠券等你来抢！</Paragraph>
        <Button type="primary" size="large" onClick={handleStart}>
          {isAuthenticated ? '开始秒杀' : '立即登录'}
        </Button>
      </div>

      <Row gutter={[16, 16]} className="features-section">
        <Col xs={24} sm={12} lg={8}>
          <Card title="海量优惠券" bordered={false}>
            <Paragraph>提供各种品类的优惠券，满足你的不同需求</Paragraph>
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={8}>
          <Card title="极速秒杀" bordered={false}>
            <Paragraph>高并发支持，确保秒杀过程流畅无阻</Paragraph>
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={8}>
          <Card title="安全保障" bordered={false}>
            <Paragraph>多重安全机制，保护用户信息和交易安全</Paragraph>
          </Card>
        </Col>
      </Row>

      <div className="cta-section">
        <Title level={3}>准备好开始了吗？</Title>
        <Button type="primary" size="large" onClick={handleStart}>
          {isAuthenticated ? '查看所有优惠券' : '加入我们'}
        </Button>
      </div>
    </div>
  )
}

export default Home