<script setup lang="ts">
import { NSwitch } from 'naive-ui'
import { useSpeechStore } from './store'
defineProps<{ channelId?: string }>()
const speech = useSpeechStore()
</script>
<template>
  <div v-if="speech.quota?.enabled" class="temporary-speech" @click.stop @mousedown.stop @pointerdown.stop>
    <label>临时语音合成 <NSwitch :value="speech.optIn(channelId || '')" :disabled="!channelId || !speech.quota?.enabled || !speech.quota.autoSynthesis" @update:value="value => { if (channelId) speech.setTemporary(channelId, value) }" /></label>
    <small>{{ !speech.quota?.enabled ? '平台未启用语音' : !speech.quota.autoSynthesis ? '个人自动合成已关闭' : '仅当前标签页、当前频道的新发送消息' }}</small>
  </div>
</template>
<style scoped>
.temporary-speech { padding: 10px 12px; }
label { display: flex; gap: 12px; align-items: center; justify-content: space-between; }
small { display: block; margin-top: 4px; opacity: .7; }
</style>
