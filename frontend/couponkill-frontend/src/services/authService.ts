import axios from 'axios'
import type { User } from '../store/slices/authSlice'

// 创建axios实例
const apiClient = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080',
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json',
  },
})

// 请求拦截器：添加认证token
apiClient.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// 响应拦截器：处理通用错误
apiClient.interceptors.response.use(
  (response) => {
    return response.data
  },
  (error) => {
    if (error.response?.status === 401) {
      // token过期或无效，清除本地存储
      localStorage.removeItem('token')
      window.location.href = '/login'
    }
    return Promise.reject(error.response?.data || error.message)
  }
)

// 认证相关API
export const authService = {
  // 用户登录
  async login(username: string, password: string) {
    const response = await apiClient.post('/api/v1/auth/login', {
      username,
      password,
    })
    return response.data
  },

  // 用户注册
  async register(userData: {
    username: string
    password: string
    email: string
    phone?: string
  }) {
    const response = await apiClient.post('/api/v1/auth/register', userData)
    return response.data
  },

  // 用户登出
  async logout() {
    await apiClient.post('/api/v1/auth/logout')
  },

  // 获取当前用户信息
  async getCurrentUser(): Promise<User> {
    const response = await apiClient.get('/api/v1/auth/me')
    return response.data
  },

  // 刷新token
  async refreshToken() {
    const response = await apiClient.post('/api/v1/auth/refresh')
    return response.data
  },
}

// 导出便捷方法
export const login = authService.login
export const register = authService.register
export const logout = authService.logout

export default apiClient
