// app.js — 对齐现网 JWT + /api/v1 契约
const api = require('./utils/api')

App({
  globalData: {
    userInfo: null,
    // 本地联调网关；真机/体验版请改为可访问的公网或内网 Gateway 地址
    apiBase: 'http://127.0.0.1:8088',
    version: '1.1.0',
  },

  onLaunch() {
    this.restoreSession()
  },

  restoreSession() {
    const token = wx.getStorageSync(api.STORAGE.token)
    const userId = wx.getStorageSync(api.STORAGE.userId)
    const username = wx.getStorageSync(api.STORAGE.username)
    if (!token || !userId) {
      this.globalData.userInfo = null
      return
    }
    this.globalData.userInfo = {
      id: String(userId),
      username: String(username || ''),
      roles: JSON.parse(wx.getStorageSync(api.STORAGE.roles) || '[]'),
    }
    // 用 profile 校验 token；失败则清会话
    api
      .getProfile()
      .then((u) => {
        this.globalData.userInfo = {
          id: String(u.id),
          username: u.username,
          roles: this.globalData.userInfo.roles || [],
        }
      })
      .catch(() => {
        api.clearAuth()
      })
  },

  getUserInfo() {
    return this.globalData.userInfo
  },

  isLoggedIn() {
    return api.isLoggedIn() && !!this.globalData.userInfo
  },

  logout() {
    api.clearAuth()
  },
})
