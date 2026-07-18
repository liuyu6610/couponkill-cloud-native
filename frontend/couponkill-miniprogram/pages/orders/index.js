const api = require('../../utils/api')

Page({
  data: {
    orders: [],
    isLoading: true,
    pageNum: 1,
    pageSize: 10,
    hasMore: true,
  },

  onShow() {
    if (!api.isLoggedIn()) {
      wx.navigateTo({ url: '/pages/login/index' })
      return
    }
    this.setData({ pageNum: 1, orders: [], hasMore: true })
    this.loadOrders()
  },

  onPullDownRefresh() {
    this.setData({ pageNum: 1, orders: [], hasMore: true })
    this.loadOrders(() => wx.stopPullDownRefresh())
  },

  onReachBottom() {
    if (this.data.hasMore && !this.data.isLoading) {
      this.setData({ pageNum: this.data.pageNum + 1 })
      this.loadOrders(null, true)
    }
  },

  async loadOrders(cb, append) {
    this.setData({ isLoading: true })
    try {
      const list = await api.listMyOrders(this.data.pageNum, this.data.pageSize)
      const mapped = list.map((o) => ({
        ...o,
        statusText: api.orderStatusText(o.status),
      }))
      // 观测：订单侧 ID 均为字符串
      if (mapped[0]) {
        console.log(
          '[orders] sample id/userId/couponId types=',
          typeof mapped[0].id,
          typeof mapped[0].userId,
          typeof mapped[0].couponId
        )
      }
      this.setData({
        orders: append ? this.data.orders.concat(mapped) : mapped,
        hasMore: mapped.length >= this.data.pageSize,
        isLoading: false,
      })
    } catch (e) {
      this.setData({ isLoading: false })
      wx.showToast({ title: (e && e.message) || '加载失败', icon: 'none' })
    }
    if (cb) cb()
  },

  goToOrderDetail(e) {
    wx.navigateTo({
      url: '/pages/order-detail/index?id=' + e.currentTarget.dataset.id,
    })
  },

  cancelOrder(e) {
    const orderId = String(e.currentTarget.dataset.id)
    wx.showModal({
      title: '确认取消',
      content: '确定取消该订单？',
      success: async (res) => {
        if (!res.confirm) return
        try {
          await api.cancelOrder(orderId)
          wx.showToast({ title: '已取消', icon: 'success' })
          this.setData({ pageNum: 1, orders: [], hasMore: true })
          this.loadOrders()
        } catch (err) {
          wx.showToast({ title: (err && err.message) || '取消失败', icon: 'none' })
        }
      },
    })
  },
})
