import type { ResolvedSpeechModel, RoleSpeechConfig, SpeechPresetSource, SpeechProviderMeta, SpeechRequest, SpeechVoice, SystemVoice, VoiceContext } from './types'

// Adapters keep the provider/model context shared by all voice selectors.
export type VoiceOwnership = 'platform' | 'mine' | 'public'
export type VoiceVisibility = 'system' | 'private' | 'public'
export type VoiceSelection =
  | { type: 'inherit' }
  | { type: 'system'; id: string; modelId?: string; providerKind?: string; providerId?: string }
  | { type: 'personal'; id: string }
export type VoiceSourceKey = 'all' | 'mine' | 'public' | `system:${string}`

export interface VoiceCatalogItem {
  key: string
  id: string
  source: 'system' | 'personal'
  presetSource: string
  providerId?: string
  providerKind?: string
  models: string[]
  modelId: string
  name: string
  description?: string
  tags: string[]
  languages: string[]
  speechLanguages: string[]
  kind: string
  ownership: VoiceOwnership
  visibility: VoiceVisibility
  available: boolean
  previewResourceId?: string
}

// Display registry. It describes how the catalog is presented, never which
// voices exist. Option lists are always derived from returned data; these maps
// only label known values and fall back to the raw value otherwise.
export function voiceSourceOptions(presetSources: SpeechPresetSource[]): Array<{ value: VoiceSourceKey; label: string }> {
  return [
    { value: 'all', label: '全部' },
    ...presetSources.map(source => ({ value: `system:${source.key}` as VoiceSourceKey, label: source.label })),
    { value: 'mine', label: '我的音色' },
    { value: 'public', label: '公开音色' },
  ]
}
const kindLabels: Readonly<Record<string, string>> = { basic: '基础', system: '系统', classic: '精品音色', large: '大模型音色', design: '声音设计', clone: '样本复刻' }
const languageLabels: Readonly<Record<string, string>> = { zh: '中文', en: '英文' }
export const voiceKindLabel = (kind: string) => kindLabels[kind] ?? kind
export const voiceLanguageLabel = (code: string) => languageLabels[code] ?? code

const speechLanguageLabels: Readonly<Record<string, string>> = {
  zh: '中文（中文）',
  en: '英语（English）',
  ja: '日语（日本語）',
  ko: '韩语（한국어）',
  yue: '粤语（粤语）',
  fr: '法语（Français）',
  de: '德语（Deutsch）',
  pt: '葡萄牙语（Português）',
  it: '意大利语（Italiano）',
  vi: '越南语（Tiếng Việt）',
  id: '印尼语（Bahasa Indonesia）',
  ru: '俄语（Русский）',
  th: '泰语（ไทย）',
  es: '西班牙语（Español）',
  ms: '马来语（Bahasa Melayu）',
  fil: '菲律宾语（Filipino）',
  ar: '阿拉伯语（العربية）',
}
export function speechLanguageOptions(languages: string[]): Array<{ value: string; label: string }> {
  return [{ value: '', label: '跟随原文' }, ...[...new Set(languages)].filter(Boolean).map(value => ({ value, label: speechLanguageLabels[value] ?? voiceLanguageLabel(value) }))]
}
export function compatibleSpeechLanguage(language: string, languages: string[]): string {
  return languages.includes(language) ? language : ''
}
export function selectedSpeechLanguages(selection: VoiceSelection, voices: SystemVoice[] | null, contexts: VoiceContext[], defaultContext: VoiceContext | null, defaultVoice: string): string[] | null {
  if (selection.type === 'personal') return []
  if (voices === null) return null // Loading must not clear an existing binding.
  if (selection.type === 'inherit') {
    const voice = voices.find(voice => voice.id === defaultVoice && systemVoiceSupported(voice, defaultContext))
    return voice && defaultContext ? systemVoiceItem(voice, defaultContext).speechLanguages : []
  }
  return resolveLegacySystemSelection(selection, voices, contexts, defaultContext)?.speechLanguages ?? []
}

export function providerDisplayName(providers: SpeechProviderMeta[], kind: string): string {
  return providers.find(provider => provider.kind === kind)?.name ?? kind
}

