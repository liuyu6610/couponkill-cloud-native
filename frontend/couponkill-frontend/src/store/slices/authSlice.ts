import { createSlice, createAsyncThunk } from '@reduxjs/toolkit'
import type { PayloadAction } from '@reduxjs/toolkit'
import { login as loginApi, register as registerApi, logout as logoutApi } from '../../services/authService'

export interface User {
  id: string
  username: string
  email: string
  phone?: string
  avatar?: string
  createdAt: string
}

export interface AuthState {
  user: User | null
  token: string | null
  isAuthenticated: boolean
  isLoading: boolean
  error: string | null
}

const initialState: AuthState = {
  user: null,
  token: localStorage.getItem('token'),
  isAuthenticated: !!localStorage.getItem('token'),
  isLoading: false,
  error: null,
}

// 异步登录action
export const loginUser = createAsyncThunk(
  'auth/loginUser',
  async ({ username, password }: { username: string; password: string }, { rejectWithValue }) => {
    try {
      const response = await loginApi(username, password)
      localStorage.setItem('token', response.token)
      return response
    } catch (error: any) {
      return rejectWithValue(error.message || '登录失败')
    }
  }
)

// 异步注册action
export const registerUser = createAsyncThunk(
  'auth/registerUser',
  async (userData: { username: string; password: string; email: string; phone?: string }, { rejectWithValue }) => {
    try {
      const response = await registerApi(userData)
      return response
    } catch (error: any) {
      return rejectWithValue(error.message || '注册失败')
    }
  }
)

// 异步登出action
export const logoutUser = createAsyncThunk(
  'auth/logoutUser',
  async (_, { rejectWithValue }) => {
    try {
      await logoutApi()
      localStorage.removeItem('token')
      return null
    } catch (error: any) {
      localStorage.removeItem('token')
      return rejectWithValue(error.message || '登出失败')
    }
  }
)

const authSlice = createSlice({
  name: 'auth',
  initialState,
  reducers: {
    clearError: (state) => {
      state.error = null
    },
    setUser: (state, action: PayloadAction<User>) => {
      state.user = action.payload
      state.isAuthenticated = true
    },
    clearAuth: (state) => {
      state.user = null
      state.token = null
      state.isAuthenticated = false
      state.error = null
    },
  },
  extraReducers: (builder) => {
    builder
      // 登录
      .addCase(loginUser.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(loginUser.fulfilled, (state, action) => {
        state.isLoading = false
        state.isAuthenticated = true
        state.user = action.payload.user
        state.token = action.payload.token
        state.error = null
      })
      .addCase(loginUser.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
      // 注册
      .addCase(registerUser.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(registerUser.fulfilled, (state) => {
        state.isLoading = false
        state.error = null
      })
      .addCase(registerUser.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
      // 登出
      .addCase(logoutUser.pending, (state) => {
        state.isLoading = true
      })
      .addCase(logoutUser.fulfilled, (state) => {
        state.isLoading = false
        state.user = null
        state.token = null
        state.isAuthenticated = false
        state.error = null
      })
      .addCase(logoutUser.rejected, (state) => {
        state.isLoading = false
        state.user = null
        state.token = null
        state.isAuthenticated = false
        state.error = null
      })
  },
})

export const { clearError, setUser, clearAuth } = authSlice.actions
export default authSlice.reducer
