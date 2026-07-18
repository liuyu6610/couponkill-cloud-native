const api = require('../../utils/api')
const app = getApp()

Page({
  data: {
    username: 'demo',
    password: 'demo1234',
    isLoading: false,
  },

  onLoad() {
    if (app.isLoggedIn()) {
      wx.switchTab({ url: '/pages/index/index' })
    }
  },

  onUsername(e) {
    this.setData({ username: e.detail.value })
  },

  onPassword(e) {
    this.setData({ password: e.detail.value })
  },

  async doLogin() {
    const username = (this.data.username || '').trim()
    const password = this.data.password || ''
    if (!username || !password) {
      wx.showToast({ title: '请输入用户名和密码', icon: 'none' })
      return
    }
    this.setData({ isLoading: true })
    try {
      const result = await api.login(username, password)
      wx.showToast({ title: '登录成功', icon: 'success' })
      // 冒烟可观测：userId 必须是字符串
      console.log('[login] userId typeof=', typeof result.userId, 'value=', result.userId)
      setTimeout(() => {
        wx.switchTab({ url: '/pages/index/index' })
      }, 400)
    } catch (e) {
      wx.showToast({ title: (e && e.message) || '登录失败', icon: 'none' })
    } finally {
      this.setData({ isLoading: false })
    }
  },

  goRegister() {
    wx.navigateTo({ url: '/pages/register/index' })
  },
})
