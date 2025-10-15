// pages/login/index.js
const app = getApp()

Page({
  data: {
    isLoading: false
  },

  onLoad: function () {
    // 检查是否已经登录
    if (app.isLoggedIn()) {
      wx.switchTab({
        url: '/pages/index/index'
      })
    }
  },

  // 微信登录
  wechatLogin: function () {
    this.setData({ isLoading: true })

    app.login((userInfo) => {
      this.setData({ isLoading: false })

      wx.showToast({
        title: '登录成功',
        icon: 'success'
      })

      // 跳转到首页
      wx.switchTab({
        url: '/pages/index/index'
      })
    })
  }
})
