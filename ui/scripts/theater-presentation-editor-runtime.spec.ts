import assert from 'node:assert/strict'
import { reactive } from 'vue'

import {
  buildTheaterPresentationPatch,
  captureTheaterEditorSnapshot,
  commitTheaterEditorTransaction,
  createTheaterPresentationEditorState,
  createTheaterVisualLayer,
  dispatchTheaterEditorCommand,
  redoTheaterEditor,
  undoTheaterEditor,
} from '../src/components/theater-presentation/theaterPresentationEditorState'
import { resolveTheaterMediaCandidates } from '../src/components/theater-presentation/theaterPresentationMedia'
import {
  buildTheaterAppearanceAssetFields,
  canApplyTheaterAppearanceAsset,
  getTheaterAssetErrorCode,
  isTheaterAppearanceAssetProcessing,
  type TheaterAppearanceAsset,
} from '../src/components/theater-presentation/theaterAppearanceAssetState'
import { createDefaultTheaterPresentation, type TheaterMediaRef } from '../src/types/theaterPresentation'
import {
  cloneChannelIdentityTheaterPresentation,
  cloneChannelIdentityTheaterPresentationPatch,
  resolveChannelIdentityVariantTheaterPatch,
} from '../src/utils/channelIdentityTheaterPresentation'

const media = (id: string, mimeType: TheaterMediaRef['mimeType'] = 'image/png'): TheaterMediaRef => ({
  assetId: `asset-${id}`,
  resourceAttachmentId: `attachment-${id}`,
  fallbackAttachmentId: mimeType === 'video/webm' ? `fallback-${id}` : undefined,
  mimeType,
  kind: mimeType === 'video/webm' ? 'video' : 'static_image',
  width: 1000,
  height: 1500,
})

const outer = createDefaultTheaterPresentation()
assert.equal(outer.dialogue.speaker.transform.width, 0.34, 'default speaker width must fit about ten characters')
assert.equal(outer.dialogue.speaker.transform.x, outer.dialogue.content.transform.x, 'speaker and content must share left edge')
assert.deepEqual(outer.dialogue.speaker.transform, { x: 0.025, y: 0.065, width: 0.34, height: 0.12, rotation: 0, opacity: 1, zIndex: 2 })
assert.deepEqual(outer.dialogue.content.transform, { x: 0.025, y: 0.28, width: 0.95, height: 0.68, rotation: 0, opacity: 1, zIndex: 2 })
const defaultPortrait = createTheaterVisualLayer(media('default-portrait'), 'viewport', 'default-portrait')
assert.deepEqual(defaultPortrait.transform, {
  x: 0.13,
  y: 0.22,
  width: 0.27,
  height: 0.54,
  rotation: 0,
  opacity: 1,
  zIndex: 0,
}, 'default portrait must be 40% smaller while keeping its center')
const reactivePresentation = reactive(createDefaultTheaterPresentation())
const reactivePatch = reactive({ portrait: null })
assert.doesNotThrow(() => cloneChannelIdentityTheaterPresentation(reactivePresentation))
assert.deepEqual(cloneChannelIdentityTheaterPresentationPatch(reactivePatch), { portrait: null })
const state0 = createTheaterPresentationEditorState({ mode: 'base', presentation: outer })
const reactiveSnapshot = reactive(captureTheaterEditorSnapshot(state0))
assert.doesNotThrow(() => dispatchTheaterEditorCommand(
  state0,
  { type: 'set-transform', target: { kind: 'dialogue' }, transform: { x: 0.1 } },
  { historySnapshot: reactiveSnapshot },
))
let state = dispatchTheaterEditorCommand(state0, { type: 'set-media', target: { kind: 'portrait' }, media: media('portrait') })
assert.equal(outer.portrait, null, 'modal draft must not mutate outer form before apply')
assert.equal(state.draft.portrait?.media.assetId, 'asset-portrait')

state = dispatchTheaterEditorCommand(state, { type: 'set-transform', target: { kind: 'portrait' }, transform: { x: 0.25 } })
assert.equal(state.draft.portrait?.transform.x, 0.25, 'left drag command must update inspector source')
state = dispatchTheaterEditorCommand(state, { type: 'set-transform', target: { kind: 'portrait' }, transform: { width: 0.72 } })
assert.equal(state.draft.portrait?.transform.width, 0.72, 'inspector command must update preview source')
state = dispatchTheaterEditorCommand(state, { type: 'set-transform', target: { kind: 'speaker' }, transform: { x: 0.2 } })
assert.equal(state.draft.dialogue.speaker.transform.x, 0.2, 'speaker must have an independent transform')
state = dispatchTheaterEditorCommand(state, { type: 'set-transform', target: { kind: 'content' }, transform: { y: 0.4 } })
assert.equal(state.draft.dialogue.content.transform.y, 0.4, 'content must have an independent transform')
state = dispatchTheaterEditorCommand(state, { type: 'set-layer-property', target: { kind: 'speaker' }, property: 'fontScale', value: 1.5 })
assert.equal(state.draft.dialogue.speaker.fontScale, 1.5, 'speaker font scale must update independently')
state = dispatchTheaterEditorCommand(state, { type: 'set-layer-property', target: { kind: 'content' }, property: 'fontScale', value: 0.8 })
assert.equal(state.draft.dialogue.content.fontScale, 0.8, 'content font scale must update independently')
state = dispatchTheaterEditorCommand(state, { type: 'set-dialogue-property', property: 'contentColor', value: '#DDEEFF' })
assert.equal(state.draft.dialogue.contentColor, '#DDEEFF', 'content color must update independently')
state = dispatchTheaterEditorCommand(state, { type: 'set-dialogue-property', property: 'charactersPerSecond', value: 12 })
assert.equal(state.draft.dialogue.charactersPerSecond, 12, 'content playback speed must update')
state = dispatchTheaterEditorCommand(state, { type: 'set-narration-property', property: 'enabled', value: true })
state = dispatchTheaterEditorCommand(state, { type: 'set-narration-property', property: 'backdropColor', value: '#121212' })
state = dispatchTheaterEditorCommand(state, { type: 'set-narration-property', property: 'backdropOpacity', value: 0.7 })
assert.deepEqual(state.draft.narration, { enabled: true, backdropColor: '#121212', backdropOpacity: 0.7 })

const transactionStart = captureTheaterEditorSnapshot(state)
state = dispatchTheaterEditorCommand(state, { type: 'set-transform', target: { kind: 'portrait' }, transform: { x: 0.3 } }, { recordHistory: false })
state = dispatchTheaterEditorCommand(state, { type: 'set-transform', target: { kind: 'portrait' }, transform: { x: 0.4 } }, { recordHistory: false })
state = dispatchTheaterEditorCommand(state, { type: 'set-transform', target: { kind: 'portrait' }, transform: { x: 0.5 } }, { recordHistory: false })
state = commitTheaterEditorTransaction(state, transactionStart)
const historyLength = state.history.past.length
state = undoTheaterEditor(state)
assert.equal(state.draft.portrait?.transform.x, 0.25)
state = redoTheaterEditor(state)
assert.equal(state.draft.portrait?.transform.x, 0.5)
assert.equal(state.history.past.length, historyLength)

for (let index = 0; index < 16; index += 1) {
  state = dispatchTheaterEditorCommand(state, { type: 'add-decoration', layer: createTheaterVisualLayer(media(`decoration-${index}`), 'portrait', `decoration-${index}`) })
}
assert.equal(state.draft.portraitDecorations.length, 16)
const atLimit = dispatchTheaterEditorCommand(state, { type: 'add-decoration', layer: createTheaterVisualLayer(media('overflow'), 'portrait', 'overflow') })
assert.equal(atLimit, state)
const duplicate = dispatchTheaterEditorCommand(state, { type: 'add-decoration', layer: createTheaterVisualLayer(media('duplicate'), 'portrait', 'decoration-0') })
assert.equal(duplicate, state)
state = dispatchTheaterEditorCommand(state, { type: 'reorder-decoration', id: 'decoration-15', beforeId: 'decoration-0' })
assert.equal(state.draft.portraitDecorations[0].id, 'decoration-15')

const base = state.draft
let variant = createTheaterPresentationEditorState({ mode: 'variant', base, patch: {} })
assert.deepEqual(buildTheaterPresentationPatch(variant), {})
variant = dispatchTheaterEditorCommand(variant, { type: 'set-section-mode', section: 'portrait', mode: 'clear' })
variant = dispatchTheaterEditorCommand(variant, { type: 'set-section-mode', section: 'decorations', mode: 'custom' })
variant = dispatchTheaterEditorCommand(variant, { type: 'set-section-mode', section: 'dialogue', mode: 'inherit' })
assert.deepEqual(buildTheaterPresentationPatch(variant), {
  portrait: null,
  portraitDecorations: base.portraitDecorations,
})
variant = dispatchTheaterEditorCommand(variant, { type: 'set-section-mode', section: 'narration', mode: 'custom' })
variant = dispatchTheaterEditorCommand(variant, { type: 'set-narration-property', property: 'enabled', value: true })
assert.equal(buildTheaterPresentationPatch(variant).narration?.enabled, true)
assert.deepEqual(resolveChannelIdentityVariantTheaterPatch({ theaterPresentation: undefined, appearance: { theaterPresentation: { portrait: null } } }), { portrait: null })
assert.deepEqual(resolveChannelIdentityVariantTheaterPatch({ theaterPresentation: { dialogue: null }, appearance: { theaterPresentation: { portrait: null } } }), { dialogue: null })

let textVariant = createTheaterPresentationEditorState({ mode: 'variant', base, patch: {} })
textVariant = dispatchTheaterEditorCommand(textVariant, { type: 'set-section-mode', section: 'speaker', mode: 'clear' })
assert.equal(buildTheaterPresentationPatch(textVariant).dialogue?.speaker.enabled, false)

const statuses = ['pending', 'processing', 'ready', 'failed'] as const
const assets = statuses.map((status): TheaterAppearanceAsset => ({
  id: status,
  channelId: 'channel-1',
  ownerUserId: 'user-2',
  identityId: 'identity-1',
  purpose: 'portrait',
  status,
  progress: status === 'processing' ? 0.5 : 0,
  failureCode: status === 'failed' ? 'TRANSCODE_FAILED' : undefined,
  media: status === 'ready' ? media('ready') : undefined,
}))
assert.deepEqual(assets.map(isTheaterAppearanceAssetProcessing), [true, true, false, false])
assert.deepEqual(assets.map(canApplyTheaterAppearanceAsset), [false, false, true, false])
assert.equal(assets[3].failureCode, 'TRANSCODE_FAILED')
assert.deepEqual(buildTheaterAppearanceAssetFields({
  purpose: 'dialogue-frame',
  identityId: 'identity-1',
  variantId: 'variant-1',
  targetUserId: 'delegated-user',
}), {
  purpose: 'dialogue-frame',
  identityId: 'identity-1',
  variantId: 'variant-1',
  targetUserId: 'delegated-user',
})
assert.equal(getTheaterAssetErrorCode({ response: { data: { error: { code: 'ASSET_IN_USE' } } } }), 'ASSET_IN_USE')

const video = media('video', 'video/webm')
assert.deepEqual(resolveTheaterMediaCandidates(video, { supportsVideo: true, preferStatic: false }), [
  { kind: 'video', attachmentId: 'attachment-video' },
  { kind: 'image', attachmentId: 'fallback-video' },
])
assert.deepEqual(resolveTheaterMediaCandidates(video, { supportsVideo: false }), [
  { kind: 'image', attachmentId: 'fallback-video' },
  { kind: 'video', attachmentId: 'attachment-video' },
])
assert.deepEqual(resolveTheaterMediaCandidates({ ...video, fallbackAttachmentId: undefined }, { supportsVideo: false }), [
  { kind: 'video', attachmentId: 'attachment-video' },
])

console.log('theater presentation editor runtime tests passed')
