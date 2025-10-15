// utils/util.js
const app = getApp()

/**
 * 格式化日期
 * @param {string|Date} date - 日期
 * @param {string} format - 格式化字符串
 * @returns {string} 格式化后的日期字符串
 */
function formatDate(date, format = 'YYYY-MM-DD HH:mm:ss') {
  const d = new Date(date)
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const hours = String(d.getHours()).padStart(2, '0')
  const minutes = String(d.getMinutes()).padStart(2, '0')
  const seconds = String(d.getSeconds()).padStart(2, '0')

  return format
    .replace('YYYY', year)
    .replace('MM', month)
    .replace('DD', day)
    .replace('HH', hours)
    .replace('mm', minutes)
    .replace('ss', seconds)
}

/**
 * 格式化金额
 * @param {number} amount - 金额
 * @param {string} currency - 货币符号
 * @returns {string} 格式化后的金额字符串
 */
function formatMoney(amount, currency = '¥') {
  return `${currency}${Number(amount).toFixed(2)}`
}

/**
 * 防抖函数
 * @param {Function} func - 要防抖的函数
 * @param {number} delay - 延迟时间（毫秒）
 * @returns {Function} 防抖后的函数
 */
function debounce(func, delay = 300) {
  let timeoutId
  return function (...args) {
    clearTimeout(timeoutId)
    timeoutId = setTimeout(() => func.apply(this, args), delay)
  }
}

/**
 * 节流函数
 * @param {Function} func - 要节流的函数
 * @param {number} delay - 节流间隔（毫秒）
 * @returns {Function} 节流后的函数
 */
function throttle(func, delay = 300) {
  let lastCall = 0
  return function (...args) {
    const now = Date.now()
    if (now - lastCall >= delay) {
      lastCall = now
      return func.apply(this, args)
    }
  }
}

/**
 * 显示加载提示
 * @param {string} title - 提示文本
 */
function showLoading(title = '加载中...') {
  wx.showLoading({
    title: title,
    mask: true
  })
}

/**
 * 隐藏加载提示
 */
function hideLoading() {
  wx.hideLoading()
}

/**
 * 显示成功提示
 * @param {string} title - 提示文本
 * @param {number} duration - 显示时长（毫秒）
 */
function showSuccess(title = '成功', duration = 1500) {
  wx.showToast({
    title: title,
    icon: 'success',
    duration: duration
  })
}

/**
 * 显示错误提示
 * @param {string} title - 提示文本
 * @param {number} duration - 显示时长（毫秒）
 */
function showError(title = '错误', duration = 1500) {
  wx.showToast({
    title: title,
    icon: 'none',
    duration: duration
  })
}

/**
 * 确认对话框
 * @param {string} title - 标题
 * @param {string} content - 内容
 * @param {string} confirmText - 确认按钮文本
 * @param {string} cancelText - 取消按钮文本
 * @returns {Promise} Promise对象
 */
function showConfirm(title = '提示', content = '', confirmText = '确定', cancelText = '取消') {
  return new Promise((resolve) => {
    wx.showModal({
      title: title,
      content: content,
      confirmText: confirmText,
      cancelText: cancelText,
      success: (res) => {
        resolve(res.confirm)
      }
    })
  })
}

/**
 * 本地存储工具类
 */
const storage = {
  /**
   * 获取存储数据
   * @param {string} key - 键名
   * @param {*} defaultValue - 默认值
   * @returns {*} 存储的数据
   */
  get: function (key, defaultValue = null) {
    try {
      const value = wx.getStorageSync(key)
      return value !== '' ? value : defaultValue
    } catch (e) {
      console.error('获取存储数据失败：', e)
      return defaultValue
    }
  },

  /**
   * 设置存储数据
   * @param {string} key - 键名
   * @param {*} value - 值
   */
  set: function (key, value) {
    try {
      wx.setStorageSync(key, value)
    } catch (e) {
      console.error('设置存储数据失败：', e)
    }
  },

  /**
   * 删除存储数据
   * @param {string} key - 键名
   */
  remove: function (key) {
    try {
      wx.removeStorageSync(key)
    } catch (e) {
      console.error('删除存储数据失败：', e)
    }
  },

  /**
   * 清空所有存储数据
   */
  clear: function () {
    try {
      wx.clearStorageSync()
    } catch (e) {
      console.error('清空存储数据失败：', e)
    }
  }
}

/**
 * 网络请求工具类
 */
const request = {
  /**
   * GET请求
   * @param {string} url - 请求地址
   * @param {object} data - 请求数据
   * @param {object} options - 其他选项
   * @returns {Promise} Promise对象
   */
  get: function (url, data = {}, options = {}) {
    return this.request(url, data, { ...options, method: 'GET' })
  },

  /**
   * POST请求
   * @param {string} url - 请求地址
   * @param {object} data - 请求数据
   * @param {object} options - 其他选项
   * @returns {Promise} Promise对象
   */
  post: function (url, data = {}, options = {}) {
    return this.request(url, data, { ...options, method: 'POST' })
  },

  /**
   * 通用请求方法
   * @param {string} url - 请求地址
   * @param {object} data - 请求数据
   * @param {object} options - 请求选项
   * @returns {Promise} Promise对象
   */
  request: function (url, data = {}, options = {}) {
    const token = storage.get('token')

    return new Promise((resolve, reject) => {
      wx.request({
        url: url,
        method: options.method || 'GET',
        data: data,
        header: {
          'Content-Type': 'application/json',
          'Authorization': token ? `Bearer ${token}` : '',
          ...options.header
        },
        timeout: options.timeout || 10000,
        success: (res) => {
          if (res.statusCode === 200) {
            resolve(res.data)
          } else {
            reject(new Error(`请求失败: ${res.statusCode}`))
          }
        },
        fail: (err) => {
          reject(err)
        }
      })
    })
  }
}

/**
 * 登录检查装饰器
 * @param {Function} func - 需要登录检查的函数
 * @returns {Function} 包装后的函数
 */
function requireAuth(func) {
  return function (...args) {
    if (!app.isLoggedIn()) {
      wx.showModal({
        title: '提示',
        content: '请先登录',
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
    return func.apply(this, args)
  }
}

module.exports = {
  formatDate,
  formatMoney,
  debounce,
  throttle,
  showLoading,
  hideLoading,
  showSuccess,
  showError,
  showConfirm,
  storage,
  request,
  requireAuth
}
