import type { AIFeatureCapability, UserAIProviderProfile } from '@/types'

export const AI_PROFILE_STORAGE_KEY = 'sealchat_user_ai_profiles_v2'
const AI_MODEL_DISCOVERY_TIMEOUT_MS = 15000

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
  const normalized = items.map((item, index) => normalizeProfile(item, index))
  const storage = safeLocalStorage()
  if (storage) {
    try {
      storage.setItem(AI_PROFILE_STORAGE_KEY, JSON.stringify(normalized))
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

export async function runLocalAIChat(options: {
  featureKey: string
  input: string
  feature?: AIFeatureCapability | null
  profiles: UserAIProviderProfile[]
}): Promise<{ result: string; model: string; providerId: string }> {
  const feature = options.feature
  if (!feature?.enabled) {
    throw new Error('AI 功能未启用')
  }

  const input = String(options.input || '').trim()
  if (!input) {
    throw new Error('请输入需要处理的内容')
  }

  const profile = getActiveLocalAIProfile(options.profiles)
  const baseUrl = normalizeBaseUrl(profile.baseUrl.trim())
  const model = String(profile.selectedModel || profile.models[0] || feature.defaultModel || '').trim()
  if (!model) {
    throw new Error('未找到可用模型')
  }

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
