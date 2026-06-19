import type { AIFeatureCapability, UserAIFeatureBinding, UserAIProviderProfile, UserAISettings } from '@/types'

export const AI_PROFILE_STORAGE_KEY = 'sealchat_user_ai_profiles_v2'
export const AI_SETTINGS_STORAGE_KEY = 'sealchat_user_ai_settings_v3'
const AI_MODEL_DISCOVERY_TIMEOUT_MS = 15000

const normalizeFeatureBinding = (binding?: Partial<UserAIFeatureBinding> | null): UserAIFeatureBinding | null => {
  const providerId = String(binding?.providerId || '').trim()
  const model = String(binding?.model || '').trim()
  if (!providerId || !model) return null
  return { providerId, model }
}

const normalizeProfile = (profile: Partial<UserAIProviderProfile>, index: number): UserAIProviderProfile => ({
  id: String(profile.id || '').trim() || `user-ai-${Date.now().toString(36)}-${index}`,
  name: String(profile.name || '').trim(),
  enabled: profile.enabled !== false,
  baseUrl: String(profile.baseUrl || '').trim(),
  apiKey: String(profile.apiKey || ''),
  models: Array.isArray(profile.models)
    ? profile.models.map((item) => String(item || '').trim()).filter(Boolean)
    : [],
  selectedModel: String(profile.selectedModel || '').trim(),
  hasApiKey: String(profile.apiKey || '').trim().length > 0 || profile.hasApiKey === true,
})

const normalizeProfiles = (items: UserAIProviderProfile[]): UserAIProviderProfile[] => (
  Array.isArray(items) ? items.map((item, index) => normalizeProfile(item, index)) : []
)

const buildDefaultFeatureBindings = (profiles: UserAIProviderProfile[]): Record<string, UserAIFeatureBinding> => {
  const profile = profiles.find((item) => item.enabled)
  const model = String(profile?.selectedModel || profile?.models?.[0] || '').trim()
  if (!profile || !model) return {}
  return {
    polish: { providerId: profile.id, model },
    battle_summary: { providerId: profile.id, model },
  }
}

const safeLocalStorage = () => {
  if (typeof window === 'undefined') return null
  try {
    return window.localStorage
  } catch {
    return null
  }
}

const normalizeBaseUrl = (value: string) => value.replace(/\/+$/, '')

export function readLocalAIProfiles(): UserAIProviderProfile[] {
  const storage = safeLocalStorage()
  if (!storage) return []
  try {
    const raw = storage.getItem(AI_PROFILE_STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.map((item, index) => normalizeProfile(item, index))
  } catch {
    return []
  }
}

export function writeLocalAIProfiles(items: UserAIProviderProfile[]): UserAIProviderProfile[] {
  const current = readLocalAISettings()
  const normalized = normalizeProfiles(items)
  writeLocalAISettings({
    profiles: normalized,
    featureBindings: current.featureBindings,
  })
  return normalized
}

export function readLocalAISettings(): UserAISettings {
  const storage = safeLocalStorage()
  if (!storage) return { profiles: [], featureBindings: {} }
  try {
    const raw = storage.getItem(AI_SETTINGS_STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw)
      const profiles = normalizeProfiles(parsed?.profiles || [])
      const featureBindings: Record<string, UserAIFeatureBinding> = {}
      Object.entries(parsed?.featureBindings || {}).forEach(([featureKey, binding]) => {
        const normalized = normalizeFeatureBinding(binding as Partial<UserAIFeatureBinding>)
        if (normalized) featureBindings[String(featureKey).trim()] = normalized
      })
      return { profiles, featureBindings }
    }
  } catch {
    // fall through to v2 migration
  }
  const profiles = readLocalAIProfiles()
  return {
    profiles,
    featureBindings: buildDefaultFeatureBindings(profiles),
  }
}

export function writeLocalAISettings(settings: UserAISettings): UserAISettings {
  const normalized: UserAISettings = {
    profiles: normalizeProfiles(settings.profiles),
    featureBindings: {},
  }
  Object.entries(settings.featureBindings || {}).forEach(([featureKey, binding]) => {
    const normalizedBinding = normalizeFeatureBinding(binding)
    if (normalizedBinding) normalized.featureBindings[String(featureKey).trim()] = normalizedBinding
  })
  const storage = safeLocalStorage()
  if (storage) {
    try {
      storage.setItem(AI_SETTINGS_STORAGE_KEY, JSON.stringify(normalized))
      storage.setItem(AI_PROFILE_STORAGE_KEY, JSON.stringify(normalized.profiles))
    } catch {
      // ignore localStorage failure in private mode or restricted env
    }
  }
  return normalized
}

