// pages/orders/index.js
const app = getApp()

Page({
  data: {
    orders: [],
    isLoading: true,
    hasMore: true,
    currentPage: 1,
    pageSize: 10,
    statusFilter: '',
    statusList: [
      { value: '', label: '全部订单' },
      { value: 'PENDING', label: '待支付' },
      { value: 'PAID', label: '已支付' },
      { value: 'SHIPPED', label: '已发货' },
      { value: 'DELIVERED', label: '已完成' },
      { value: 'CANCELLED', label: '已取消' }
    ]
  },

  onLoad: function () {
    this.loadOrders()
  },

  onPullDownRefresh: function () {
    this.setData({
      currentPage: 1,
      orders: [],
      hasMore: true
    })
    this.loadOrders(() => {
      wx.stopPullDownRefresh()
    })
  },

  onReachBottom: function () {
    if (this.data.hasMore && !this.data.isLoading) {
      this.loadMoreOrders()
    }
  },

  // 加载订单列表
  loadOrders: function (callback) {
    this.setData({ isLoading: true })

    wx.request({
      url: app.globalData.apiBase + '/orders',
      method: 'GET',
      header: {
        'Authorization': 'Bearer ' + wx.getStorageSync('token')
      },
      data: {
        page: this.data.currentPage,
        size: this.data.pageSize,
        status: this.data.statusFilter
      },
      success: (res) => {
        if (res.data.code === 200) {
          const data = res.data.data
          this.setData({
            orders: data.records || [],
            hasMore: this.data.currentPage < data.pages,
            currentPage: data.current,
            isLoading: false
          })
        } else {
          this.setData({ isLoading: false })
          wx.showToast({
            title: res.data.message || '加载失败',
            icon: 'none'
          })
        }
        if (callback) callback()
      },
      fail: () => {
        this.setData({ isLoading: false })
        wx.showToast({
          title: '网络错误',
          icon: 'none'
        })
        if (callback) callback()
      }
    })
  },

  // 加载更多订单
  loadMoreOrders: function () {
    if (!this.data.hasMore || this.data.isLoading) return

    this.setData({
      currentPage: this.data.currentPage + 1
    })

    wx.request({
      url: app.globalData.apiBase + '/orders',
      method: 'GET',
      header: {
        'Authorization': 'Bearer ' + wx.getStorageSync('token')
      },
      data: {
        page: this.data.currentPage,
        size: this.data.pageSize,
        status: this.data.statusFilter
      },
      success: (res) => {
        if (res.data.code === 200) {
          const data = res.data.data
          this.setData({
            orders: [...this.data.orders, ...(data.records || [])],
            hasMore: this.data.currentPage < data.pages
          })
        }
      }
    })
  },

  // 筛选订单状态
  onStatusFilter: function (e) {
    this.setData({
      statusFilter: e.detail.value,
      currentPage: 1,
      orders: [],
      hasMore: true
    })
    this.loadOrders()
  },

  // 跳转到订单详情
  goToOrderDetail: function (e) {
    const orderId = e.currentTarget.dataset.id
    wx.navigateTo({
      url: `/pages/order-detail/index?id=${orderId}`
    })
  },

  // 取消订单
  cancelOrder: function (e) {
    e.stopPropagation()
    const orderId = e.currentTarget.dataset.id

    wx.showModal({
      title: '确认取消',
      content: '确定要取消这个订单吗？',
      success: (res) => {
        if (res.confirm) {
          wx.request({
            url: app.globalData.apiBase + `/orders/${orderId}/cancel`,
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

                // 刷新订单列表
                this.setData({
                  currentPage: 1,
                  orders: [],
                  hasMore: true
                })
                this.loadOrders()
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
  }
})
