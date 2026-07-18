import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const proxyTarget = env.VITE_PROXY_TARGET || 'http://localhost:8088'

  // Phase1：order 已迁 /api/v1/order（走 /api 代理）；保留 /order 兼容旧调用
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
      },
    },
    build: {
      // 生产关闭 sourcemap，缩小制品并避免源码映射泄露
      sourcemap: false,
      // 构建结束输出 gzip 体积报告（Vite 默认开启，显式声明便于审计）
      reportCompressedSize: true,
      chunkSizeWarningLimit: 700,
      rollupOptions: {
        output: {
          manualChunks(id) {
            if (!id.includes('node_modules')) return
            // icons 与 antd 主体拆开，便于并行加载、缩小首屏 antd 主包感知体积
            if (id.includes('@ant-design/icons')) return 'vendor-antd-icons'
            if (id.includes('antd') || id.includes('@ant-design')) return 'vendor-antd'
            if (
              id.includes('react-dom') ||
              id.includes('react-router') ||
              id.includes('react-redux') ||
              id.includes('@reduxjs') ||
              /[/\\]react[/\\]/.test(id)
            ) {
              return 'vendor-react'
            }
            if (id.includes('@tanstack')) return 'vendor-query'
          },
        },
      },
    },
  }
})