export async function discoverLocalAIModels(baseUrl: string, apiKey: string): Promise<string[]> {
  const normalizedBaseUrl = normalizeBaseUrl(String(baseUrl || '').trim())
  if (!normalizedBaseUrl) {
    throw new Error('AI Base URL 不能为空')
  }
  if (!String(apiKey || '').trim()) {
    throw new Error('请先填写本地 AI 的 API Key')
  }
  const response = await fetch(`${normalizedBaseUrl}/models`, {
    method: 'GET',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${String(apiKey || '').trim()}`,
    },
    signal: AbortSignal.timeout(AI_MODEL_DISCOVERY_TIMEOUT_MS),
  })
  let payload: any = null
  try {
    payload = await response.json()
  } catch {
    payload = null
  }
  if (!response.ok) {
    const message = payload?.error?.message || payload?.message || `模型列表请求失败(${response.status})`
    throw new Error(message)
  }
  const models = Array.isArray(payload?.data) ? payload.data : []
  const seen = new Set<string>()
  return models
    .map((item: any) => String(item?.id || '').trim())
    .filter((item: string) => {
      if (!item || seen.has(item)) return false
      seen.add(item)
      return true
    })
}

export function getActiveLocalAIProfile(profiles: UserAIProviderProfile[]): UserAIProviderProfile {
  const profile = profiles.find((item) => item.enabled)
  if (!profile) {
    throw new Error('请先在 AI 设置中启用至少一个本地 API 配置')
  }
  if (!profile.baseUrl.trim()) {
    throw new Error('AI Base URL 不能为空')
  }
  if (!profile.apiKey?.trim()) {
    throw new Error('请先填写本地 AI 的 API Key')
  }
  if (!Array.isArray(profile.models) || profile.models.length === 0 || !String(profile.models[0] || '').trim()) {
    throw new Error('请先填写至少一个可用模型')
  }
  return profile
}

export function resolveLocalAIProfileForFeature(options: {
  featureKey: string
  profiles: UserAIProviderProfile[]
  featureBindings?: Record<string, UserAIFeatureBinding>
}): { profile: UserAIProviderProfile; model: string } {
  const featureKey = String(options.featureKey || '').trim()
  const binding = normalizeFeatureBinding(options.featureBindings?.[featureKey])
  const enabledProfiles = options.profiles.filter((item) => item.enabled)
  if (binding) {
    const profile = enabledProfiles.find((item) => item.id === binding.providerId)
    if (!profile) {
      throw new Error('当前 AI 功能绑定的 API 配置不可用')
    }
    if (!profile.baseUrl.trim()) {
      throw new Error('AI Base URL 不能为空')
    }
    if (!profile.apiKey?.trim()) {
      throw new Error('请先填写本地 AI 的 API Key')
    }
    return { profile, model: binding.model }
  }
  const profile = getActiveLocalAIProfile(options.profiles)
  const model = String(profile.selectedModel || profile.models[0] || '').trim()
  if (!model) {
    throw new Error('未找到可用模型')
  }
  return { profile, model }
}

export async function runLocalAIChat(options: {
  featureKey: string
  input: string
  feature?: AIFeatureCapability | null
  profiles: UserAIProviderProfile[]
  featureBindings?: Record<string, UserAIFeatureBinding>
}): Promise<{ result: string; model: string; providerId: string }> {
  const feature = options.feature
  if (!feature?.enabled) {
    throw new Error('AI 功能未启用')
  }

  const input = String(options.input || '').trim()
  if (!input) {
    throw new Error('请输入需要处理的内容')
  }

  const { profile, model } = resolveLocalAIProfileForFeature({
    featureKey: options.featureKey,
    profiles: options.profiles,
    featureBindings: options.featureBindings,
  })
  const baseUrl = normalizeBaseUrl(profile.baseUrl.trim())

  const response = await fetch(`${baseUrl}/chat/completions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${profile.apiKey.trim()}`,
    },
    body: JSON.stringify({
      model,
      messages: [
        { role: 'system', content: String(feature.defaultPrompt || '').trim() },
        { role: 'user', content: input },
      ],
      temperature: feature.params?.temperature,
      top_p: feature.params?.topP,
      max_tokens: feature.params?.maxTokens,
    }),
  })

  let payload: any = null
  try {
    payload = await response.json()
  } catch {
    payload = null
  }

  if (!response.ok) {
    const message = payload?.error?.message || payload?.message || `AI 请求失败(${response.status})`
    throw new Error(message)
  }

  const result = String(payload?.choices?.[0]?.message?.content || '').trim()
  if (!result) {
    throw new Error('AI 未返回有效结果')
  }

  return {
    result,
    model: String(payload?.model || model),
    providerId: profile.id,
  }
}
