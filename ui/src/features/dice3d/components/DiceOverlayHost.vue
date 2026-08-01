<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import type { DiceVisualPayload } from '@/types'
import { DiceArena } from '../engine/DiceArena'
import { dice3dRuntime } from '../runtime'

const props = defineProps<{
  surfaceElement?: HTMLElement | null
  chatSurfaceElement?: HTMLElement | null
}>()
const emit = defineEmits<{ (event: 'ready'): void, (event: 'failed'): void }>()

const canvasRef = ref<HTMLCanvasElement | null>(null)
const hostRef = ref<HTMLDivElement | null>(null)
let arena: DiceArena | null = null
let ready = false
let resizeObserver: ResizeObserver | null = null
let unsubscribe: (() => void) | null = null
const pending: DiceVisualPayload[] = []
const surfaceMode = ref<DiceVisualPayload['surfaceMode']>('auto')
const customSurface = ref<DiceVisualPayload['customSurface'] | null>(null)

const syncRect = () => {
	const host = hostRef.value
	if (!host) return
	let rect: Pick<DOMRect, 'left' | 'top' | 'width' | 'height'>
	if (surfaceMode.value === 'fullscreen') {
		rect = { left: 0, top: 0, width: window.innerWidth, height: window.innerHeight }
	} else if (surfaceMode.value === 'custom' && customSurface.value) {
		rect = {
			left: customSurface.value.x * window.innerWidth,
			top: customSurface.value.y * window.innerHeight,
			width: customSurface.value.width * window.innerWidth,
			height: customSurface.value.height * window.innerHeight,
		}
	} else {
		const surface = surfaceMode.value === 'chat'
			? props.chatSurfaceElement || props.surfaceElement
			: props.surfaceElement
		if (!surface) return
		rect = surface.getBoundingClientRect()
	}
  host.style.left = `${rect.left}px`
  host.style.top = `${rect.top}px`
  host.style.width = `${Math.max(1, rect.width)}px`
  host.style.height = `${Math.max(1, rect.height)}px`
  host.style.display = rect.width > 1 && rect.height > 1 ? 'block' : 'none'
  arena?.resize(rect.width, rect.height)
}

const bindSurface = () => {
	resizeObserver?.disconnect()
	resizeObserver = null
	window.removeEventListener('resize', syncRect)
	window.removeEventListener('scroll', syncRect, true)
	if (!props.surfaceElement) return
	resizeObserver = new ResizeObserver(syncRect)
	resizeObserver.observe(props.surfaceElement)
	if (props.chatSurfaceElement) resizeObserver.observe(props.chatSurfaceElement)
  window.addEventListener('resize', syncRect)
  window.addEventListener('scroll', syncRect, true)
  syncRect()
}

watch(() => [props.surfaceElement, props.chatSurfaceElement], () => nextTick(bindSurface))

onMounted(async () => {
  await nextTick()
  if (!canvasRef.value) return
  arena = new DiceArena(canvasRef.value)
  unsubscribe = dice3dRuntime.subscribe(payload => {
		surfaceMode.value = payload.surfaceMode || 'auto'
		customSurface.value = payload.customSurface || null
		syncRect()
		if (!arena || !ready) pending.push(payload)
		else arena.play(payload)
  })
	try {
		await arena.init()
		ready = true
		emit('ready')
	} catch (error) {
		console.warn('[dice3d] 初始化失败，已降级为文字骰点', error)
		emit('failed')
		return
	}
  syncRect()
  pending.splice(0).forEach(payload => arena?.play(payload))
  bindSurface()
})

onBeforeUnmount(() => {
  unsubscribe?.()
  unsubscribe = null
  resizeObserver?.disconnect()
  window.removeEventListener('resize', syncRect)
  window.removeEventListener('scroll', syncRect, true)
  arena?.dispose()
  arena = null
	ready = false
})
</script>

<template>
  <Teleport to="body">
    <div ref="hostRef" class="dice3d-overlay-host" aria-hidden="true">
      <canvas ref="canvasRef" class="dice3d-overlay-canvas" />
    </div>
  </Teleport>
</template>

<style scoped>
.dice3d-overlay-host {
  position: fixed;
  z-index: 9500;
  overflow: hidden;
  pointer-events: none;
  contain: strict;
}
.dice3d-overlay-canvas { width: 100%; height: 100%; display: block; pointer-events: none; }
</style>
