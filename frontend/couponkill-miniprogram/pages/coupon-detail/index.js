const api = require('../../utils/api')

Page({
  data: {
    couponId: '',
    coupon: null,
    isLoading: true,
    isSeckill: false,
    canBuy: false,
    countdown: '',
  },

  onLoad(options) {
    const couponId = options.id ? String(options.id) : ''
    if (!couponId) return
    this.setData({ couponId })
    this.loadCouponDetail(couponId)
  },

  onUnload() {
    if (this.countdownTimer) clearInterval(this.countdownTimer)
  },

  async loadCouponDetail(couponId) {
    this.setData({ isLoading: true })
    try {
      const coupon = await api.getCoupon(couponId)
      const isSeckill = Number(coupon.type) === api.CouponType.SECKILL
      this.setData({ coupon, isSeckill, isLoading: false })
      if (isSeckill) {
        this.startCountdown()
      } else {
        const stock = Number(coupon.remainingStock || 0)
        this.setData({ canBuy: stock > 0 && Number(coupon.status) === 1 })
      }
    } catch (e) {
      this.setData({ isLoading: false })
      wx.showToast({ title: (e && e.message) || '加载失败', icon: 'none' })
    }
  },

  parseTime(v) {
    if (!v) return null
    const normalized = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}/.test(v) ? v.replace(' ', 'T') : v
    const n = Date.parse(normalized)
    return Number.isNaN(n) ? null : n
  },

  startCountdown() {
    if (this.countdownTimer) clearInterval(this.countdownTimer)
    this.countdownTimer = setInterval(() => {
      const coupon = this.data.coupon
      if (!coupon) return
      const now = Date.now()
      const start = this.parseTime(coupon.seckillStartAt)
      const end = this.parseTime(coupon.seckillEndAt)
      if (start == null || end == null) {
        this.setData({
          countdown: '请先配置秒杀时间窗',
          canBuy: false,
        })
        return
      }
      if (now < start) {
        this.formatCountdown(start - now, '即将开始')
        this.setData({ canBuy: false })
      } else if (now <= end) {
        this.formatCountdown(end - now, '秒杀中')
        const stock = Number(coupon.seckillRemainingStock || 0)
        this.setData({ canBuy: stock > 0 })
      } else {
        this.setData({ countdown: '秒杀已结束', canBuy: false })
      }
    }, 1000)
  },

  formatCountdown(diff, status) {
    const hours = Math.floor(diff / 3600000)
    const minutes = Math.floor((diff % 3600000) / 60000)
    const seconds = Math.floor((diff % 60000) / 1000)
    const pad = (n) => String(n).padStart(2, '0')
    this.setData({
      countdown: `${status}: ${pad(hours)}:${pad(minutes)}:${pad(seconds)}`,
    })
  },

  ensureLogin() {
    if (api.isLoggedIn()) return true
    wx.showModal({
      title: '提示',
      content: '请先登录',
      confirmText: '去登录',
      success: (res) => {
        if (res.confirm) wx.navigateTo({ url: '/pages/login/index' })
      },
    })
    return false
  },

  async buyNow() {
    if (!this.ensureLogin() || !this.data.canBuy) return
    try {
      wx.showLoading({ title: '领取中' })
      const order = await api.createOrder(this.data.couponId)
      wx.hideLoading()
      wx.navigateTo({ url: `/pages/order-detail/index?id=${order.id}` })
    } catch (e) {
      wx.hideLoading()
      wx.showToast({ title: (e && e.message) || '领取失败', icon: 'none' })
    }
  },

  async seckillNow() {
    if (!this.ensureLogin() || !this.data.canBuy) return
    try {
      wx.showLoading({ title: '抢购中' })
      await api.seckillUntilReceived(this.data.couponId)
      wx.hideLoading()
      wx.showToast({ title: '抢购成功', icon: 'success' })
      wx.switchTab({ url: '/pages/orders/index' })
    } catch (e) {
      wx.hideLoading()
      wx.showToast({ title: (e && e.message) || '秒杀失败', icon: 'none' })
    }
  },
})
