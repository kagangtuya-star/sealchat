<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

import type { StageSceneOverlayBinding } from '../shared/stage-types'
import { resolveTheaterReducedMotion } from '../shared/theater-reduced-motion'
import { registerBuiltInSceneOverlayEffects } from './effects'
import { registerBuiltInSceneOverlayRenderers } from './renderers'
import { SceneOverlayRuntime } from './scene-overlay-runtime'

const props = defineProps<{
  sceneId: string
  overlays: StageSceneOverlayBinding[]
}>()

registerBuiltInSceneOverlayEffects()
registerBuiltInSceneOverlayRenderers()

const belowCharactersRef = ref<HTMLDivElement | null>(null)
const aboveCharactersRef = ref<HTMLDivElement | null>(null)
let runtime: SceneOverlayRuntime | null = null

onMounted(() => {
  if (!belowCharactersRef.value || !aboveCharactersRef.value) return
  runtime = new SceneOverlayRuntime({
    belowCharactersHost: belowCharactersRef.value,
    aboveCharactersHost: aboveCharactersRef.value,
    buildContext: () => ({
      reducedMotion: resolveTheaterReducedMotion().effectiveReducedMotion,
    }),
  })
  runtime.reconcile(props.overlays, props.sceneId)
})

watch(
  [() => props.sceneId, () => props.overlays],
  ([sceneId, overlays]) => runtime?.reconcile(overlays, sceneId),
  { deep: true },
)

onBeforeUnmount(() => {
  void runtime?.destroy()
  runtime = null
})
</script>

<template>
  <div ref="belowCharactersRef" class="scene-overlay-layer scene-overlay-below-characters" aria-hidden="true" />
  <div ref="aboveCharactersRef" class="scene-overlay-layer scene-overlay-above-characters" aria-hidden="true" />
</template>

<style scoped>
.scene-overlay-layer {
  position: absolute;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
}
.scene-overlay-below-characters { z-index: 5; }
.scene-overlay-above-characters { z-index: 8980; }
.scene-overlay-layer :deep(.scene-overlay-effect-host) {
  position: absolute;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
}
</style>
