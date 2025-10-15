import { createSlice, createAsyncThunk } from '@reduxjs/toolkit'
import type { PayloadAction } from '@reduxjs/toolkit'
import { getUserProfile, updateUserProfile, getUserCoupons, getUserStats } from '../../services/userService'

export interface UserProfile {
  id: string
  username: string
  email: string
  phone?: string
  avatar?: string
  gender?: 'MALE' | 'FEMALE' | 'OTHER'
  birthday?: string
  address?: {
    province: string
    city: string
    district: string
    address: string
    postalCode?: string
  }
  preferences?: {
    categories: string[]
    notifications: boolean
    emailMarketing: boolean
  }
  createdAt: string
  updatedAt: string
}

export interface UserCoupon {
  id: string
  couponId: string
  couponName: string
  couponType: string
  value: number
  status: 'AVAILABLE' | 'USED' | 'EXPIRED'
  obtainedAt: string
  usedAt?: string
  expiresAt: string
  orderId?: string
}

export interface UserStats {
  totalCoupons: number
  usedCoupons: number
  availableCoupons: number
  totalSavings: number
  thisMonthSavings: number
  favoriteCategories: string[]
}

export interface UserState {
  profile: UserProfile | null
  coupons: UserCoupon[]
  stats: UserStats | null
  isLoading: boolean
  error: string | null
}

const initialState: UserState = {
  profile: null,
  coupons: [],
  stats: null,
  isLoading: false,
  error: null,
}

// 获取用户资料
export const fetchUserProfile = createAsyncThunk(
  'user/fetchUserProfile',
  async (_, { rejectWithValue }) => {
    try {
      const response = await getUserProfile()
      return response
    } catch (error: any) {
      return rejectWithValue(error.message || '获取用户资料失败')
    }
  }
)

// 更新用户资料
export const updateUserProfileAsync = createAsyncThunk(
  'user/updateUserProfile',
  async (profileData: Partial<UserProfile>, { rejectWithValue }) => {
    try {
      const response = await updateUserProfile(profileData)
      return response
    } catch (error: any) {
      return rejectWithValue(error.message || '更新用户资料失败')
    }
  }
)

// 获取用户优惠券
export const fetchUserCoupons = createAsyncThunk(
  'user/fetchUserCoupons',
  async (params: { status?: string; page?: number; size?: number } = {}, { rejectWithValue }) => {
    try {
      const response = await getUserCoupons(params)
      return response
    } catch (error: any) {
      return rejectWithValue(error.message || '获取用户优惠券失败')
    }
  }
)

// 获取用户统计信息
export const fetchUserStats = createAsyncThunk(
  'user/fetchUserStats',
  async (_, { rejectWithValue }) => {
    try {
      const response = await getUserStats()
      return response
    } catch (error: any) {
      return rejectWithValue(error.message || '获取用户统计信息失败')
    }
  }
)

const userSlice = createSlice({
  name: 'user',
  initialState,
  reducers: {
    clearError: (state) => {
      state.error = null
    },
    updateProfileLocally: (state, action: PayloadAction<Partial<UserProfile>>) => {
      if (state.profile) {
        state.profile = { ...state.profile, ...action.payload }
      }
    },
    clearUserData: (state) => {
      state.profile = null
      state.coupons = []
      state.stats = null
      state.error = null
    },
  },
  extraReducers: (builder) => {
    builder
      // 获取用户资料
      .addCase(fetchUserProfile.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(fetchUserProfile.fulfilled, (state, action) => {
        state.isLoading = false
        state.profile = action.payload
        state.error = null
      })
      .addCase(fetchUserProfile.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
      // 更新用户资料
      .addCase(updateUserProfileAsync.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(updateUserProfileAsync.fulfilled, (state, action) => {
        state.isLoading = false
        state.profile = action.payload
        state.error = null
      })
      .addCase(updateUserProfileAsync.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
      // 获取用户优惠券
      .addCase(fetchUserCoupons.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(fetchUserCoupons.fulfilled, (state, action) => {
        state.isLoading = false
        state.coupons = action.payload.data || []
        state.error = null
      })
      .addCase(fetchUserCoupons.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
      // 获取用户统计信息
      .addCase(fetchUserStats.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(fetchUserStats.fulfilled, (state, action) => {
        state.isLoading = false
        state.stats = action.payload
        state.error = null
      })
      .addCase(fetchUserStats.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
  },
})

export const { clearError, updateProfileLocally, clearUserData } = userSlice.actions
export default userSlice.reducer
