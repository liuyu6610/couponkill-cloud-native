// pages/index/index.js
const app = getApp()

Page({
  data: {
    banners: [],
    statistics: {
      totalCoupons: 0,
      totalUsers: 0,
      totalOrders: 0,
      totalSales: 0
    },
    hotCoupons: [],
    seckillCoupons: [],
    isLoading: true
  },

  onLoad: function () {
    this.loadHomeData()
  },

  onPullDownRefresh: function () {
    this.loadHomeData(() => {
      wx.stopPullDownRefresh()
    })
  },

  // 加载首页数据
  loadHomeData: function (callback) {
    this.setData({ isLoading: true })

    // 并发请求多个接口
    Promise.all([
      this.getBanners(),
      this.getStatistics(),
      this.getHotCoupons(),
      this.getSeckillCoupons()
    ]).then(() => {
      this.setData({ isLoading: false })
      if (callback) callback()
    }).catch((error) => {
      console.error('加载首页数据失败：', error)
      this.setData({ isLoading: false })
      wx.showToast({
        title: '加载失败',
        icon: 'none'
      })
      if (callback) callback()
    })
  },

  // 获取轮播图
  getBanners: function () {
    return new Promise((resolve, reject) => {
      wx.request({
        url: app.globalData.apiBase + '/banners',
        method: 'GET',
        success: (res) => {
          if (res.data.code === 200) {
            this.setData({
              banners: res.data.data || []
            })
          }
          resolve(res.data)
        },
        fail: reject
      })
    })
  },

  // 获取统计数据
  getStatistics: function () {
    return new Promise((resolve, reject) => {
      wx.request({
        url: app.globalData.apiBase + '/statistics',
        method: 'GET',
        success: (res) => {
          if (res.data.code === 200) {
            this.setData({
              statistics: res.data.data || this.data.statistics
            })
          }
          resolve(res.data)
        },
        fail: reject
      })
    })
  },

  // 获取热门优惠券
  getHotCoupons: function () {
    return new Promise((resolve, reject) => {
      wx.request({
        url: app.globalData.apiBase + '/coupons/hot',
        method: 'GET',
        data: {
          page: 1,
          size: 6
        },
        success: (res) => {
          if (res.data.code === 200) {
            this.setData({
              hotCoupons: res.data.data.records || []
            })
          }
          resolve(res.data)
        },
        fail: reject
      })
    })
  },

  // 获取秒杀优惠券
  getSeckillCoupons: function () {
    return new Promise((resolve, reject) => {
      wx.request({
        url: app.globalData.apiBase + '/coupons/seckill',
        method: 'GET',
        data: {
          page: 1,
          size: 3
        },
        success: (res) => {
          if (res.data.code === 200) {
            this.setData({
              seckillCoupons: res.data.data.records || []
            })
          }
          resolve(res.data)
        },
        fail: reject
      })
    })
  },

  // 跳转到优惠券列表
  goToCoupons: function () {
    wx.switchTab({
      url: '/pages/coupons/index'
    })
  },

  // 跳转到秒杀专区
  goToSeckill: function () {
    wx.switchTab({
      url: '/pages/seckill/index'
    })
  },

  // 跳转到优惠券详情
  goToCouponDetail: function (e) {
    const couponId = e.currentTarget.dataset.id
    wx.navigateTo({
      url: `/pages/coupon-detail/index?id=${couponId}`
    })
  },

  // 跳转到登录页
  goToLogin: function () {
    wx.navigateTo({
      url: '/pages/login/index'
    })
  },

  // 立即抢购
  buyNow: function (e) {
    const couponId = e.currentTarget.dataset.id

    if (!app.isLoggedIn()) {
      wx.showModal({
        title: '提示',
        content: '请先登录后再进行购买',
        showCancel: true,
        confirmText: '去登录',
        success: (res) => {
          if (res.confirm) {
            this.goToLogin()
          }
        }
      })
      return
    }

    // 调用购买接口
    wx.request({
      url: app.globalData.apiBase + '/orders',
      method: 'POST',
      header: {
        'Authorization': 'Bearer ' + wx.getStorageSync('token')
      },
      data: {
        couponId: couponId,
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
  }
})
