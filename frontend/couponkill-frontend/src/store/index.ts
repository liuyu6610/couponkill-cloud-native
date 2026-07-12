import { configureStore } from '@reduxjs/toolkit'
import authSlice from './slices/authSlice'

// 仅保留会话/本地状态（auth）；服务端状态统一由 TanStack Query 管理
export const store = configureStore({
  reducer: {
    auth: authSlice,
  },
})

export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch
