import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { ConfigProvider, App as AntApp } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { QueryClientProvider } from '@tanstack/react-query'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import './index.css'
import App from './App.tsx'
import { store } from './store/index.ts'
import { queryClient } from './lib/queryClient.ts'
import { theme } from './theme/index.ts'

// 配置dayjs中文语言包
dayjs.locale('zh-cn')

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ConfigProvider locale={zhCN} theme={theme}>
      <AntApp>
        <QueryClientProvider client={queryClient}>
          <Provider store={store}>
            <App />
          </Provider>
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>
  </StrictMode>,
)
