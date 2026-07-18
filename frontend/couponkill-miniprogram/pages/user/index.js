const api = require('../../utils/api')
const app = getApp()

Page({
  data: {
    userInfo: null,
    recentOrders: [],
    isLoading: true,
  },

  onShow() {
    this.loadUserData()
  },

  onPullDownRefresh() {
    this.loadUserData(() => wx.stopPullDownRefresh())
  },

  async loadUserData(cb) {
    if (!api.isLoggedIn()) {
      wx.redirectTo({ url: '/pages/login/index' })
      return
    }
    this.setData({ isLoading: true })
    try {
      const profile = await api.getProfile()
      const orders = await api.listMyOrders(1, 5)
      this.setData({
        userInfo: profile,
        recentOrders: orders.map((o) => ({
          ...o,
          statusText: api.orderStatusText(o.status),
        })),
        isLoading: false,
      })
    } catch (e) {
      this.setData({ isLoading: false })
      wx.showToast({ title: (e && e.message) || '加载失败', icon: 'none' })
    }
    if (cb) cb()
  },

  goOrders() {
    wx.switchTab({ url: '/pages/orders/index' })
  },

  logout() {
    app.logout()
    wx.redirectTo({ url: '/pages/login/index' })
  },
})
