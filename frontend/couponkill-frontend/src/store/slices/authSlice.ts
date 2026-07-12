import { createSlice, createAsyncThunk } from '@reduxjs/toolkit'
import type { PayloadAction } from '@reduxjs/toolkit'
import { authService } from '../../services/authService'
import { STORAGE_KEYS } from '../../lib/apiClient'
import type { LoginResult } from '../../types/api'

export interface AuthUser {
  id: string
  username: string
  roles: string[]
}

export interface AuthState {
  user: AuthUser | null
  token: string | null
  isAuthenticated: boolean
  isLoading: boolean
  error: string | null
}

function parseRoles(raw: string | null): string[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.map(String) : []
  } catch {
    return raw.split(',').map((s) => s.trim()).filter(Boolean)
  }
}

function restoreUser(): AuthUser | null {
  const id = localStorage.getItem(STORAGE_KEYS.userId)
  const username = localStorage.getItem(STORAGE_KEYS.username)
  const roles = parseRoles(localStorage.getItem(STORAGE_KEYS.roles))
  return id && username ? { id, username, roles } : null
}

const initialState: AuthState = {
  user: restoreUser(),
  token: localStorage.getItem(STORAGE_KEYS.token),
  isAuthenticated: !!localStorage.getItem(STORAGE_KEYS.token),
  isLoading: false,
  error: null,
}

export const loginUser = createAsyncThunk(
  'auth/loginUser',
  async (
    { username, password }: { username: string; password: string },
    { rejectWithValue }
  ) => {
    try {
      const result = await authService.login(username, password)
      const userId = String(result.userId)
      const roles = (result.roles || []).map(String)
      localStorage.setItem(STORAGE_KEYS.token, result.token)
      localStorage.setItem(STORAGE_KEYS.userId, userId)
      localStorage.setItem(STORAGE_KEYS.username, result.username)
      localStorage.setItem(STORAGE_KEYS.roles, JSON.stringify(roles))
      return { ...result, userId, roles } as LoginResult
    } catch (error) {
      return rejectWithValue((error as Error).message || '登录失败')
    }
  }
)

export const registerUser = createAsyncThunk(
  'auth/registerUser',
  async (
    { username, password, phone }: { username: string; password: string; phone: string },
    { rejectWithValue }
  ) => {
    try {
      await authService.register(username, password, phone)
      return true
    } catch (error) {
      return rejectWithValue((error as Error).message || '注册失败')
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
    logout: (state) => {
      authService.logout()
      localStorage.removeItem(STORAGE_KEYS.roles)
      state.user = null
      state.token = null
      state.isAuthenticated = false
      state.error = null
    },
  },
  extraReducers: (builder) => {
    builder
      .addCase(loginUser.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(loginUser.fulfilled, (state, action: PayloadAction<LoginResult>) => {
        state.isLoading = false
        state.isAuthenticated = true
        state.token = action.payload.token
        state.user = {
          id: String(action.payload.userId),
          username: action.payload.username,
          roles: (action.payload.roles || []).map(String),
        }
        state.error = null
      })
      .addCase(loginUser.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
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
  },
})

export const { clearError, logout } = authSlice.actions

export const selectIsAdmin = (state: { auth: AuthState }) =>
  !!state.auth.user?.roles?.some((r) => r.toLowerCase() === 'admin')

export default authSlice.reducer
