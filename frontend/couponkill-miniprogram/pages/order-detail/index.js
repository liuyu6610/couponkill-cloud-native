const api = require('../../utils/api')

Page({
  data: {
    orderId: '',
    order: null,
    isLoading: true,
    statusText: '',
  },

  onLoad(options) {
    const orderId = options.id ? String(options.id) : ''
    this.setData({ orderId })
    this.loadOrderDetail(orderId)
  },

  async loadOrderDetail(orderId) {
    if (!api.isLoggedIn()) {
      wx.navigateTo({ url: '/pages/login/index' })
      return
    }
    this.setData({ isLoading: true })
    try {
      // 后端无单订单 GET：从最近订单列表解析（与 Web 一致）
      const list = await api.listMyOrders(1, 50)
      const order = (list || []).find((o) => String(o.id) === String(orderId))
      if (!order) {
        this.setData({ order: null, isLoading: false })
        wx.showToast({ title: '订单不在最近列表中', icon: 'none' })
        return
      }
      this.setData({
        order,
        statusText: api.orderStatusText(order.status),
        isLoading: false,
      })
      console.log(
        '[order-detail] id/userId/couponId types=',
        typeof order.id,
        typeof order.userId,
        typeof order.couponId
      )
    } catch (e) {
      this.setData({ isLoading: false })
      wx.showToast({ title: (e && e.message) || '加载失败', icon: 'none' })
    }
  },

  async cancelOrder() {
    const order = this.data.order
    if (!order) return
    wx.showModal({
      title: '确认取消',
      content: '确定取消该订单？',
      success: async (res) => {
        if (!res.confirm) return
        try {
          await api.cancelOrder(order.id)
          wx.showToast({ title: '已取消', icon: 'success' })
          this.loadOrderDetail(order.id)
        } catch (err) {
          wx.showToast({ title: (err && err.message) || '取消失败', icon: 'none' })
        }
      },
    })
  },
})
