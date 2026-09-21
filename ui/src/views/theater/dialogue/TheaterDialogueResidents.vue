<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch, type CSSProperties } from 'vue'
import { useMessage } from 'naive-ui'
import TheaterPresentationMedia from '@/components/theater-presentation/TheaterPresentationMedia.vue'
import { useTheaterAppearanceCache } from '@/composables/useTheaterAppearanceCache'
import { normalizeTheaterTransform, resolveTheaterTransformStyle, type TheaterPresentation, type TheaterTransform, type TheaterVisualLayer } from '@/types/theaterPresentation'
import { DialogueResidency, dialogueActorKey, type DialogueActorReference } from './theater-dialogue-residency'
import { layoutDialoguePortraits, normalizedDialoguePosition, type DialoguePortraitRect, type DialoguePosition } from './theater-dialogue-layout'
import type { DialogueController, DialogueControllerTemplate } from './theater-dialogue-controller'
import type { TheaterDialogueRuntimeController } from './theater-dialogue-runtime'

const props = defineProps<{ worldId: string; channelId: string; runtime: TheaterDialogueRuntimeController; controller: DialogueController; template: DialogueControllerTemplate; canDrag: boolean; savePosition: (key: string, position: DialoguePosition) => Promise<void> }>()
const root = ref<HTMLElement | null>(null)
const size = ref({ width: 0, height: 0 })
const message = useMessage()
const cache = useTheaterAppearanceCache()
type ResidentPortrait = { portrait: TheaterVisualLayer; transform?: TheaterTransform }
let residency = new DialogueResidency<ResidentPortrait>(props.worldId)
const residents = shallowRef(residency.snapshot())
const dimensions = ref<Record<string, number>>({})
const preview = ref<Record<string, DialoguePosition> | null>(null)
let unsubscribe: (() => void) | null = null
let observer: ResizeObserver | null = null
let epoch = 0
let lastSignature = ''
let dismissed = 0
let gesture: { id: number; key: string; scope: string; target: HTMLElement; dx: number; dy: number } | null = null
const layout = computed(() => layoutDialoguePortraits({ ...size.value,
  actors: residents.value.residents.map(item => ({
    actorKey: item.actorKey,
    aspectRatio: dimensions.value[item.actorKey] || item.portrait.portrait.media.width / item.portrait.portrait.media.height,
    transform: normalizeTheaterTransform(item.portrait.transform, props.template.portraitStyle.transform),
  })),
  positions: preview.value || props.controller.positions,
}))
const layers = computed(() => layout.value.map(rect => ({ ...rect, resident: residents.value.residents.find(item => item.actorKey === rect.actorKey)! })))
const decorationStyle = (layer: TheaterVisualLayer): CSSProperties => ({
  ...resolveTheaterTransformStyle(layer.transform),
  mixBlendMode: layer.blendMode,
})
const refresh = () => { residents.value = residency.snapshot() }
const playPortrait = (actor: DialogueActorReference, presentation: TheaterPresentation | null | undefined) => {
  const portrait = presentation?.portrait
    ? {
        portrait: presentation.portrait,
        transform: presentation.multiplayerPortraitTransform,
      }
    : null
  const key = dialogueActorKey(actor)
  if (residents.value.residents.find(item => item.actorKey === key)?.portrait.portrait.media.assetId !== portrait?.portrait.media.assetId) delete dimensions.value[key]
  residency.play(actor, portrait, props.controller)
  refresh()
}
const endGesture = () => {
  if (gesture?.target.hasPointerCapture(gesture.id)) gesture.target.releasePointerCapture(gesture.id)
  gesture = null
  preview.value = null
}
const cancelGesture = (event: PointerEvent) => { if (gesture?.id === event.pointerId) endGesture() }
const playCurrent = () => {
  const snapshot = props.runtime.getSnapshot()
  if (snapshot.queue.dismissedThroughSequence !== dismissed) {
    dismissed = snapshot.queue.dismissedThroughSequence
    residency.clear(); endGesture(); lastSignature = ''; epoch++
  }
  const current = snapshot.queue.current?.message
  if (!current) { residency.end(); refresh(); return }
  const signature = JSON.stringify([current.messageId, current.actor])
  if (signature === lastSignature) { residency.configure(props.controller); refresh(); return }
  lastSignature = signature
  const actor = current.actor
  const source = { worldId: props.worldId, sourceChannelId: current.sourceChannelId || props.channelId, identityId: actor.identityId || '', sharedIdentityId: actor.sharedIdentityId }
  playPortrait(source, actor.appearance.theaterPresentation)
  const requestEpoch = ++epoch
  // Appearance resolution uses the existing per-actor cache, never a shared root
  // in place of a channel identity and never an extra message subscription.
  void cache.resolve(source.worldId, source.sourceChannelId, { identityId: source.identityId, variantId: actor.variantId }).then(result => {
    if (requestEpoch !== epoch || props.runtime.getSnapshot().queue.current?.message.messageId !== current.messageId) return
    if (result?.presentation) playPortrait(source, result.presentation)
  }).catch(() => undefined)
}
const down = (event: PointerEvent, rect: DialoguePortraitRect) => {
  if (!props.canDrag || !props.controller.dragEnabled || event.button !== 0) return
  event.stopPropagation(); event.preventDefault()
  endGesture()
  const target = event.currentTarget as HTMLElement
  const bounds = root.value!.getBoundingClientRect()
  gesture = { id: event.pointerId, key: rect.actorKey, scope: `${props.worldId}:${props.channelId}`, target, dx: event.clientX - bounds.left - rect.x, dy: event.clientY - bounds.top - rect.y }
  preview.value = Object.fromEntries(layout.value.map(item => [item.actorKey, normalizedDialoguePosition(item, size.value.width, size.value.height)]))
  target.setPointerCapture(event.pointerId)
}
const move = (event: PointerEvent) => {
  if (!gesture || event.pointerId !== gesture.id) return
  event.stopPropagation(); event.preventDefault()
  const rect = layout.value.find(item => item.actorKey === gesture!.key)
  if (!rect || !root.value) { endGesture(); return }
  const bounds = root.value.getBoundingClientRect()
  preview.value = { ...preview.value, [rect.actorKey]: normalizedDialoguePosition({ ...rect, x: event.clientX - bounds.left - gesture.dx, y: event.clientY - bounds.top - gesture.dy }, size.value.width, size.value.height) }
}
const up = async (event: PointerEvent) => {
  if (!gesture || event.pointerId !== gesture.id) return
  event.stopPropagation(); event.preventDefault()
  if (gesture.scope !== `${props.worldId}:${props.channelId}`) { endGesture(); return }
  const key = gesture.key
  const rect = layout.value.find(item => item.actorKey === key)
  const scope = `${props.worldId}:${props.channelId}`
  const position = rect && normalizedDialoguePosition(rect, size.value.width, size.value.height)
  endGesture()
  if (!position) return
  try { await props.savePosition(key, position) }
  catch { if (scope === `${props.worldId}:${props.channelId}`) message.error('立绘位置保存失败，已恢复确认的位置') }
}
watch(() => [props.worldId, props.channelId], () => { epoch++; residency = new DialogueResidency(props.worldId); lastSignature = ''; dismissed = 0; dimensions.value = {}; endGesture(); refresh(); playCurrent() }, { flush: 'sync' })
watch(() => [props.controller.allowList, props.controller.denyList, props.controller.maxPortraits], () => { lastSignature = ''; residency.configure(props.controller); playCurrent() }, { deep: true })
watch(() => [props.controller.dragEnabled, props.canDrag], () => { if (!props.controller.dragEnabled || !props.canDrag) endGesture() })
watch(() => props.controller.positions, endGesture, { deep: true })
onMounted(() => {
  unsubscribe = props.runtime.subscribe(playCurrent)
  observer = new ResizeObserver(entries => { endGesture(); size.value = { width: entries[0].contentRect.width, height: entries[0].contentRect.height } })
  if (root.value) observer.observe(root.value)
})
onBeforeUnmount(() => { epoch++; endGesture(); unsubscribe?.(); observer?.disconnect() })
</script>

