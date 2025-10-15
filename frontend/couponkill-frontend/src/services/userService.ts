import apiClient from './authService'

// 用户相关API
export const userService = {
  // 获取用户资料
  async getUserProfile() {
    const response = await apiClient.get('/api/v1/user/profile')
    return response.data
  },

  // 更新用户资料
  async updateUserProfile(profileData: {
    username?: string
    email?: string
    phone?: string
    avatar?: string
    gender?: 'MALE' | 'FEMALE' | 'OTHER'
    birthday?: string
    address?: {
      province: string
      city: string
      district: string
      address: string
      postalCode?: string
    }
    preferences?: {
      categories: string[]
      notifications: boolean
      emailMarketing: boolean
    }
  }) {
    const response = await apiClient.put('/api/v1/user/profile', profileData)
    return response.data
  },

  // 获取用户优惠券
  async getUserCoupons(params: {
    status?: string
    page?: number
    size?: number
  } = {}) {
    const response = await apiClient.get('/api/v1/user/coupons', { params })
    return response.data
  },

  // 获取用户统计信息
  async getUserStats() {
    const response = await apiClient.get('/api/v1/user/stats')
    return response.data
  },

  // 修改密码
  async changePassword(data: {
    oldPassword: string
    newPassword: string
  }) {
    const response = await apiClient.put('/api/v1/user/password', data)
    return response.data
  },

  // 上传头像
  async uploadAvatar(file: File) {
    const formData = new FormData()
    formData.append('avatar', file)

    const response = await apiClient.post('/api/v1/user/avatar', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    })
    return response.data
  },
}

// 导出便捷方法
export const getUserProfile = userService.getUserProfile
export const updateUserProfile = userService.updateUserProfile
export const getUserCoupons = userService.getUserCoupons
export const getUserStats = userService.getUserStats

export default userService
