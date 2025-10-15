import apiClient from './authService'

// 订单相关API
export const orderService = {
  // 获取订单列表
  async getOrders(params: {
    page?: number
    size?: number
    status?: string
    startDate?: string
    endDate?: string
  } = {}) {
    const response = await apiClient.get('/api/v1/orders', { params })
    return response.data
  },

  // 获取订单详情
  async getOrderDetail(orderId: string) {
    const response = await apiClient.get(`/api/v1/orders/${orderId}`)
    return response.data
  },

  // 创建订单
  async createOrder(orderData: {
    couponId: string
    quantity?: number
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
  }) {
    const response = await apiClient.post('/api/v1/orders', orderData)
    return response.data
  },

  // 取消订单
  async cancelOrder(orderId: string, reason?: string) {
    const response = await apiClient.put(`/api/v1/orders/${orderId}/cancel`, {
      reason
    })
    return response.data
  },

  // 申请退款
  async requestRefund(orderId: string, reason: string) {
    const response = await apiClient.post(`/api/v1/orders/${orderId}/refund`, {
      reason
    })
    return response.data
  },

  // 确认收货
  async confirmReceive(orderId: string) {
    const response = await apiClient.put(`/api/v1/orders/${orderId}/receive`)
    return response.data
  },
}

// 导出便捷方法
export const getOrders = orderService.getOrders
export const getOrderDetail = orderService.getOrderDetail
export const createOrder = orderService.createOrder

export default orderService
