const api = require('../../utils/api')

Page({
  data: {
    form: {
      username: '',
      password: '',
      confirmPassword: '',
      phone: '',
    },
    isLoading: false,
  },

  onInputChange(e) {
    const field = e.currentTarget.dataset.field
    this.setData({ [`form.${field}`]: e.detail.value })
  },

  async register() {
    const form = this.data.form
    if (!form.username || !form.password || !form.phone) {
      wx.showToast({ title: '请填写用户名、密码、手机号', icon: 'none' })
      return
    }
    if (form.password.length < 6) {
      wx.showToast({ title: '密码至少6位', icon: 'none' })
      return
    }
    if (form.password !== form.confirmPassword) {
      wx.showToast({ title: '两次密码不一致', icon: 'none' })
      return
    }
    if (!/^1[3-9]\d{9}$/.test(form.phone)) {
      wx.showToast({ title: '手机号格式不正确', icon: 'none' })
      return
    }

    this.setData({ isLoading: true })
    try {
      await api.register(form.username.trim(), form.password, form.phone.trim())
      wx.showToast({ title: '注册成功', icon: 'success' })
      setTimeout(() => {
        wx.redirectTo({ url: '/pages/login/index' })
      }, 500)
    } catch (e) {
      wx.showToast({ title: (e && e.message) || '注册失败', icon: 'none' })
    } finally {
      this.setData({ isLoading: false })
    }
  },

  goLogin() {
    wx.redirectTo({ url: '/pages/login/index' })
  },
})
