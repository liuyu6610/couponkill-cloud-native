import { createSlice, createAsyncThunk } from '@reduxjs/toolkit'
import type { PayloadAction } from '@reduxjs/toolkit'
import { getOrders, getOrderDetail, createOrder } from '../../services/orderService'

export interface OrderItem {
  id: string
  orderId: string
  couponId: string
  couponName: string
  couponType: string
  originalPrice: number
  discountAmount: number
  finalPrice: number
  quantity: number
}

export interface Order {
  id: string
  orderNo: string
  userId: string
  username: string
  status: 'PENDING' | 'PAID' | 'SHIPPED' | 'DELIVERED' | 'CANCELLED' | 'REFUNDING' | 'REFUNDED'
  totalAmount: number
  discountAmount: number
  paymentAmount: number
  paymentMethod?: string
  paymentTime?: string
  shipTime?: string
  receiveTime?: string
  orderItems: OrderItem[]
  shippingAddress?: {
    name: string
    phone: string
    province: string
    city: string
    district: string
    address: string
    postalCode?: string
  }
  remark?: string
  createdAt: string
  updatedAt: string
}

export interface OrderState {
  orders: Order[]
  currentOrder: Order | null
  isLoading: boolean
  error: string | null
  filters: {
    status?: string
    dateRange?: [string, string]
  }
  pagination: {
    page: number
    size: number
    total: number
  }
}

const initialState: OrderState = {
  orders: [],
  currentOrder: null,
  isLoading: false,
  error: null,
  filters: {},
  pagination: {
    page: 1,
    size: 20,
    total: 0,
  },
}

// 获取订单列表
export const fetchOrders = createAsyncThunk(
  'order/fetchOrders',
  async (params: {
    page?: number
    size?: number
    status?: string
    startDate?: string
    endDate?: string
  } = {}, { rejectWithValue }) => {
    try {
      const response = await getOrders(params)
      return response
    } catch (error: any) {
      return rejectWithValue(error.message || '获取订单列表失败')
    }
  }
)

// 获取订单详情
export const fetchOrderDetail = createAsyncThunk(
  'order/fetchOrderDetail',
  async (orderId: string, { rejectWithValue }) => {
    try {
      const response = await getOrderDetail(orderId)
      return response
    } catch (error: any) {
      return rejectWithValue(error.message || '获取订单详情失败')
    }
  }
)

// 创建订单
export const createOrderAsync = createAsyncThunk(
  'order/createOrder',
  async (orderData: {
    couponId: string
    quantity?: number
    shippingAddress?: Order['shippingAddress']
    remark?: string
  }, { rejectWithValue }) => {
    try {
      const response = await createOrder(orderData)
      return response
    } catch (error: any) {
      return rejectWithValue(error.message || '创建订单失败')
    }
  }
)

const orderSlice = createSlice({
  name: 'order',
  initialState,
  reducers: {
    clearError: (state) => {
      state.error = null
    },
    setFilters: (state, action: PayloadAction<Partial<OrderState['filters']>>) => {
      state.filters = { ...state.filters, ...action.payload }
      state.pagination.page = 1 // 重置页码
    },
    setPagination: (state, action: PayloadAction<Partial<OrderState['pagination']>>) => {
      state.pagination = { ...state.pagination, ...action.payload }
    },
    clearCurrentOrder: (state) => {
      state.currentOrder = null
    },
  },
  extraReducers: (builder) => {
    builder
      // 获取订单列表
      .addCase(fetchOrders.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(fetchOrders.fulfilled, (state, action) => {
        state.isLoading = false
        state.orders = action.payload.data || []
        state.pagination.total = action.payload.total || 0
        state.error = null
      })
      .addCase(fetchOrders.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
      // 获取订单详情
      .addCase(fetchOrderDetail.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(fetchOrderDetail.fulfilled, (state, action) => {
        state.isLoading = false
        state.currentOrder = action.payload
        state.error = null
      })
      .addCase(fetchOrderDetail.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
      // 创建订单
      .addCase(createOrderAsync.pending, (state) => {
        state.isLoading = true
        state.error = null
      })
      .addCase(createOrderAsync.fulfilled, (state, action) => {
        state.isLoading = false
        state.orders.unshift(action.payload) // 将新订单添加到列表开头
        state.error = null
      })
      .addCase(createOrderAsync.rejected, (state, action) => {
        state.isLoading = false
        state.error = action.payload as string
      })
  },
})

export const { clearError, setFilters, setPagination, clearCurrentOrder } = orderSlice.actions
export default orderSlice.reducer
