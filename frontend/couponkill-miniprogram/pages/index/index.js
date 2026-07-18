const api = require('../../utils/api')
const app = getApp()

Page({
  data: {
    hotCoupons: [],
    seckillCoupons: [],
    isLoading: true,
  },

  onLoad() {
    this.loadHomeData()
  },

  onPullDownRefresh() {
    this.loadHomeData(() => wx.stopPullDownRefresh())
  },

  async loadHomeData(callback) {
    this.setData({ isLoading: true })
    try {
      const list = await api.listAvailableCoupons()
      const seckill = list.filter((c) => Number(c.type) === api.CouponType.SECKILL)
      const normal = list.filter((c) => Number(c.type) !== api.CouponType.SECKILL)
      this.setData({
        hotCoupons: normal.slice(0, 6),
        seckillCoupons: seckill.slice(0, 3),
        isLoading: false,
      })
    } catch (e) {
      this.setData({ isLoading: false })
      wx.showToast({ title: (e && e.message) || '加载失败', icon: 'none' })
    }
    if (callback) callback()
  },

  goToCoupons() {
    wx.switchTab({ url: '/pages/coupons/index' })
  },

  goToSeckill() {
    wx.switchTab({ url: '/pages/seckill/index' })
  },

  goToCouponDetail(e) {
    const couponId = e.currentTarget.dataset.id
    wx.navigateTo({ url: `/pages/coupon-detail/index?id=${couponId}` })
  },

  goToLogin() {
    wx.navigateTo({ url: '/pages/login/index' })
  },

  async buyNow(e) {
    const couponId = String(e.currentTarget.dataset.id)
    if (!api.isLoggedIn()) {
      wx.showModal({
        title: '提示',
        content: '请先登录',
        confirmText: '去登录',
        success: (res) => {
          if (res.confirm) this.goToLogin()
        },
      })
      return
    }
    try {
      wx.showLoading({ title: '领取中' })
      const order = await api.createOrder(couponId)
      wx.hideLoading()
      wx.showToast({ title: '领取成功', icon: 'success' })
      wx.navigateTo({ url: `/pages/order-detail/index?id=${order.id}` })
    } catch (err) {
      wx.hideLoading()
      wx.showToast({ title: (err && err.message) || '领取失败', icon: 'none' })
    }
  },
})
