<script setup lang="ts">
import { computed, useAttrs } from 'vue'
import type { HTMLAttributes } from 'vue'
import type { CameraState, StageEmbedEventPublished, StageEntrancePlayback, StageObject } from '../shared/stage-types'
import { compareStageLayersBottomToTop, stageObjectHasDomVisualDescendant } from './stage-layer-order'
import StageTextVisualObject from './StageTextVisualObject.vue'
import type { StageDomVisualTransform } from './stage-dom-visual-preview'
import { stageRootCoordinateSpace, stageViewportBandCamera } from './stage-coordinate-space'
import type { ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  objects: Record<string, StageObject>
  camera: CameraState
  viewportWidth: number
  viewportHeight: number
  entrancePlaybacks: Record<string, StageEntrancePlayback>
  hiddenObjectIds: string[]
  stackingOrder: Record<string, number>
  visualTransforms?: Readonly<Record<string, StageDomVisualTransform>>
  characterSnapshot: ChatCharactersSnapshotPayload
  worldId?: string
  channelId?: string
  scopeType?: 'world' | 'channel'
}>()
const emit = defineEmits<{ embedEventPublished: [event: StageEmbedEventPublished] }>()

const attrs = useAttrs() as HTMLAttributes
const hiddenObjectIds = computed(() => new Set(props.hiddenObjectIds))
const roots = computed(() => Object.values(props.objects)
  .filter((object) => (
    object.parentId === null
    && (object.visible || props.entrancePlaybacks[object.id]?.direction === 'exit')
  ))
  .sort(compareStageLayersBottomToTop))

const domVisualRoots = computed(() => roots.value.filter((object) => stageObjectHasDomVisualDescendant(object, props.objects)))

const viewport = computed(() => ({ width: props.viewportWidth, height: props.viewportHeight }))

// Mirrors the Konva band cameras: world roots follow the scene camera, viewport roots only the
// viewport design scale. Each root picks the frame of its own coordinate space.
const worldCameraStyle = computed(() => ({
  transform: `translate(${props.viewportWidth / 2 + props.camera.x}px, ${props.viewportHeight / 2 + props.camera.y}px) scale(${props.camera.zoom})`,
}))

const viewportCameraStyle = computed(() => ({
  transform: `scale(${stageViewportBandCamera(viewport.value).scale.x})`,
}))

const cameraStyle = (object: StageObject) => (
  stageRootCoordinateSpace(object) === 'viewport' ? viewportCameraStyle.value : worldCameraStyle.value
)

const rootStyle = (object: StageObject) => ({
  zIndex: String(props.stackingOrder[object.id] ?? 101),
})
</script>

<template>
  <div class="theater-text-overlay-stack">
    <div
      v-for="object in domVisualRoots"
      :key="object.id"
      class="theater-text-overlay"
      :class="attrs.class"
      :style="[attrs.style, rootStyle(object)]"
    >
      <div class="theater-text-overlay__camera" :style="cameraStyle(object)">
        <StageTextVisualObject
          :key="`${object.id}:${props.entrancePlaybacks[object.id]?.token || 0}`"
          :object="object"
          :world-id="props.worldId"
          :channel-id="props.channelId"
          :scope-type="props.scopeType"
          :objects="props.objects"
          :entrance-playbacks="props.entrancePlaybacks"
          :hidden-object-ids="hiddenObjectIds"
          :visual-transforms="props.visualTransforms"
          :viewport="viewport"
          :character-snapshot="props.characterSnapshot"
          @embed-event-published="emit('embedEventPublished', $event)"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.theater-text-overlay-stack { display: contents; }

.theater-text-overlay {
  position: absolute;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
}

.theater-text-overlay__camera {
  position: absolute;
  top: 0;
  left: 0;
  width: 0;
  height: 0;
  transform-origin: 0 0;
}
</style>
