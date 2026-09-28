<script setup lang="ts">
import { computed, watch } from 'vue'
import { NButton, NIcon, NTooltip } from 'naive-ui'
import { PlayerStop, Volume } from '@vicons/tabler'
import { speechPlayer } from './player'
import { useSpeechStore } from './store'
import type { MessageSpeech } from './types'
const props = defineProps<{ message: { id: string; tts?: MessageSpeech | null } }>()
const speech = useSpeechStore()
const metadata = computed(() => {
  const current = props.message.tts
  if (!current || ['invalidated', 'cancelled'].includes(current.status)) return null
  const polled = speech.messageStates[props.message.id]
  if (polled === null) return null
  return polled?.messageRevision === current.messageRevision ? polled : current
})
const active = computed(() => speechPlayer.state.key === `messages:${props.message.id}`)
watch(metadata, (value) => {
  if (active.value && (!value || value.status !== 'ready')) speechPlayer.stop()
})
</script>
<template>
  <NTooltip v-if="metadata?.status === 'ready' && metadata.audioResourceId" trigger="hover">
    <template #trigger>
      <NButton text size="small" aria-label="播放或停止已保存语音" :loading="active && speechPlayer.state.loading" @click.stop="speechPlayer.play('messages', message.id)">
        <NIcon :component="active ? PlayerStop : Volume" size="18" />
      </NButton>
    </template>
    {{ speechPlayer.state.error || (active ? '停止语音' : '播放已保存语音（免费）') }}
  </NTooltip>
</template>
