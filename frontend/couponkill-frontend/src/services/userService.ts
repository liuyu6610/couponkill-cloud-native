import { authService } from './authService'
import type { UserInfo } from '../types/api'

export const userService = {
  async getProfile(): Promise<UserInfo> {
    return authService.getProfile()
  },
}

export default userService