export function voiceSourceLabel(item: VoiceCatalogItem, providers: SpeechProviderMeta[], presetSources: SpeechPresetSource[] = []): string {
  if (item.source === 'system') return presetSources.find(source => source.key === item.presetSource)?.label ?? `${providerDisplayName(providers, item.providerKind ?? '')}预设`
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

export function systemVoiceItem(voice: SystemVoice, context: VoiceContext): VoiceCatalogItem {
  return {
    key: `system:${voice.providerKind}:${context.providerId}:${context.modelId}:${voice.id}`, id: voice.id, source: 'system',
    presetSource: voice.presetSource || voice.providerKind,
    providerKind: context.providerKind, providerId: context.providerId, models: voice.models, modelId: context.modelId, name: voice.name,
    tags: splitTags(voice.tags), languages: voice.languages ?? [], kind: voice.kind,
    speechLanguages: voice.speechLanguages?.length ? voice.speechLanguages : voice.languages ?? [],
    ownership: 'platform', visibility: 'system', available: true,
  }
}

// Only saved voices belong in the catalog; previews and creating voices are
// managed from the personal workbench, not selected.
export function personalVoiceItem(voice: SpeechVoice, userId: string): VoiceCatalogItem {
  return {
    key: `personal:${voice.id}`, id: voice.id, source: 'personal', providerId: voice.providerId || undefined,
    presetSource: '',
    models: [voice.targetModel], modelId: voice.targetModel, name: voice.name, description: voice.description || undefined,
    tags: splitTags(voice.tags), languages: [], kind: voice.kind,
    speechLanguages: [],
    ownership: voice.ownerUserId === userId ? 'mine' : 'public',
    visibility: voice.isPublic ? 'public' : 'private',
    available: voice.lifecycle === 'saved' && voice.providerStatus === 'OK' && voice.supported !== false,
    previewResourceId: voice.previewResourceId || undefined,
  }
}

export function systemVoiceSupported(voice: SystemVoice, context: VoiceContext | null): boolean {
  return !!context && voice.providerKind === context.providerKind && voice.models.includes(context.modelId)
}
export function resolveSystemVoiceContext(voice: SystemVoice, contexts: VoiceContext[], defaultContext: VoiceContext | null): VoiceContext | null {
  if (systemVoiceSupported(voice, defaultContext)) return defaultContext
  return contexts.find(context => systemVoiceSupported(voice, context)) ?? null
}
// Resolve older bindings for display without changing the selection or role.
export function resolveLegacySystemSelection(selection: VoiceSelection, voices: SystemVoice[], contexts: VoiceContext[], defaultContext: VoiceContext | null): VoiceCatalogItem | null {
  if (selection.type !== 'system') return null
  const itemForContext = (context: VoiceContext | null): VoiceCatalogItem | null => {
    if (!context) return null
    const voice = voices.find(voice => voice.id === selection.id && systemVoiceSupported(voice, context))
    return voice ? systemVoiceItem(voice, context) : null
  }
  if (selection.providerId) {
    const context = [ ...(defaultContext ? [defaultContext] : []), ...contexts ].find(context =>
      context.providerKind === selection.providerKind && context.providerId === selection.providerId && context.modelId === selection.modelId)
    return itemForContext(context ?? null)
  }
  if (!selection.providerKind && !selection.modelId) {
    const defaultItem = itemForContext(defaultContext)
    if (defaultItem) return defaultItem
    const candidates = contexts.map(itemForContext).filter(item => item !== null)
    return candidates.length === 1 ? candidates[0] : null
  }
  if (!selection.providerKind || !selection.modelId) return null
  const matchesNamespace = (context: VoiceContext) => context.providerKind === selection.providerKind && context.modelId === selection.modelId
  if (defaultContext && matchesNamespace(defaultContext)) {
    const item = itemForContext(defaultContext)
    if (item) return item
  }
  const candidates = contexts.filter(context => matchesNamespace(context) && itemForContext(context))
  return candidates.length === 1 ? itemForContext(candidates[0]) : null
}
export function defaultVoiceForContext(voices: SystemVoice[], models: ResolvedSpeechModel[], context: VoiceContext | null, current: string): string {
  if (!context) return ''
  if (voices.some(voice => voice.id === current && systemVoiceSupported(voice, context))) return current
  return models.find(model => model.providerKind === context.providerKind && model.id === context.modelId)?.defaultVoice ?? ''
}

export function sameSelection(a: VoiceSelection, b: VoiceSelection): boolean {
  if (a.type === 'inherit' || b.type === 'inherit') return a.type === b.type
  if (a.type === 'system' && b.type === 'system') {
    return a.id === b.id && a.modelId === b.modelId && a.providerKind === b.providerKind && a.providerId === b.providerId
  }
  return a.type === b.type && a.id === b.id
}
// Only a completely empty namespace is a legacy UI binding.
export function selectsItem(selection: VoiceSelection, item: VoiceCatalogItem): boolean {
  if (selection.type === 'inherit' || selection.type !== item.source || selection.id !== item.id) return false
  if (selection.type !== 'system') return true
  const legacy = !selection.modelId && !selection.providerKind && !selection.providerId
  return legacy || (selection.modelId === item.modelId && selection.providerKind === item.providerKind && selection.providerId === item.providerId)
}
export function itemSelection(item: VoiceCatalogItem): VoiceSelection {
  return item.source === 'system'
    ? { type: 'system', id: item.id, modelId: item.modelId, providerKind: item.providerKind, providerId: item.providerId }
    : { type: 'personal', id: item.id }
}

type RoleVoiceFields = Pick<RoleSpeechConfig, 'voiceId' | 'systemVoice' | 'systemVoiceProvider' | 'systemVoiceProviderId' | 'systemVoiceModel'>
export function roleVoiceSelection(role: RoleVoiceFields): VoiceSelection {
  if (role.voiceId) return { type: 'personal', id: role.voiceId }
  if (role.systemVoice) return { type: 'system', id: role.systemVoice, providerKind: role.systemVoiceProvider || undefined, providerId: role.systemVoiceProviderId || undefined, modelId: role.systemVoiceModel || undefined }
  return { type: 'inherit' }
}
export function roleVoiceFields(selection: VoiceSelection): RoleVoiceFields {
  const empty = { voiceId: '', systemVoice: '', systemVoiceProvider: '', systemVoiceProviderId: '', systemVoiceModel: '' }
  if (selection.type === 'personal') return { ...empty, voiceId: selection.id }
  if (selection.type === 'system') return { ...empty, systemVoice: selection.id, systemVoiceProvider: selection.providerKind ?? '', systemVoiceProviderId: selection.providerId ?? '', systemVoiceModel: selection.modelId ?? '' }
  return empty
}
export function requestVoiceFields(selection: VoiceSelection): Pick<SpeechRequest, 'voiceId' | 'systemVoice' | 'systemVoiceProvider' | 'systemVoiceProviderId' | 'systemVoiceModel'> {
  if (selection.type === 'personal') return { voiceId: selection.id }
  if (selection.type === 'system') return { systemVoice: selection.id, systemVoiceProvider: selection.providerKind, systemVoiceProviderId: selection.providerId, systemVoiceModel: selection.modelId }
  return {}
}

export function matchesSource(item: VoiceCatalogItem, source: VoiceSourceKey): boolean {
  if (source.startsWith('system:')) return item.source === 'system' && item.presetSource === source.slice('system:'.length)
  if (source === 'mine') return item.ownership === 'mine'
  if (source === 'public') return item.visibility === 'public'
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
export type VoiceFacetKey = 'language' | 'kind' | 'tag'
export type VoiceFacetFilters = Record<VoiceFacetKey, string>
export const emptyVoiceFilters = (): VoiceFacetFilters => ({ language: '', kind: '', tag: '' })

function facetValues(item: VoiceCatalogItem, key: VoiceFacetKey): string[] {
  if (key === 'language') return item.languages
  if (key === 'kind') return item.kind ? [item.kind] : []
  return item.tags.filter(isDisplayVoiceTag)
}
const facetMeta: ReadonlyArray<{ key: VoiceFacetKey; label: string; format: (value: string) => string; min: number }> = [
  // `min` hides dimensions whose returned values cannot narrow anything.
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
