import type { RoleSpeechConfig, SpeechRequest, SpeechVoice, SystemVoice } from './types'

// Front-end catalog model. API types stay unchanged; the adapters below are the
// only place that knows how `VoiceDirectory.system` / `.items` map onto it.
export type VoiceOwnership = 'platform' | 'mine' | 'public'
export type VoiceVisibility = 'system' | 'private' | 'public'
export type VoiceSelection =
  | { type: 'inherit' }
  // System ids are model-specific. modelId/providerId are picker-side metadata;
  // a selection restored from RoleSpeechConfig.systemVoice carries only the id.
  | { type: 'system'; id: string; modelId?: string; providerId?: string }
  | { type: 'personal'; id: string }
export type VoiceCategory = 'all' | 'platform' | 'mine' | 'public'

export interface VoiceCatalogItem {
  key: string
  id: string
  source: 'system' | 'personal'
  providerId?: string
  modelId: string
  name: string
  description?: string
  tags: string[]
  languages: string[]
  kind: string
  ownership: VoiceOwnership
  visibility: VoiceVisibility
  available: boolean
  previewResourceId?: string
}

// Display registry. It describes how the catalog is presented, never which
// voices exist. Option lists are always derived from returned data; these maps
// only label known values and fall back to the raw value otherwise.
export interface SpeechProviderCatalogMeta {
  id: string
  label: string
}
// Keyed by the `providerId` carried on voice data. The directory currently
// returns admin-assigned provider ids for personal voices only and no provider
// for system voices, so no entry is registered yet; unknown ids show verbatim.
export const speechProviderCatalog: Readonly<Record<string, SpeechProviderCatalogMeta>> = {}
export const voiceCategoryOptions: ReadonlyArray<{ value: VoiceCategory; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'platform', label: '平台预设' },
  { value: 'mine', label: '我的音色' },
  { value: 'public', label: '公开音色' },
]
const kindLabels: Readonly<Record<string, string>> = { basic: '基础', system: '系统', design: '声音设计', clone: '样本复刻' }
const languageLabels: Readonly<Record<string, string>> = { zh: '中文', en: '英文' }
export const voiceKindLabel = (kind: string) => kindLabels[kind] ?? kind
export const voiceLanguageLabel = (code: string) => languageLabels[code] ?? code
export const voiceProviderLabel = (id: string) => speechProviderCatalog[id]?.label ?? id

export function voiceSourceLabel(item: VoiceCatalogItem): string {
  if (item.ownership === 'platform') return '平台预设'
  if (item.ownership === 'mine') return item.visibility === 'public' ? '我的 · 公开' : '我的 · 私有'
  return '公开音色'
}

function splitTags(value: string | undefined): string[] {
  return (value ?? '').split(/[,，、/|]+/).map(tag => tag.trim()).filter(Boolean)
}

// Some provider catalogs mix age numbers into free-form tags. They are not
// useful as picker facets; keep descriptive text tags and omit bare ages.
export function isDisplayVoiceTag(value: string): boolean {
  const text = value.trim()
  return !!text && !/^\d+(?:\.\d+)?(?:\s*(?:岁|years?|yrs?))?$/i.test(text)
}

export function systemVoiceItem(voice: SystemVoice): VoiceCatalogItem {
  return {
    key: `system:${voice.providerId ?? ''}:${voice.targetModel}:${voice.id}`, id: voice.id, source: 'system',
    providerId: voice.providerId || undefined, modelId: voice.targetModel, name: voice.name,
    tags: splitTags(voice.tags), languages: voice.languages ?? [], kind: voice.kind,
    ownership: 'platform', visibility: 'system', available: true,
  }
}

// Only saved voices belong in the catalog; previews and creating voices are
// managed from the personal workbench, not selected.
export function personalVoiceItem(voice: SpeechVoice, userId: string): VoiceCatalogItem {
  return {
    key: `personal:${voice.id}`, id: voice.id, source: 'personal', providerId: voice.providerId || undefined,
    modelId: voice.targetModel, name: voice.name, description: voice.description || undefined,
    tags: splitTags(voice.tags), languages: [], kind: voice.kind,
    ownership: voice.ownerUserId === userId ? 'mine' : 'public',
    visibility: voice.isPublic ? 'public' : 'private',
    available: voice.lifecycle === 'saved' && voice.providerStatus === 'OK',
    previewResourceId: voice.previewResourceId || undefined,
  }
}

