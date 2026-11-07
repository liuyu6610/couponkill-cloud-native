import { loadEnv } from 'vite'
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const proxyTarget = env.VITE_PROXY_TARGET || 'http://localhost:8088'

  // 前端只访问网关的 /api/v1/**（同源相对路径）。开发时由 Vite 把 /api 代理到网关（默认 :8088），
  // 网关再按路由表转发到 user/coupon/order/connector；/internal/** 网关一律 404，前端也不会请求。
  const proxyRule = {
    target: proxyTarget,
    changeOrigin: true,
  }

  return {
    plugins: [react()],
    // 单元 / 组件测试：jsdom + Testing Library；全局初始化见 src/test/setup.ts
    test: {
      environment: 'jsdom',
      setupFiles: ['./src/test/setup.ts'],
      include: ['src/**/*.test.{ts,tsx}'],
    },
    server: {
      proxy: {
        '/api': proxyRule,
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
            // 不再把 antd / icons 强行合成单个 vendor 包：那样 Table、Form、DatePicker(rc-picker)、
            // Descriptions 等只在管理台/详情页用到的组件也会进首屏（实测首屏 JS ≈1.4MB）。
            // 交给 Rollup 按路由懒加载边界自动拆分，首屏只带 Header/布局实际用到的 antd 模块。
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
