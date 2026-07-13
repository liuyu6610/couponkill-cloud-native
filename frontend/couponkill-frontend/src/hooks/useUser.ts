import { useQuery } from '@tanstack/react-query'
import { userService } from '../services/userService'
import { queryKeys, staleTimes } from '../lib/queryClient'

export function useUserProfile(userId?: string) {
  return useQuery({
    queryKey: queryKeys.user.profile(userId ?? 'me'),
    queryFn: () => userService.getProfile(),
    enabled: !!userId,
    staleTime: staleTimes.userProfile,
  })
}
