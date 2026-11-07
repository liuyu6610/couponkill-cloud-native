const api = require('../../utils/api')

Page({
  data: {
    coupons: [],
    isLoading: true,
  },

  onShow() {
    this.load()
  },

  onPullDownRefresh() {
    this.load(() => wx.stopPullDownRefresh())
  },

  async load(cb) {
    this.setData({ isLoading: true })
    try {
      const all = await api.listAvailableCoupons()
      const coupons = all.filter((c) => Number(c.type) === api.CouponType.SECKILL)
      this.setData({ coupons, isLoading: false })
    } catch (e) {
      this.setData({ isLoading: false })
      wx.showToast({ title: (e && e.message) || '加载失败', icon: 'none' })
    }
    if (cb) cb()
  },

  goDetail(e) {
    wx.navigateTo({
      url: '/pages/coupon-detail/index?id=' + e.currentTarget.dataset.id,
    })
  },
})