export function sameSelection(a: VoiceSelection, b: VoiceSelection): boolean {
  if (a.type === 'inherit' || b.type === 'inherit') return a.type === b.type
  if (a.type === 'system' && b.type === 'system') {
    return a.id === b.id && (!a.modelId || !b.modelId || a.modelId === b.modelId)
      && (!a.providerId || !b.providerId || a.providerId === b.providerId)
  }
  return a.type === b.type && a.id === b.id
}
// A system selection without modelId/providerId is a legacy persisted binding
// and matches by id within the presets the caller allows.
export function selectsItem(selection: VoiceSelection, item: VoiceCatalogItem): boolean {
  if (selection.type === 'inherit' || selection.type !== item.source || selection.id !== item.id) return false
  if (selection.type !== 'system') return true
  return (!selection.modelId || selection.modelId === item.modelId)
    && (!selection.providerId || selection.providerId === item.providerId)
}
export function itemSelection(item: VoiceCatalogItem): VoiceSelection {
  return item.source === 'system'
    ? { type: 'system', id: item.id, modelId: item.modelId, providerId: item.providerId }
    : { type: 'personal', id: item.id }
}

// RoleSpeechConfig keeps two fields; at most one of them may carry a value.
export function roleVoiceSelection(role: Pick<RoleSpeechConfig, 'voiceId' | 'systemVoice'>): VoiceSelection {
  if (role.voiceId) return { type: 'personal', id: role.voiceId }
  if (role.systemVoice) return { type: 'system', id: role.systemVoice }
  return { type: 'inherit' }
}
export function roleVoiceFields(selection: VoiceSelection): Pick<RoleSpeechConfig, 'voiceId' | 'systemVoice'> {
  if (selection.type === 'personal') return { voiceId: selection.id, systemVoice: '' }
  if (selection.type === 'system') return { voiceId: '', systemVoice: selection.id }
  return { voiceId: '', systemVoice: '' }
}
// Job requests carry either an explicit personal voice or a system voice, with
// the platform default standing in for "inherit".
export function requestVoiceFields(selection: VoiceSelection, defaultVoice: string | undefined): Pick<SpeechRequest, 'voiceId' | 'systemVoice'> {
  if (selection.type === 'personal') return { voiceId: selection.id }
  return { systemVoice: selection.type === 'system' ? selection.id : defaultVoice }
}

export function matchesCategory(item: VoiceCatalogItem, category: VoiceCategory): boolean {
  if (category === 'platform') return item.ownership === 'platform'
  if (category === 'mine') return item.ownership === 'mine'
  if (category === 'public') return item.visibility === 'public'
  return true
}
// Personal voices are searched by the server; system voices are only returned
// as a whole list, so the same name/tag match is applied locally to them.
export function matchesSearch(item: VoiceCatalogItem, search: string): boolean {
  const text = search.trim().toLowerCase()
  if (!text) return true
  return item.name.toLowerCase().includes(text) || item.tags.some(tag => tag.toLowerCase().includes(text))
}

export interface VoiceFacetOption { value: string; label: string; count: number }
export interface VoiceFacet { key: VoiceFacetKey; label: string; options: VoiceFacetOption[] }
export type VoiceFacetKey = 'providerId' | 'language' | 'kind' | 'tag'
export type VoiceFacetFilters = Record<VoiceFacetKey, string>
export const emptyVoiceFilters = (): VoiceFacetFilters => ({ providerId: '', language: '', kind: '', tag: '' })

function facetValues(item: VoiceCatalogItem, key: VoiceFacetKey): string[] {
  if (key === 'providerId') return item.providerId ? [item.providerId] : []
  if (key === 'language') return item.languages
  if (key === 'kind') return item.kind ? [item.kind] : []
  return item.tags.filter(isDisplayVoiceTag)
}
const facetMeta: ReadonlyArray<{ key: VoiceFacetKey; label: string; format: (value: string) => string; min: number }> = [
  // `min` hides dimensions whose returned values cannot narrow anything.
  { key: 'providerId', label: '服务', format: voiceProviderLabel, min: 2 },
  { key: 'language', label: '语言', format: voiceLanguageLabel, min: 2 },
  { key: 'kind', label: '类型', format: voiceKindLabel, min: 2 },
  { key: 'tag', label: '标签', format: value => value, min: 1 },
]

// Facets are aggregated from the items actually returned; empty dimensions are omitted.
export function collectVoiceFacets(items: VoiceCatalogItem[]): VoiceFacet[] {
  return facetMeta.flatMap(({ key, label, format, min }) => {
    const counts = new Map<string, number>()
    for (const item of items) for (const value of new Set(facetValues(item, key))) counts.set(value, (counts.get(value) ?? 0) + 1)
    if (counts.size < min) return []
    const options = [...counts].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([value, count]) => ({ value, label: format(value), count }))
    return [{ key, label, options }]
  })
}
export function matchesVoiceFilters(item: VoiceCatalogItem, filters: VoiceFacetFilters): boolean {
  return (Object.keys(filters) as VoiceFacetKey[]).every(key => !filters[key] || facetValues(item, key).includes(filters[key]))
}
