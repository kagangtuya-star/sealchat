import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { AIFeatureCapability, AIRunSource, UserAIProviderProfile } from '@/types'
import { api } from '@/stores/_config'
import { useUserStore } from '@/stores/user'
import { discoverLocalAIModels, readLocalAIProfiles, runLocalAIChat, writeLocalAIProfiles } from '@/services/ai/local-ai'

const AI_SOURCE_STORAGE_KEY = 'sealchat_ai_source_v1'
const PLATFORM_AI_TASK_TIMEOUT_MS = 120000

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

export const useAIStore = defineStore('ai', () => {
  const loading = ref(false)
  const profileLoading = ref(false)
  const features = ref<Record<string, AIFeatureCapability>>({})
  const currentSource = ref<AIRunSource>(readStoredSource())
  const userProfiles = ref<UserAIProviderProfile[]>(readLocalAIProfiles())

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
        acc[item.key] = item
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

  function setSource(source: AIRunSource) {
    currentSource.value = normalizeSource(source)
    persistSource(currentSource.value)
  }

  async function loadUserProfiles() {
    profileLoading.value = true
    try {
      userProfiles.value = readLocalAIProfiles()
      return userProfiles.value
    } finally {
      profileLoading.value = false
    }
  }

  async function saveUserProfiles(items: UserAIProviderProfile[]) {
    profileLoading.value = true
    try {
      userProfiles.value = writeLocalAIProfiles(items)
      return userProfiles.value
    } finally {
      profileLoading.value = false
    }
  }

  async function discoverUserProfileModels(baseUrl: string, apiKey: string) {
    return discoverLocalAIModels(baseUrl, apiKey)
  }

  async function runTask(featureKey: string, payload: { worldId?: string; channelId?: string; input: string; source?: 'platform' | 'user' }) {
    const source = payload.source || currentSource.value
    if (source === 'user') {
      return {
        data: await runLocalAIChat({
          featureKey,
          input: payload.input,
          feature: getFeatureCapability(featureKey),
          profiles: userProfiles.value,
        }),
      }
    }
    const user = useUserStore()
    return api.post(`api/v1/ai/tasks/${featureKey}`, {
      ...payload,
      source,
    }, {
      headers: { Authorization: user.token },
      timeout: PLATFORM_AI_TASK_TIMEOUT_MS,
    })
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
    hasEnabledUserProfile,
    loadCapabilities,
    isFeatureEnabled,
    getFeatureCapability,
    setSource,
    loadUserProfiles,
    saveUserProfiles,
    discoverUserProfileModels,
    runTask,
  }
})
