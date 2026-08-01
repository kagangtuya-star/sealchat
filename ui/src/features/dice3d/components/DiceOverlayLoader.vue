<script setup lang="ts">
import { defineAsyncComponent, onBeforeUnmount, onMounted, ref } from 'vue'

import { dice3dRuntime } from '../runtime'

defineProps<{
  surfaceElement?: HTMLElement | null
  chatSurfaceElement?: HTMLElement | null
}>()

const active = ref(false)
const engineReady = ref(false)
const engineFailed = ref(false)
let unsubscribe: (() => void) | null = null

const DiceOverlayHost = defineAsyncComponent({
  loader: () => import('./DiceOverlayHost.vue'),
  delay: 0,
  timeout: 30_000,
})

onMounted(() => {
  unsubscribe = dice3dRuntime.subscribeActivation(() => {
    active.value = true
  })
})

onBeforeUnmount(() => {
  unsubscribe?.()
  unsubscribe = null
})
</script>

<template>
	<DiceOverlayHost
	  v-if="active"
	  :surface-element="surfaceElement"
	  :chat-surface-element="chatSurfaceElement"
	  @ready="engineReady = true"
	  @failed="engineFailed = true"
	/>
	<div v-if="active && !engineReady && !engineFailed" class="dice3d-loading">3D 骰子加载中…</div>
</template>

<style scoped>
.dice3d-loading { position: fixed; z-index: 9501; right: 18px; bottom: 18px; padding: 7px 11px; border: 1px solid rgba(127, 127, 127, .24); border-radius: 999px; color: var(--sc-text-secondary, #52525b); background: color-mix(in srgb, var(--sc-bg-elevated, #fff) 88%, transparent); box-shadow: 0 6px 20px rgba(0, 0, 0, .12); font-size: 12px; pointer-events: none; }
</style>
