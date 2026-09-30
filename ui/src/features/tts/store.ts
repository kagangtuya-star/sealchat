import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { clearSpeechSubmissionKeys, speechAPI } from './api'
import { speechPlayer } from './player'
import { useUserStore } from '@/stores/user'
import type { MessageSpeech, SpeechQuota } from './types'

export const useSpeechStore = defineStore('tts', () => {
  const visible = ref(false)
  const scopeChannel = ref('')
  const messageStates = ref<Record<string, MessageSpeech | null>>({})
  const quota = ref<SpeechQuota | null>(null)
  const canSynthesize = computed(() => !!quota.value?.enabled && (quota.value.pricingMode === 'token'
    ? quota.value.inputTokenPrice != null && quota.value.outputTokenPrice != null
    : quota.value.characterPrice != null))
  const temporary = ref<Record<string, boolean>>({})
  let generation = 0
  function reset() {
    clearSpeechSubmissionKeys()
    generation++
    visible.value = false
    scopeChannel.value = ''
    quota.value = null
    messageStates.value = {}
    temporary.value = {}
    speechPlayer.stop()
  }
  async function refresh() {
    const current = generation
    const result = await speechAPI.me()
    if (current === generation) quota.value = result
  }
  function optIn(channelId: string) {
    return !!channelId && !!quota.value?.enabled && !!quota.value.autoSynthesis && temporary.value[channelId] !== false
  }
  function open(channelId = '') {
    if (window.parent !== window) {
      window.parent.postMessage({ type: 'sealchat:tts-intent', action: 'open', channelId, userId: useUserStore().info.id }, window.location.origin)
    } else {
      if (channelId) scopeChannel.value = channelId
      visible.value = true
    }
  }
  function setTemporary(channelId: string, enabled: boolean) {
    temporary.value[channelId] = enabled
    if (window.parent !== window) window.parent.postMessage({ type: 'sealchat:tts-intent', action: 'temporary', channelId, enabled, userId: useUserStore().info.id }, window.location.origin)
  }
  return { visible, quota, canSynthesize, temporary, messageStates, scopeChannel, reset, refresh, optIn, open, setTemporary }
})
