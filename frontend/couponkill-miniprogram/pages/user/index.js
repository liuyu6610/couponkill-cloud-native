// pages/user/index.js
const app = getApp()

Page({
  data: {
    userInfo: null,
    isLoading: true,
    stats: {
      totalCoupons: 0,
      usedCoupons: 0,
      totalSavings: 0,
      totalOrders: 0
    },
    recentCoupons: [],
    recentOrders: []
  },

  onLoad: function () {
    this.loadUserData()
  },

  onShow: function () {
    // 每次显示页面时刷新数据
    if (app.isLoggedIn()) {
      this.loadUserData()
    }
  },

  onPullDownRefresh: function () {
    this.loadUserData(() => {
      wx.stopPullDownRefresh()
    })
  },

  // 加载用户数据
  loadUserData: function (callback) {
    if (!app.isLoggedIn()) {
      wx.redirectTo({
        url: '/pages/login/index'
      })
      return
    }

    this.setData({ isLoading: true })

    const userInfo = app.getUserInfo()
    this.setData({ userInfo: userInfo })

    // 并发加载用户统计、优惠券和订单
    Promise.all([
      this.loadUserStats(),
      this.loadRecentCoupons(),
      this.loadRecentOrders()
    ]).then(() => {
      this.setData({ isLoading: false })
      if (callback) callback()
    }).catch((error) => {
      console.error('加载用户数据失败：', error)
      this.setData({ isLoading: false })
      wx.showToast({
        title: '加载失败',
        icon: 'none'
      })
      if (callback) callback()
    })
  },

  // 加载用户统计数据
  loadUserStats: function () {
    return new Promise((resolve, reject) => {
      wx.request({
        url: app.globalData.apiBase + '/user/stats',
        method: 'GET',
        header: {
          'Authorization': 'Bearer ' + wx.getStorageSync('token')
        },
        success: (res) => {
          if (res.data.code === 200) {
            this.setData({
              stats: res.data.data || this.data.stats
            })
          }
          resolve(res.data)
        },
        fail: reject
      })
    })
  },

  // 加载最近优惠券
  loadRecentCoupons: function () {
    return new Promise((resolve, reject) => {
      wx.request({
        url: app.globalData.apiBase + '/user/coupons',
        method: 'GET',
        header: {
          'Authorization': 'Bearer ' + wx.getStorageSync('token')
        },
        data: {
          page: 1,
          size: 3
        },
        success: (res) => {
          if (res.data.code === 200) {
            this.setData({
              recentCoupons: res.data.data.records || []
            })
          }
          resolve(res.data)
        },
        fail: reject
      })
    })
  },

  // 加载最近订单
  loadRecentOrders: function () {
    return new Promise((resolve, reject) => {
      wx.request({
        url: app.globalData.apiBase + '/user/orders',
        method: 'GET',
        header: {
          'Authorization': 'Bearer ' + wx.getStorageSync('token')
        },
        data: {
          page: 1,
          size: 3
        },
        success: (res) => {
          if (res.data.code === 200) {
            this.setData({
              recentOrders: res.data.data.records || []
            })
          }
          resolve(res.data)
        },
        fail: reject
      })
    })
  },

  // 跳转到优惠券管理
  goToCoupons: function () {
    wx.navigateTo({
      url: '/pages/user-coupons/index'
    })
  },

  // 跳转到订单管理
  goToOrders: function () {
    wx.navigateTo({
      url: '/pages/orders/index'
    })
  },

  // 跳转到个人信息编辑
  goToProfile: function () {
    wx.navigateTo({
      url: '/pages/user-profile/index'
    })
  },

  // 跳转到设置页面
  goToSettings: function () {
    wx.navigateTo({
      url: '/pages/settings/index'
    })
  },

  // 退出登录
  logout: function () {
    wx.showModal({
      title: '确认退出',
      content: '确定要退出登录吗？',
      success: (res) => {
        if (res.confirm) {
          // 清除本地存储
          wx.removeStorageSync('token')
          app.globalData.userInfo = null

          // 跳转到首页
          wx.switchTab({
            url: '/pages/index/index'
          })
        }
      }
    })
  },

  // 获取订单状态文本
  getOrderStatusText: function (status) {
    const statusMap = {
      'PENDING': '待支付',
      'PAID': '已支付',
      'SHIPPED': '已发货',
      'DELIVERED': '已完成',
      'CANCELLED': '已取消',
      'REFUNDING': '退款中',
      'REFUNDED': '已退款'
    }
    return statusMap[status] || status
  },

  // 获取订单状态颜色
  getOrderStatusColor: function (status) {
    const colorMap = {
      'PENDING': '#ff9800',
      'PAID': '#2196f3',
      'SHIPPED': '#9c27b0',
      'DELIVERED': '#4caf50',
      'CANCELLED': '#f44336',
      'REFUNDING': '#ff9800',
      'REFUNDED': '#4caf50'
    }
    return colorMap[status] || '#666'
  }
})
