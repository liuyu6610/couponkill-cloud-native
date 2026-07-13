import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { connectorService } from '../services/connectorService'
import { queryKeys, staleTimes } from '../lib/queryClient'
import type { ConnectorPlatformInfo } from '../types/api'

async function fetchPlatforms(): Promise<ConnectorPlatformInfo[]> {
  try {
    return await connectorService.getPlatforms()
  } catch {
    return connectorService.getHealth()
  }
}

export function useConnectorPlatforms() {
  return useQuery({
    queryKey: queryKeys.connector.platforms,
    queryFn: fetchPlatforms,
    staleTime: staleTimes.connectorPlatforms,
  })
}

export function useConnectorBindings() {
  return useQuery({
    queryKey: queryKeys.connector.bindings,
    queryFn: async () => {
      const list = await connectorService.listBindings()
      return (list || []).map((b) => ({
        ...b,
        id: String(b.id),
        couponId: String(b.couponId),
      }))
    },
    staleTime: staleTimes.connectorBindings,
  })
}

export function useCreateBinding() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: connectorService.createOrUpdateBinding,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.connector.bindings })
    },
  })
}

export function useSyncOneBinding() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, force }: { id: string; force: boolean }) =>
      connectorService.syncOne(id, force),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.connector.bindings })
    },
  })
}

export function useSyncAllBindings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (force: boolean) => connectorService.syncAll(force),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.connector.bindings })
    },
  })
}
