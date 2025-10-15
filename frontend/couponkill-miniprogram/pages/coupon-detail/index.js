// pages/coupon-detail/index.js
const app = getApp()

Page({
  data: {
    couponId: '',
    coupon: null,
    isLoading: true,
    countdown: '',
    canBuy: false,
    isSeckill: false
  },

  onLoad: function (options) {
    const couponId = options.id
    if (couponId) {
      this.setData({ couponId })
      this.loadCouponDetail(couponId)
    }
  },

  onUnload: function () {
    if (this.countdownTimer) {
      clearInterval(this.countdownTimer)
    }
  },

  // 加载优惠券详情
  loadCouponDetail: function (couponId) {
    this.setData({ isLoading: true })

    wx.request({
      url: app.globalData.apiBase + `/coupons/${couponId}`,
      method: 'GET',
      success: (res) => {
        if (res.data.code === 200) {
          const coupon = res.data.data
          this.setData({
            coupon: coupon,
            isSeckill: !!coupon.seckillStartTime,
            isLoading: false
          })

          // 如果是秒杀，开始倒计时
          if (coupon.seckillStartTime) {
            this.startCountdown()
          } else {
            this.checkCanBuy(coupon)
          }
        } else {
          this.setData({ isLoading: false })
          wx.showToast({
            title: res.data.message || '加载失败',
            icon: 'none'
          })
        }
      },
      fail: () => {
        this.setData({ isLoading: false })
        wx.showToast({
          title: '网络错误',
          icon: 'none'
        })
      }
    })
  },

  // 开始倒计时
  startCountdown: function () {
    this.countdownTimer = setInterval(() => {
      const coupon = this.data.coupon
      if (!coupon) return

      const now = new Date().getTime()
      const startTime = new Date(coupon.seckillStartTime).getTime()
      const endTime = new Date(coupon.seckillEndTime).getTime()

      if (now < startTime) {
        // 未开始
        const diff = startTime - now
        this.formatCountdown(diff, '即将开始')
      } else if (now < endTime) {
        // 进行中
        const diff = endTime - now
        this.formatCountdown(diff, '秒杀中')
      } else {
        // 已结束
        this.setData({
          countdown: '秒杀已结束',
          canBuy: false
        })
      }
    }, 1000)
  },

  // 格式化倒计时
  formatCountdown: function (diff, status) {
    const hours = Math.floor(diff / (1000 * 60 * 60))
    const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60))
    const seconds = Math.floor((diff % (1000 * 60)) / 1000)

    const countdown = `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`

    this.setData({
      countdown: `${status}: ${countdown}`,
      canBuy: status === '秒杀中' && diff > 0
    })
  },

  // 检查是否可以购买
  checkCanBuy: function (coupon) {
    const now = new Date().getTime()
    const startTime = new Date(coupon.startTime).getTime()
    const endTime = new Date(coupon.endTime).getTime()

    const canBuy = now >= startTime && now <= endTime && coupon.availableStock > 0
    this.setData({ canBuy })
  },

  // 立即购买
  buyNow: function () {
    if (!app.isLoggedIn()) {
      wx.showModal({
        title: '提示',
        content: '请先登录后再进行购买',
        showCancel: true,
        confirmText: '去登录',
        success: (res) => {
          if (res.confirm) {
            wx.navigateTo({
              url: '/pages/login/index'
            })
          }
        }
      })
      return
    }

    if (!this.data.canBuy) {
      wx.showToast({
        title: '暂无可购买',
        icon: 'none'
      })
      return
    }

    const coupon = this.data.coupon
    wx.request({
      url: app.globalData.apiBase + '/orders',
      method: 'POST',
      header: {
        'Authorization': 'Bearer ' + wx.getStorageSync('token')
      },
      data: {
        couponId: coupon.id,
        quantity: 1
      },
      success: (res) => {
        if (res.data.code === 200) {
          wx.showToast({
            title: '购买成功',
            icon: 'success'
          })

          // 跳转到订单详情
          wx.navigateTo({
            url: `/pages/order-detail/index?id=${res.data.data.id}`
          })
        } else {
          wx.showToast({
            title: res.data.message || '购买失败',
            icon: 'none'
          })
        }
      },
      fail: () => {
        wx.showToast({
          title: '网络错误',
          icon: 'none'
        })
      }
    })
  },

  // 秒杀购买
  seckillNow: function () {
    if (!app.isLoggedIn()) {
      wx.showModal({
        title: '提示',
        content: '请先登录后再进行秒杀',
        showCancel: true,
        confirmText: '去登录',
        success: (res) => {
          if (res.confirm) {
            wx.navigateTo({
              url: '/pages/login/index'
            })
          }
        }
      })
      return
    }

    if (!this.data.canBuy) {
      wx.showToast({
        title: '秒杀已结束',
        icon: 'none'
      })
      return
    }

    const coupon = this.data.coupon
    wx.request({
      url: app.globalData.apiBase + '/orders/seckill',
      method: 'POST',
      header: {
        'Authorization': 'Bearer ' + wx.getStorageSync('token')
      },
      data: {
        couponId: coupon.id
      },
      success: (res) => {
        if (res.data.code === 200) {
          wx.showToast({
            title: '秒杀成功',
            icon: 'success'
          })

          // 跳转到订单详情
          wx.navigateTo({
            url: `/pages/order-detail/index?id=${res.data.data.id}`
          })
        } else {
          wx.showToast({
            title: res.data.message || '秒杀失败',
            icon: 'none'
          })
        }
      },
      fail: () => {
        wx.showToast({
          title: '网络错误',
          icon: 'none'
        })
      }
    })
  },

  // 分享优惠券
  onShareAppMessage: function () {
    const coupon = this.data.coupon
    return {
      title: `${coupon.name} - ${coupon.description}`,
      path: `/pages/coupon-detail/index?id=${coupon.id}`,
      imageUrl: coupon.image || ''
    }
  }
})
