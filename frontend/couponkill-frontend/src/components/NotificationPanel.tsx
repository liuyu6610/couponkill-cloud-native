import React from 'react'
import { Button, List, Space, Typography } from 'antd'
import type { UserNotification } from '../types/api'

const { Text } = Typography

export interface NotificationPanelProps {
  /** 当前用户的站内通知（由 Header 的 react-query 拉取） */
  notices: UserNotification[]
  /** 点击「全部已读」 */
  onMarkAllRead: () => void
  /** 点击 / 回车 / 空格选中某条通知 */
  onClickNotice: (id: string) => void
}

/**
 * 站内通知下拉面板内容。
 *
 * 单独成文件并由 Header 通过 React.lazy 按需加载：List 会连带 Pagination → Select → rc-virtual-list
 * 以及 Typography 等约 200kB 的 antd 模块，它们只在用户点开铃铛时才需要，不应进入首屏入口包。
 * Header 在铃铛 hover/focus 时预取本 chunk，点开时通常已加载完成，交互与原先一致。
 */
const NotificationPanel: React.FC<NotificationPanelProps> = ({ notices, onMarkAllRead, onClickNotice }) => {
  return (
    <>
      <Space style={{ width: '100%', justifyContent: 'space-between', padding: '4px 8px' }}>
        <Text strong>站内通知</Text>
        <Button type="link" size="small" onClick={onMarkAllRead}>
          全部已读
        </Button>
      </Space>
      <List
        size="small"
        locale={{ emptyText: '暂无通知' }}
        dataSource={notices}
        renderItem={(item) => (
          <List.Item
            style={{
              cursor: 'pointer',
              background: item.readFlag ? undefined : '#f0f5ff',
              padding: '8px 12px',
            }}
            role="button"
            tabIndex={0}
            onClick={() => onClickNotice(item.id)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                onClickNotice(item.id)
              }
            }}
          >
            <List.Item.Meta
              title={item.title}
              description={
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {item.content || item.type}
                </Text>
              }
            />
          </List.Item>
        )}
      />
    </>
  )
}

export default NotificationPanel
