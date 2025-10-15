// app.js
App({
  globalData: {
    userInfo: null,
    apiBase: 'https://your-api-domain.com/api/v1',
    version: '1.0.0'
  },

  onLaunch: function () {
    console.log('小程序启动')
    // 检查登录状态
    this.checkLoginStatus()

    // 获取系统信息
    wx.getSystemInfo({
      success: (res) => {
        this.globalData.systemInfo = res
        console.log('系统信息：', res)
      }
    })
  },

  onShow: function () {
    console.log('小程序显示')
  },

  onHide: function () {
    console.log('小程序隐藏')
  },

  // 检查登录状态
  checkLoginStatus: function () {
    const token = wx.getStorageSync('token')
    if (token) {
      // 验证token有效性
      this.validateToken(token)
    }
  },

  // 验证token
  validateToken: function (token) {
    wx.request({
      url: this.globalData.apiBase + '/auth/validate',
      method: 'POST',
      header: {
        'Authorization': 'Bearer ' + token
      },
      success: (res) => {
        if (res.data.code === 200) {
          this.globalData.userInfo = res.data.data
        } else {
          // token无效，清除本地存储
          wx.removeStorageSync('token')
          this.globalData.userInfo = null
        }
      },
      fail: () => {
        wx.removeStorageSync('token')
        this.globalData.userInfo = null
      }
    })
  },

  // 登录方法
  login: function (callback) {
    wx.login({
      success: (res) => {
        wx.request({
          url: this.globalData.apiBase + '/auth/wechat-login',
          method: 'POST',
          data: {
            code: res.code
          },
          success: (loginRes) => {
            if (loginRes.data.code === 200) {
              const { token, user } = loginRes.data.data
              wx.setStorageSync('token', token)
              this.globalData.userInfo = user
              if (callback) callback(user)
            } else {
              wx.showToast({
                title: '登录失败',
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
  },

  // 获取用户信息
  getUserInfo: function () {
    return this.globalData.userInfo
  },

  // 是否已登录
  isLoggedIn: function () {
    return !!this.globalData.userInfo
  }
})
