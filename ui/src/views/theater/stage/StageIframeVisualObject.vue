<script setup lang="ts">
import { computed } from 'vue'
import { resolveMediaFxCapabilities } from '@/features/media-fx/media-fx'
import type { MediaFxDirectiveValue } from '@/features/media-fx/media-fx-dom'
import { normalizeStageIframeContent, stageObjectMediaFx, type StageObject } from '../shared/stage-types'
import type { ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'
import { resolveTheaterReducedMotion } from '../shared/theater-reduced-motion'
import StageIframeFrame from './StageIframeFrame.vue'

const props = defineProps<{
  object: StageObject
  characterSnapshot: ChatCharactersSnapshotPayload
}>()
const iframeContent = computed(() => normalizeStageIframeContent(props.object.content?.iframe))
const mediaFxBinding = computed<MediaFxDirectiveValue>(() => ({
  spec: stageObjectMediaFx(props.object),
  filters: resolveMediaFxCapabilities('dom', false).filters,
  reducedMotion: resolveTheaterReducedMotion().effectiveReducedMotion,
}))
</script>

<template>
  <StageIframeFrame
    class="theater-iframe-visual-object"
    :iframe="iframeContent"
    :interactive="props.object.interactive"
    :title="props.object.name"
    :object-id="props.object.id"
    :character-snapshot="props.characterSnapshot"
    :media-fx="mediaFxBinding"
  />
</template>
