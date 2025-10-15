// pages/register/index.js
const app = getApp()

Page({
  data: {
    form: {
      username: '',
      email: '',
      password: '',
      confirmPassword: '',
      phone: ''
    },
    isLoading: false,
    passwordVisible: false,
    confirmPasswordVisible: false
  },

  // 输入框变化
  onInputChange: function (e) {
    const field = e.currentTarget.dataset.field
    this.setData({
      [`form.${field}`]: e.detail.value
    })
  },

  // 切换密码可见性
  togglePasswordVisible: function () {
    this.setData({
      passwordVisible: !this.data.passwordVisible
    })
  },

  // 切换确认密码可见性
  toggleConfirmPasswordVisible: function () {
    this.setData({
      confirmPasswordVisible: !this.data.confirmPasswordVisible
    })
  },

  // 注册
  register: function () {
    const form = this.data.form

    // 表单验证
    if (!form.username) {
      wx.showToast({
        title: '请输入用户名',
        icon: 'none'
      })
      return
    }

    if (!form.email) {
      wx.showToast({
        title: '请输入邮箱',
        icon: 'none'
      })
      return
    }

    if (!form.password) {
      wx.showToast({
        title: '请输入密码',
        icon: 'none'
      })
      return
    }

    if (form.password !== form.confirmPassword) {
      wx.showToast({
        title: '两次密码不一致',
        icon: 'none'
      })
      return
    }

    if (form.password.length < 6) {
      wx.showToast({
        title: '密码至少6位',
        icon: 'none'
      })
      return
    }

    this.setData({ isLoading: true })

    wx.request({
      url: app.globalData.apiBase + '/auth/register',
      method: 'POST',
      data: {
        username: form.username,
        email: form.email,
        password: form.password,
        phone: form.phone
      },
      success: (res) => {
        this.setData({ isLoading: false })

        if (res.data.code === 200) {
          wx.showToast({
            title: '注册成功',
            icon: 'success'
          })

          // 跳转到首页
          setTimeout(() => {
            wx.switchTab({
              url: '/pages/index/index'
            })
          }, 1500)
        } else {
          wx.showToast({
            title: res.data.message || '注册失败',
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

  // 跳转到登录页
  goToLogin: function () {
    wx.navigateTo({
      url: '/pages/login/index'
    })
  },

  // 获取验证码（预留功能）
  getVerificationCode: function () {
    const email = this.data.form.email

    if (!email) {
      wx.showToast({
        title: '请输入邮箱',
        icon: 'none'
      })
      return
    }

    // 发送验证码请求
    wx.request({
      url: app.globalData.apiBase + '/auth/send-verification',
      method: 'POST',
      data: {
        email: email,
        type: 'register'
      },
      success: (res) => {
        if (res.data.code === 200) {
          wx.showToast({
            title: '验证码已发送',
            icon: 'success'
          })

          // 开始倒计时
          this.startCountdown()
        } else {
          wx.showToast({
            title: res.data.message || '发送失败',
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

  // 开始倒计时（预留功能）
  startCountdown: function () {
    // 这里可以实现验证码倒计时功能
  }
})
