import type { ThemeConfig } from 'antd'

// CouponKill 设计系统：以“秒杀橙红”为品牌主色，统一圆角与控件密度。
// 通过 antd v5 的设计 token 集中管理，避免页面内散落硬编码颜色。
export const BRAND = {
  primary: '#fa541c', // 主色（秒杀橙红）
  seckill: '#f5222d', // 秒杀强调色
  success: '#52c41a',
  warning: '#faad14',
  error: '#ff4d4f',
} as const

export const theme: ThemeConfig = {
  token: {
    colorPrimary: BRAND.primary,
    colorInfo: BRAND.primary,
    colorSuccess: BRAND.success,
    colorWarning: BRAND.warning,
    colorError: BRAND.error,
    borderRadius: 8,
    fontSize: 14,
    wireframe: false,
  },
  components: {
    Button: {
      controlHeight: 38,
      fontWeight: 500,
    },
    Card: {
      borderRadiusLG: 12,
    },
    Layout: {
      headerBg: '#1f1f1f',
      headerHeight: 64,
    },
    Menu: {
      darkItemBg: '#1f1f1f',
      darkItemSelectedBg: BRAND.primary,
    },
  },
}

export default theme
