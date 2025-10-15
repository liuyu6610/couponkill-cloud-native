import { createSlice, createAsyncThunk } from '@reduxjs/toolkit'
import type { PayloadAction } from '@reduxjs/toolkit'
import { getCoupons, getCouponDetail, participateSeckill } from '../../services/couponService'

export interface Coupon {
  id: string
  name: string
  description: string
  type: 'DISCOUNT' | 'CASH' | 'PERCENTAGE'
  value: number
  minAmount?: number
  maxDiscount?: number
  totalStock: number
  availableStock: number
  startTime: string
  endTime: string
  status: 'ACTIVE' | 'INACTIVE' | 'EXPIRED'
  seckillStartTime?: string
  seckillEndTime?: string
  imageUrl?: string
  tags: string[]
  createdAt: string
  updatedAt: string
}

export interface SeckillCoupon extends Coupon {
  seckillPrice: number
  seckillStock: number
  seckillParticipants: number
  maxParticipants?: number
}

export interface CouponState {
  coupons: Coupon[]
  seckillCoupons: SeckillCoupon[]
  currentCoupon: Coupon | null
  isLoading: boolean
  error: string | null
  filters: {
    type?: string
    status?: string
    search?: string
  }
  pagination: {
    page: number
    size: number
    total: number
  }
}

const initialState: CouponState = {
  coupons: [],
  seckillCoupons: [],
  currentCoupon: null,
  isLoading: false,
  error: null,
  filters: {},
  pagination: {
    page: 1,
    size: 20,
    total: 0,
  },
}

// 获取优惠券列表
export const fetchCoupons = createAsyncThunk(
  'coupon/fetchCoupons',
  async (params: {
    page?: number
    size?: number
    type?: string
    status?: string
    search?: string
  } = {}, { rejectWithValue }) => {
    try {
      const response = await getCoupons(params)
      return response
    } catch (error: any) {
      return rejectWithValue(error.message || '获取优惠券列表失败')
    }
  }
)

// 获取秒杀优惠券列表
export const fetchSeckillCoupons = createAsyncThunk(
  'coupon/fetchSeckillCoupons',
  async (_, { rejectWithValue }) => {
    try {
      const response = await getCoupons({ type: 'seckill' })
      return response.data || []
    } catch (error: any) {
      return rejectWithValue(error.message || '获取秒杀优惠券失败')
    }
  }
)

// 获取优惠券详情
export const fetchCouponDetail = createAsyncThunk(
  'coupon/fetchCouponDetail',
  async (couponId: string, { rejectWithValue }) => {
    try {
      const response = await getCouponDetail(couponId)
      return response
    } catch (error: any) {
      return rejectWithValue(error.message || '获取优惠券详情失败')
    }
  }
)

// 参与秒杀
export const participateSeckillAsync = createAsyncThunk(
  'coupon/participateSeckill',
  async (couponId: string, { rejectWithValue }) => {
    try {
      const response = await participateSeckill(couponId)
      return response
    } catch (error: any) {
      return rejectWithValue(error.message || '参与秒杀失败')
    }
  }
)

const couponSlice = createSlice({
  name: 'coupon',
  initialState,
  reducers: {
    clearError: (state) => {
      state.error = null
    },
    setFilters: (state, action: PayloadAction<Partial<CouponState['filters']>>) => {
      state.filters = { ...state.filters, ...action.payload }
      state.pagination.page = 1 // 重置页码
    },
    setPagination: (state, action: PayloadAction<Partial<CouponState['pagination']>>) => {
      state.pagination = { ...state.pagination, ...action.payload }
    },
    clearCurrentCoupon: (state) => {
      state.currentCoupon = null
    },
  },
  extraReducers: (builder) => {
    builder
      // 获取优惠券列表
      .addCase(fetchCoupons.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(fetchCoupons.fulfilled, (state, action) => {
        state.isLoading = false
        state.coupons = action.payload.data || []
        state.pagination.total = action.payload.total || 0
        state.error = null
      })
      .addCase(fetchCoupons.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
      // 获取秒杀优惠券列表
      .addCase(fetchSeckillCoupons.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(fetchSeckillCoupons.fulfilled, (state, action) => {
        state.isLoading = false
        state.seckillCoupons = action.payload
        state.error = null
      })
      .addCase(fetchSeckillCoupons.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
      // 获取优惠券详情
      .addCase(fetchCouponDetail.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(fetchCouponDetail.fulfilled, (state, action) => {
        state.isLoading = false
        state.currentCoupon = action.payload
        state.error = null
      })
      .addCase(fetchCouponDetail.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
      // 参与秒杀
      .addCase(participateSeckillAsync.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(participateSeckillAsync.fulfilled, (state, action) => {
        state.isLoading = false
        // 更新优惠券库存和参与人数
        const coupon = state.seckillCoupons.find(c => c.id === action.payload.couponId)
        if (coupon) {
          coupon.seckillStock -= 1
          coupon.seckillParticipants += 1
        }
        state.error = null
      })
      .addCase(participateSeckillAsync.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
  },
})

export const { clearError, setFilters, setPagination, clearCurrentCoupon } = couponSlice.actions
export default couponSlice.reducer