<template>
  <div ref="root" class="dialogue-residents">
    <div v-for="item in layers" :key="item.actorKey" class="dialogue-resident" :class="{ draggable: canDrag && controller.dragEnabled, dim: residents.currentKey !== item.actorKey && controller.inactiveStyle !== 'none', grayscale: residents.currentKey !== item.actorKey && controller.inactiveStyle === 'dim-grayscale' }"
      :style="{ left: `${item.x}px`, top: `${item.y}px`, width: `${item.width}px`, height: `${item.height}px`, transform: `rotate(${item.rotation}deg)`, opacity: item.opacity, zIndex: item.zIndex }"
      @pointerdown="down($event, item)" @pointermove="move" @pointerup="up" @pointercancel.stop="cancelGesture" @lostpointercapture="cancelGesture" @click.stop>
      <div class="dialogue-resident__portrait">
        <TheaterPresentationMedia :key="item.resident.portrait.portrait.media.assetId" :media="item.resident.portrait.portrait.media" :playback-rate="template.portraitStyle.playbackRate"
          @dimensions="(width, height) => { if (width > 0 && height > 0) dimensions[item.actorKey] = width / height }" />
      </div>
      <div v-for="layer in template.presentation.portraitDecorations.filter(item => item.enabled)" :key="layer.id" class="dialogue-decoration" :style="decorationStyle(layer)"><TheaterPresentationMedia :media="layer.media" :playback-rate="layer.playbackRate" /></div>
    </div>
  </div>
</template>
<style scoped>
.dialogue-residents { position: absolute; inset: 0; pointer-events: none; }
.dialogue-resident, .dialogue-resident__portrait, .dialogue-decoration { position: absolute; pointer-events: none; }
.dialogue-resident__portrait { inset: 0; }
.dialogue-resident.draggable { pointer-events: auto; touch-action: none; cursor: grab; }
.dialogue-resident { transition: filter .15s; }
.dialogue-resident.dim { filter: brightness(.5); }
.dialogue-resident.dim.grayscale { filter: brightness(.5) grayscale(1); }
.dialogue-resident__portrait :deep(.theater-media) { object-fit: contain !important; }
@media (prefers-reduced-motion: reduce) { .dialogue-resident { transition: none; } }
</style>
