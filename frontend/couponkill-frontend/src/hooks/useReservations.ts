import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { reservationService } from '../services/reservationService'
import { queryKeys, staleTimes } from '../lib/queryClient'
import { ReservationStatus } from '../types/api'

const ACTIVE_RESERVATION = new Set<string>([
  ReservationStatus.PENDING,
  ReservationStatus.FIRING,
  ReservationStatus.QUEUED,
])

export function isActiveReservationStatus(status?: string): boolean {
  return !!status && ACTIVE_RESERVATION.has(status)
}

export function useMyReservations(enabled = true) {
  return useQuery({
    queryKey: queryKeys.reservations.mine,
    queryFn: () => reservationService.listMine(),
    enabled,
    staleTime: staleTimes.reservations,
    refetchInterval: (q) => {
      const list = q.state.data
      if (!list?.length) return false
      const active = list.some((r) => isActiveReservationStatus(r.status))
      return active ? 3000 : false
    },
  })
}

/** couponId → 活跃预约状态（PENDING/FIRING/QUEUED） */
export function useActiveReservationMap(enabled = true) {
  const q = useMyReservations(enabled)
  const map = new Map<string, string>()
  for (const r of q.data ?? []) {
    if (isActiveReservationStatus(r.status) && r.couponId != null) {
      map.set(String(r.couponId), r.status)
    }
  }
  return { ...q, activeByCouponId: map }
}

export function useCreateReservation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (couponId: string) => reservationService.create(couponId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.reservations.root })
    },
  })
}

export function useCancelReservation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => reservationService.cancel(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.reservations.root })
    },
  })
}
