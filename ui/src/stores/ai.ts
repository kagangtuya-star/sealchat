import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { AIFeatureCapability, AIRunSource, UserAIFeatureBinding, UserAIProviderProfile, UserAISettings } from '@/types'
import { api, buildAuthorizedHeaders, urlBase } from '@/stores/_config'
import { useUserStore } from '@/stores/user'
import { discoverLocalAIModels, readLocalAISettings, runLocalAIChat, writeLocalAISettings } from '@/services/ai/local-ai'
import { persistAccessToken } from '@/utils/authToken'

const AI_SOURCE_STORAGE_KEY = 'sealchat_ai_source_v1'
export const USER_AI_SETTINGS_REQUIRED_MESSAGE = '请先在个人信息的 AI 设置中配置个人 API 后再调用'

export const isUserAISettingsRequiredMessage = (value: unknown) => {
  const message = String(value || '')
  return message.includes(USER_AI_SETTINGS_REQUIRED_MESSAGE)
    || message.includes('仅允许用户自定义调用')
    || message.includes('ai user custom provider required')
}

const normalizeSource = (value?: string | null): AIRunSource => (value === 'user' ? 'user' : 'platform')

const readStoredSource = (): AIRunSource => {
  if (typeof window === 'undefined') {
    return 'platform'
  }
  try {
    return normalizeSource(window.localStorage.getItem(AI_SOURCE_STORAGE_KEY))
  } catch {
    return 'platform'
  }
}

const persistSource = (value: AIRunSource) => {
  if (typeof window === 'undefined') {
    return
  }
  try {
    window.localStorage.setItem(AI_SOURCE_STORAGE_KEY, value)
  } catch {
    // ignore storage failure
  }
}

interface AIStreamEvent {
  event: string
  data: string
}

const parseAIStreamEvents = (raw: string): AIStreamEvent[] => {
  const events: AIStreamEvent[] = []
  let event = 'message'
  let dataLines: string[] = []
  const flush = () => {
    if (dataLines.length === 0) {
      event = 'message'
      return
    }
    events.push({ event, data: dataLines.join('\n') })
    event = 'message'
    dataLines = []
  }
  for (const line of raw.split(/\r?\n/)) {
    if (line === '') {
      flush()
      continue
    }
    if (line.startsWith(':')) continue
    const separator = line.indexOf(':')
    const field = separator >= 0 ? line.slice(0, separator) : line
    let value = separator >= 0 ? line.slice(separator + 1) : ''
    if (value.startsWith(' ')) value = value.slice(1)
    if (field === 'event') event = value
    if (field === 'data') dataLines.push(value)
  }
  flush()
  return events
}

const readAIResponseError = async (response: Response, raw: string) => {
  try {
    const payload = JSON.parse(raw) as { message?: string; error?: { message?: string } }
    const message = payload?.message || payload?.error?.message
    if (message) return String(message)
  } catch {
    // fall through to status text
  }
  return response.statusText || `AI 请求失败（${response.status}）`
}

const runPlatformAITask = async (featureKey: string, payload: { worldId?: string; channelId?: string; input: string; source: AIRunSource }, token: string) => {
  const response = await fetch(`${urlBase}/api/v1/ai/tasks/${encodeURIComponent(featureKey)}?stream=1`, {
    method: 'POST',
    credentials: 'include',
    headers: buildAuthorizedHeaders({
      Authorization: token,
      'Content-Type': 'application/json',
    }),
    body: JSON.stringify(payload),
  })
  const refreshedToken = response.headers.get('x-access-token-refresh')
  if (refreshedToken?.trim()) {
    persistAccessToken(refreshedToken)
  }
  const raw = await response.text()
  const contentType = response.headers.get('content-type') || ''
  if (!contentType.toLowerCase().includes('text/event-stream')) {
    throw new Error(await readAIResponseError(response, raw))
  }
  const events = parseAIStreamEvents(raw)
  const errorEvent = events.find((item) => item.event === 'error')
  if (errorEvent) {
    let message = 'AI 请求失败'
    try {
      const payload = JSON.parse(errorEvent.data) as { message?: string }
      message = payload.message || message
    } catch {
      // keep the generic message for malformed error events
    }
    throw new Error(message)
  }
  const resultEvent = events.find((item) => item.event === 'result')
  if (!resultEvent) {
    throw new Error(response.ok ? 'AI 响应缺少结果' : await readAIResponseError(response, raw))
  }
  return { data: JSON.parse(resultEvent.data) as { featureKey: string; result: string; model: string; providerId: string; warning?: string } }
}

