<script setup lang="ts">
import { computed } from 'vue'
import { resolveMediaFxCapabilities } from '@/features/media-fx/media-fx'
import type { MediaFxDirectiveValue } from '@/features/media-fx/media-fx-dom'
import { normalizeStageIframeContent, stageObjectMediaFx, type StageEmbedEventPublished, type StageObject } from '../shared/stage-types'
import type { ChannelEmbedTheaterEventSink } from '@/bridge/channelEmbedHost'
import type { ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'
import { resolveTheaterReducedMotion } from '../shared/theater-reduced-motion'
import StageIframeFrame from './StageIframeFrame.vue'

const props = defineProps<{
  object: StageObject
  characterSnapshot: ChatCharactersSnapshotPayload
  worldId?: string
  channelId?: string
  scopeType?: 'world' | 'channel'
}>()
const emit = defineEmits<{ embedEventPublished: [event: StageEmbedEventPublished] }>()
// Bind each event to the object instance that rendered this host; the stage
// never locates an object by formId.
const embedEventSink: ChannelEmbedTheaterEventSink = (event) => {
  emit('embedEventPublished', {
    objectId: props.object.id,
    eventId: event.eventId,
    formId: event.formId,
    channelId: event.channelId,
    topic: event.topic,
  })
}
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
    :world-id="props.worldId"
    :channel-id="props.channelId"
    :scope-type="props.scopeType"
    :character-snapshot="props.characterSnapshot"
    :media-fx="mediaFxBinding"
    :embed-event-sink="embedEventSink"
  />
</template>
