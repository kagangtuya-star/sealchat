<script setup lang="ts">
import { computed } from 'vue'
import type { ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'
import { WORLD_UNIT_PX, type CameraState, type StageSurfaceEmbed, type StageSurfaceTarget } from '../shared/stage-types'
import StageIframeFrame from './StageIframeFrame.vue'

const props = defineProps<{
  target: StageSurfaceTarget
  embed: StageSurfaceEmbed
  interactive: boolean
  camera: CameraState
  viewportWidth: number
  viewportHeight: number
  fieldWidth: number
  fieldHeight: number
  worldId: string
  channelId: string
  characterSnapshot: ChatCharactersSnapshotPayload
}>()

const frameStyle = computed(() => props.target === 'background' ? { width: '100%', height: '100%' } : {
  width: `${props.fieldWidth * WORLD_UNIT_PX}px`,
  height: `${props.fieldHeight * WORLD_UNIT_PX}px`,
  // Same centered field and viewport camera as foregroundCameraGroup.
  transform: `translate(${props.viewportWidth / 2 + props.camera.x}px, ${props.viewportHeight / 2 + props.camera.y}px) scale(${props.camera.zoom}) translate(-50%, -50%)`,
  transformOrigin: '0 0',
})
</script>

<template>
  <div class="theater-surface-iframe" :data-stage-surface="target">
    <StageIframeFrame
      :iframe="embed.iframe"
      :interactive="interactive"
      :block-focus="!interactive"
      :title="target === 'background' ? '背景网页' : '前景网页'"
      :character-snapshot="characterSnapshot"
      :world-id="worldId"
      :channel-id="channelId"
      :style="frameStyle"
    />
  </div>
</template>

<style scoped>
.theater-surface-iframe { position: absolute; inset: 0; overflow: hidden; pointer-events: none; }
</style>
