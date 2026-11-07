import React from 'react'
import { Layout, Divider } from 'antd'
import { Link } from 'react-router-dom'

const { Footer: AntFooter } = Layout

const Footer: React.FC = () => {
  return (
    <AntFooter className="app-footer">
      <div className="footer-container">
        <div className="footer-content">
          <div className="footer-section">
            <h3>关于我们</h3>
            <ul>
              <li><Link to="/about">公司介绍</Link></li>
              <li><Link to="/contact">联系我们</Link></li>
              <li><Link to="/careers">招贤纳士</Link></li>
              <li><Link to="/news">新闻资讯</Link></li>
            </ul>
          </div>

          <div className="footer-section">
            <h3>帮助中心</h3>
            <ul>
              <li><Link to="/faq">常见问题</Link></li>
              <li><Link to="/guide">使用指南</Link></li>
              <li><Link to="/feedback">意见反馈</Link></li>
              <li><Link to="/support">在线客服</Link></li>
            </ul>
          </div>

          <div className="footer-section">
            <h3>法律条款</h3>
            <ul>
              <li><Link to="/terms">服务条款</Link></li>
              <li><Link to="/privacy">隐私政策</Link></li>
              <li><Link to="/cookies">Cookie政策</Link></li>
              <li><Link to="/disclaimer">免责声明</Link></li>
            </ul>
          </div>

          <div className="footer-section">
            <h3>关注我们</h3>
            <div className="social-links">
              <a href="#" className="social-link">微信</a>
              <a href="#" className="social-link">微博</a>
              <a href="#" className="social-link">抖音</a>
              <a href="#" className="social-link">小红书</a>
            </div>
          </div>
        </div>

        <Divider />

        <div className="footer-bottom">
          <div className="footer-info">
            <p>© 2024 CouponKill 云原生秒杀系统. 保留所有权利.</p>
            <p>
              <span>京ICP备12345678号</span>
              <span className="separator">|</span>
              <span>京公网安备11000002000001号</span>
            </p>
          </div>
        </div>
      </div>
    </AntFooter>
  )
}

export default Footer
