// pages/order-detail/index.js
const app = getApp()

Page({
  data: {
    orderId: '',
    order: null,
    isLoading: true,
    logisticsInfo: null
  },

  onLoad: function (options) {
    const orderId = options.id
    if (orderId) {
      this.setData({ orderId })
      this.loadOrderDetail(orderId)
    }
  },

  // 加载订单详情
  loadOrderDetail: function (orderId) {
    this.setData({ isLoading: true })

    wx.request({
      url: app.globalData.apiBase + `/orders/${orderId}`,
      method: 'GET',
      header: {
        'Authorization': 'Bearer ' + wx.getStorageSync('token')
      },
      success: (res) => {
        if (res.data.code === 200) {
          this.setData({
            order: res.data.data,
            isLoading: false
          })

          // 如果订单已发货，获取物流信息
          if (res.data.data.status === 'SHIPPED') {
            this.loadLogisticsInfo(orderId)
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

  // 加载物流信息
  loadLogisticsInfo: function (orderId) {
    wx.request({
      url: app.globalData.apiBase + `/orders/${orderId}/logistics`,
      method: 'GET',
      header: {
        'Authorization': 'Bearer ' + wx.getStorageSync('token')
      },
      success: (res) => {
        if (res.data.code === 200) {
          this.setData({
            logisticsInfo: res.data.data
          })
        }
      }
    })
  },

  // 取消订单
  cancelOrder: function () {
    const order = this.data.order

    wx.showModal({
      title: '确认取消',
      content: '确定要取消这个订单吗？',
      success: (res) => {
        if (res.confirm) {
          wx.request({
            url: app.globalData.apiBase + `/orders/${order.id}/cancel`,
            method: 'POST',
            header: {
              'Authorization': 'Bearer ' + wx.getStorageSync('token')
            },
            success: (cancelRes) => {
              if (cancelRes.data.code === 200) {
                wx.showToast({
                  title: '取消成功',
                  icon: 'success'
                })

                // 刷新订单详情
                this.loadOrderDetail(order.id)
              } else {
                wx.showToast({
                  title: cancelRes.data.message || '取消失败',
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
      }
    })
  },

  // 确认收货
  confirmDelivery: function () {
    const order = this.data.order

    wx.showModal({
      title: '确认收货',
      content: '确认已收到货物吗？',
      success: (res) => {
        if (res.confirm) {
          wx.request({
            url: app.globalData.apiBase + `/orders/${order.id}/confirm`,
            method: 'POST',
            header: {
              'Authorization': 'Bearer ' + wx.getStorageSync('token')
            },
            success: (confirmRes) => {
              if (confirmRes.data.code === 200) {
                wx.showToast({
                  title: '确认成功',
                  icon: 'success'
                })

                // 刷新订单详情
                this.loadOrderDetail(order.id)
              } else {
                wx.showToast({
                  title: confirmRes.data.message || '确认失败',
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
      }
    })
  },

  // 申请退款
  requestRefund: function () {
    const order = this.data.order

    wx.showModal({
      title: '申请退款',
      content: '确定要申请退款吗？',
      success: (res) => {
        if (res.confirm) {
          wx.request({
            url: app.globalData.apiBase + `/orders/${order.id}/refund`,
            method: 'POST',
            header: {
              'Authorization': 'Bearer ' + wx.getStorageSync('token')
            },
            success: (refundRes) => {
              if (refundRes.data.code === 200) {
                wx.showToast({
                  title: '申请成功',
                  icon: 'success'
                })

                // 刷新订单详情
                this.loadOrderDetail(order.id)
              } else {
                wx.showToast({
                  title: refundRes.data.message || '申请失败',
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
      }
    })
  },

  // 获取订单状态文本
  getStatusText: function (status) {
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
  getStatusColor: function (status) {
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
  },

  // 格式化日期
  formatDate: function (dateStr) {
    const date = new Date(dateStr)
    const year = date.getFullYear()
    const month = String(date.getMonth() + 1).padStart(2, '0')
    const day = String(date.getDate()).padStart(2, '0')
    const hours = String(date.getHours()).padStart(2, '0')
    const minutes = String(date.getMinutes()).padStart(2, '0')
    return `${year}-${month}-${day} ${hours}:${minutes}`
  },

  // 返回上一页
  goBack: function () {
    wx.navigateBack()
  }
})
