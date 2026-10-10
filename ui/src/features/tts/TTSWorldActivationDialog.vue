<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NAlert, NButton, NInput } from 'naive-ui'
import { useChatStore } from '@/stores/chat'
import { speechAPI, speechError } from './api'
import { useSpeechStore } from './store'
import TTSWorldOverlay from './TTSWorldOverlay.vue'
const speech = useSpeechStore()
const chat = useChatStore()
const visible = ref(false)
const code = ref('')
const error = ref('')
const busy = ref(false)
const channelId = computed(() => speech.scopeChannel || chat.curChannel?.id || '')
const access = computed(() => speech.quota?.worldAccess)
watch([channelId, () => access.value?.canActivate], () => { visible.value = false; code.value = ''; error.value = '' })
function open() { code.value = ''; error.value = ''; visible.value = true }
async function activate() {
  const worldId = access.value?.worldId
  const channel = channelId.value
  if (busy.value || !worldId || !access.value?.canActivate) return
  if (!code.value.trim()) { error.value = '请输入激活码'; return }
  busy.value = true; error.value = ''
  try {
    await speechAPI.activateWorld(worldId, code.value.trim())
    if (channel !== channelId.value) return
    await speech.refresh(channel)
    visible.value = false
  } catch (e) { if (channel === channelId.value) error.value = speechError(e) }
  finally { busy.value = false }
}
</script>

<template>
  <NButton v-if="!speech.quota?.enabled && access?.canActivate" text size="small" @click="open">启用 AI 语音</NButton>
  <TTSWorldOverlay v-if="visible && access?.canActivate" title="启用世界 AI 语音" @close="visible = false">
    <form class="ta-form" @submit.prevent="activate">
      <p>输入激活码，为当前世界启用 AI 语音。</p>
      <NAlert v-if="error" type="error">{{ error }}</NAlert>
      <label>激活码<NInput v-model:value="code" type="password" show-password-on="click" autocomplete="off" placeholder="请输入激活码" :disabled="busy" /></label>
      <NButton attr-type="submit" type="primary" :loading="busy">验证并启用</NButton>
    </form>
  </TTSWorldOverlay>
</template>

<style scoped>
.ta-form { display: grid; gap: 18px; }
p { margin: 0; opacity: .75; }
label { display: grid; gap: 8px; }
</style>
