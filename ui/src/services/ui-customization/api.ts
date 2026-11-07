import { api } from '@/stores/_config'
import { normalizeUICustomizationPolicy } from './types'
import type {
  UICustomizationContext, UICustomizationPackage, UICustomizationPackageInput,
  UICustomizationPolicy, UICustomizationRuntimeResult,
} from './types'

export async function loadUICustomizations(context: UICustomizationContext): Promise<UICustomizationRuntimeResult> {
  const { surface, worldId, channelId, slug } = context
  const response = await api.get<UICustomizationRuntimeResult>('/api/v1/ui-customizations/runtime', {
    params: { surface, worldId, channelId, slug },
  })
  return response.data
}

/** Management target: the platform, or one world managed by its admins. */
export type UICustomizationManageScope = { type: 'platform' } | { type: 'world'; worldId: string }

const base = (scope: UICustomizationManageScope) => scope.type === 'platform'
  ? '/api/v1/admin/ui-customizations'
  : `/api/v1/worlds/${encodeURIComponent(scope.worldId)}/ui-customizations`

export async function listManagedUICustomizations(scope: UICustomizationManageScope) {
  const { data } = await api.get<{ packages: UICustomizationPackage[]; policy?: Partial<UICustomizationPolicy> }>(base(scope))
  return { packages: data.packages || [], policy: normalizeUICustomizationPolicy(data.policy) }
}

export async function createManagedUICustomization(scope: UICustomizationManageScope, input: UICustomizationPackageInput) {
  return (await api.post<UICustomizationPackage>(base(scope), input)).data
}

export async function updateManagedUICustomization(scope: UICustomizationManageScope, id: string, patch: UICustomizationPackageInput) {
  return (await api.patch<UICustomizationPackage>(`${base(scope)}/${encodeURIComponent(id)}`, patch)).data
}

export async function deleteManagedUICustomization(scope: UICustomizationManageScope, id: string) {
  await api.delete(`${base(scope)}/${encodeURIComponent(id)}`)
}

export async function setWorldPlatformUICustomizationEnabled(worldId: string, id: string, enabled: boolean) {
  const url = `/api/v1/worlds/${encodeURIComponent(worldId)}/ui-customizations/platform/${encodeURIComponent(id)}`
  if (enabled) await api.put(url)
  else await api.delete(url)
}

export async function replacePlatformUICustomizationFromImport(id: string, document: unknown) {
  return (await api.post<UICustomizationPackage>(`/api/v1/admin/ui-customizations/${encodeURIComponent(id)}/import`, document)).data
}

export async function reorderManagedUICustomizations(scope: UICustomizationManageScope, ids: string[]) {
  await api.post(`${base(scope)}/reorder`, { ids })
}

/** Sends a parsed sealchat.ui-package / sealchat.ui-bundle document; the server validates it. */
export async function importManagedUICustomizations(scope: UICustomizationManageScope, document: unknown) {
  return (await api.post<{ packages: UICustomizationPackage[] }>(`${base(scope)}/import`, document)).data.packages || []
}