export const useAIStore = defineStore('ai', () => {
  const loading = ref(false)
  const profileLoading = ref(false)
  const features = ref<Record<string, AIFeatureCapability>>({})
  const currentSource = ref<AIRunSource>(readStoredSource())
  const initialUserAISettings = readLocalAISettings()
  const userProfiles = ref<UserAIProviderProfile[]>(initialUserAISettings.profiles)
  const userFeatureBindings = ref<Record<string, UserAIFeatureBinding>>(initialUserAISettings.featureBindings)

  async function loadCapabilities(worldId?: string) {
    const user = useUserStore()
    loading.value = true
    try {
      const resp = await api.get('api/v1/ai/capabilities', {
        headers: { Authorization: user.token },
        params: worldId ? { worldId } : undefined,
      })
      const items = (resp.data?.features || []) as AIFeatureCapability[]
      features.value = items.reduce<Record<string, AIFeatureCapability>>((acc, item) => {
        acc[item.key] = {
          ...item,
          userCustomOnly: item?.userCustomOnly === true,
        }
        return acc
      }, Object.create(null))
      return resp
    } catch {
      features.value = {}
      throw new Error('加载 AI 能力失败')
    } finally {
      loading.value = false
    }
  }

  function isFeatureEnabled(featureKey: string) {
    return features.value[featureKey]?.enabled === true
  }

  function getFeatureCapability(featureKey: string) {
    return features.value[featureKey] || null
  }

  function resolveEffectiveSource(featureKey: string, requestedSource?: 'platform' | 'user'): AIRunSource {
    const feature = getFeatureCapability(featureKey)
    if (feature?.userCustomOnly) {
      return 'user'
    }
    return requestedSource || currentSource.value
  }

  function setSource(source: AIRunSource) {
    currentSource.value = normalizeSource(source)
    persistSource(currentSource.value)
  }

  async function loadUserProfiles() {
    profileLoading.value = true
    try {
      const settings = readLocalAISettings()
      userProfiles.value = settings.profiles
      userFeatureBindings.value = settings.featureBindings
      return userProfiles.value
    } finally {
      profileLoading.value = false
    }
  }

  async function saveUserProfiles(items: UserAIProviderProfile[]) {
    return saveUserAISettings({
      profiles: items,
      featureBindings: userFeatureBindings.value,
    })
  }

  async function loadUserAISettings() {
    profileLoading.value = true
    try {
      const settings = readLocalAISettings()
      userProfiles.value = settings.profiles
      userFeatureBindings.value = settings.featureBindings
      return settings
    } finally {
      profileLoading.value = false
    }
  }

  async function saveUserAISettings(settings: UserAISettings) {
    profileLoading.value = true
    try {
      const normalized = writeLocalAISettings(settings)
      userProfiles.value = normalized.profiles
      userFeatureBindings.value = normalized.featureBindings
      return normalized
    } finally {
      profileLoading.value = false
    }
  }

  async function discoverUserProfileModels(baseUrl: string, apiKey: string) {
    return discoverLocalAIModels(baseUrl, apiKey)
  }

  async function runTask(featureKey: string, payload: { worldId?: string; channelId?: string; input: string; source?: 'platform' | 'user' }) {
    const source = resolveEffectiveSource(featureKey, payload.source)
    if (source === 'user') {
      if (!userProfiles.value.some((item) => item.enabled && item.baseUrl.trim() && item.apiKey?.trim() && Array.isArray(item.models) && item.models.some((model) => String(model || '').trim()))) {
        throw new Error(USER_AI_SETTINGS_REQUIRED_MESSAGE)
      }
      return {
        data: await runLocalAIChat({
          featureKey,
          input: payload.input,
          feature: getFeatureCapability(featureKey),
          profiles: userProfiles.value,
          featureBindings: userFeatureBindings.value,
        }),
      }
    }
    const user = useUserStore()
    return runPlatformAITask(featureKey, {
      ...payload,
      source,
    }, user.token)
  }

  const enabledFeatureKeys = computed(() => Object.keys(features.value).filter((key) => features.value[key]?.enabled))
  const hasEnabledUserProfile = computed(() => userProfiles.value.some((item) => item.enabled))

  return {
    loading,
    profileLoading,
    features,
    enabledFeatureKeys,
    currentSource,
    userProfiles,
    userFeatureBindings,
    hasEnabledUserProfile,
    loadCapabilities,
    isFeatureEnabled,
    getFeatureCapability,
    setSource,
    resolveEffectiveSource,
    loadUserProfiles,
    saveUserProfiles,
    loadUserAISettings,
    saveUserAISettings,
    discoverUserProfileModels,
    runTask,
  }
})
