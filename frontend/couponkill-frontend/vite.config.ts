import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const proxyTarget = env.VITE_PROXY_TARGET || 'http://localhost:80'

  // 后端路径前缀：user/coupon 走 /api/v1，order/seckill 走各自根路径
  const proxyRule = {
    target: proxyTarget,
    changeOrigin: true,
  }

  return {
    plugins: [react()],
    server: {
      proxy: {
        '/api': proxyRule,
        '/order': proxyRule,
        '/seckill': proxyRule,
      },
    },
  }
})
