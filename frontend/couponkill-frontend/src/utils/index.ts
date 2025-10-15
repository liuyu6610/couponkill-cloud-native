// 导入dayjs和相对时间插件
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'

// 启用相对时间插件
dayjs.extend(relativeTime)

// 通用类型定义
export interface ApiResponse<T = any> {
  code: number
  message: string
  data: T
  timestamp: string
}

export interface PaginationParams {
  page?: number
  size?: number
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  size: number
  totalPages: number
}

export interface BaseEntity {
  id: string
  createdAt: string
  updatedAt: string
}

// 日期格式化工具
export const formatDate = (date: string | Date, format = 'YYYY-MM-DD HH:mm:ss') => {
  return dayjs(date).format(format)
}

// 相对时间格式化
export const formatRelativeTime = (date: string | Date) => {
  return dayjs(date).fromNow()
}

// 金额格式化
export const formatMoney = (amount: number, currency = '¥') => {
  return `${currency}${amount.toFixed(2)}`
}

// 倒计时格式化
export const formatCountdown = (seconds: number) => {
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = seconds % 60

  if (hours > 0) {
    return `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
  } else {
    return `${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
  }
}

// 防抖函数
export const debounce = <T extends (...args: any[]) => any>(
  func: T,
  delay: number
): ((...args: Parameters<T>) => void) => {
  let timeoutId: number

  return (...args: Parameters<T>) => {
    clearTimeout(timeoutId)
    timeoutId = setTimeout(() => func(...args), delay)
  }
}

// 节流函数
export const throttle = <T extends (...args: any[]) => any>(
  func: T,
  delay: number
): ((...args: Parameters<T>) => void) => {
  let lastCall = 0

  return (...args: Parameters<T>) => {
    const now = Date.now()
    if (now - lastCall >= delay) {
      lastCall = now
      func(...args)
    }
  }
}

// 本地存储工具
export const storage = {
  get: <T>(key: string): T | null => {
    try {
      const item = localStorage.getItem(key)
      return item ? JSON.parse(item) : null
    } catch {
      return null
    }
  },

  set: <T>(key: string, value: T): void => {
    try {
      localStorage.setItem(key, JSON.stringify(value))
    } catch (error) {
      console.error('Failed to save to localStorage:', error)
    }
  },

  remove: (key: string): void => {
    try {
      localStorage.removeItem(key)
    } catch (error) {
      console.error('Failed to remove from localStorage:', error)
    }
  },

  clear: (): void => {
    try {
      localStorage.clear()
    } catch (error) {
      console.error('Failed to clear localStorage:', error)
    }
  },
}
