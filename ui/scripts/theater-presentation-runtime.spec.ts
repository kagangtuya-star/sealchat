import assert from 'node:assert/strict'

import {
  createDefaultTheaterPresentation,
  createDefaultTheaterTransform,
  normalizeTheaterPresentation,
  normalizeTheaterTransform,
  resolveTheaterPresentation,
  resolveTheaterBackdropColor,
  resolveTheaterTransformStyle,
  theaterPresentationPatchSchema,
  theaterPresentationSchema,
  type TheaterLayerSpace,
  type TheaterVisualLayer,
} from '../src/types/theaterPresentation'

const layer = (id: string, space: TheaterLayerSpace): TheaterVisualLayer => ({
  id,
  enabled: true,
  media: {
    assetId: `asset-${id}`,
    resourceAttachmentId: `attachment-${id}`,
    mimeType: 'image/webp',
    kind: 'static_image',
    width: 800,
    height: 1200,
  },
  space,
  transform: createDefaultTheaterTransform(),
  fit: 'cover',
  playbackRate: 1,
  blendMode: 'normal',
})

const defaults = createDefaultTheaterPresentation()
assert.equal(theaterPresentationSchema.safeParse(defaults).success, true)
assert.deepEqual(normalizeTheaterPresentation({}), defaults)
assert.notEqual(createDefaultTheaterPresentation().dialogue, createDefaultTheaterPresentation().dialogue)
assert.equal(defaults.dialogue.speaker.enabled, true)
assert.equal(defaults.dialogue.content.enabled, true)
assert.equal(defaults.dialogue.speaker.fontScale, 0.85)
assert.equal(defaults.dialogue.content.fontScale, 1.2)
assert.equal(defaults.dialogue.contentColor, '#F4F4F5')
assert.equal(defaults.dialogue.charactersPerSecond, 6)
assert.deepEqual(defaults.narration, { enabled: false, backdropColor: '#000000', backdropOpacity: 1 })
assert.deepEqual(normalizeTheaterPresentation({ schemaVersion: 1 }), defaults)

const legacyDefaults = structuredClone(defaults) as any
delete legacyDefaults.dialogue.speaker.fontScale
delete legacyDefaults.dialogue.content.fontScale
delete legacyDefaults.dialogue.contentColor
delete legacyDefaults.dialogue.charactersPerSecond
delete legacyDefaults.narration
assert.deepEqual(normalizeTheaterPresentation(legacyDefaults), defaults)
assert.deepEqual(theaterPresentationSchema.parse(legacyDefaults), defaults)

const legacyLayout = structuredClone(defaults)
legacyLayout.dialogue.transform.x = 0.02
legacyLayout.dialogue.transform.width = 0.96
legacyLayout.dialogue.speaker.transform = { x: 0.08, y: 0.12, width: 0.34, height: 0.12, rotation: 0, opacity: 1, zIndex: 2 }
legacyLayout.dialogue.speaker.fontScale = 1
legacyLayout.dialogue.content.transform = { x: 0.08, y: 0.3, width: 0.84, height: 0.56, rotation: 0, opacity: 1, zIndex: 2 }
legacyLayout.dialogue.content.fontScale = 1
assert.deepEqual(normalizeTheaterPresentation(legacyLayout), defaults)

const previousLayout = structuredClone(defaults)
previousLayout.dialogue.speaker.transform = { x: 0.075, y: 0.12, width: 0.34, height: 0.12, rotation: 0, opacity: 1, zIndex: 2 }
previousLayout.dialogue.content.transform = { x: 0.075, y: 0.3, width: 0.85, height: 0.56, rotation: 0, opacity: 1, zIndex: 2 }
assert.deepEqual(normalizeTheaterPresentation(previousLayout), defaults)

const invalid = structuredClone(defaults)
invalid.portrait = layer('portrait', 'portrait')
invalid.portraitDecorations = [layer('same', 'portrait'), layer('same', 'viewport')]
assert.equal(theaterPresentationSchema.safeParse(invalid).success, false)
assert.equal(theaterPresentationSchema.safeParse({ ...defaults, unknown: true }).success, false)
assert.equal(theaterPresentationSchema.safeParse({
  ...defaults,
  dialogue: { ...defaults.dialogue, transform: { ...defaults.dialogue.transform, x: Number.NaN } },
}).success, false)
assert.equal(theaterPresentationSchema.safeParse({
  ...defaults,
  dialogue: { ...defaults.dialogue, speaker: { ...defaults.dialogue.speaker, fontScale: 5 } },
}).success, false)
assert.equal(theaterPresentationSchema.safeParse({
  ...defaults,
  portrait: {
    ...layer('video', 'viewport'),
    media: { ...layer('video', 'viewport').media, kind: 'video', mimeType: 'image/webp' },
  },
}).success, false)
assert.equal(theaterPresentationSchema.safeParse({
  ...defaults,
  portrait: {
    ...layer('animated', 'viewport'),
    media: { ...layer('animated', 'viewport').media, kind: 'animated_image', mimeType: 'image/png' },
  },
}).success, false)

const base = createDefaultTheaterPresentation()
base.portrait = layer('base', 'viewport')
base.portraitDecorations = [layer('base-decoration', 'portrait')]
base.dialogue.textAlign = 'right'

const inherited = resolveTheaterPresentation(base, {})
assert.deepEqual(inherited, base)
assert.notEqual(inherited, base)
assert.notEqual(inherited.portrait, base.portrait)

const cleared = resolveTheaterPresentation(base, {
  portrait: null,
  portraitDecorations: null,
  dialogue: null,
})
assert.equal(cleared.portrait, null)
assert.deepEqual(cleared.portraitDecorations, [])
assert.equal(cleared.dialogue.textAlign, 'left')

const replacement = layer('replacement', 'portrait')
const replaced = resolveTheaterPresentation(base, { portraitDecorations: [replacement] })
assert.deepEqual(replaced.portraitDecorations, [replacement])
assert.equal(replaced.portrait?.id, 'base')
assert.equal(replaced.dialogue.textAlign, 'right')
assert.equal(theaterPresentationPatchSchema.safeParse({ dialogue: null }).success, true)
assert.equal(theaterPresentationPatchSchema.safeParse({ narration: { enabled: true, backdropColor: '#101010', backdropOpacity: 0.75 } }).success, true)
assert.equal(resolveTheaterBackdropColor('#336699', 0.4), 'rgba(51, 102, 153, 0.4)')
assert.equal(resolveTheaterBackdropColor('#FFFFFF', 2), 'rgba(255, 255, 255, 1)')

const normalizedTransform = normalizeTheaterTransform({
  x: -10,
  y: Number.NaN,
  width: 0,
  height: 9,
  rotation: 900,
  opacity: -1,
  zIndex: 100.8,
})
assert.deepEqual(normalizedTransform, {
  x: -1,
  y: 0,
  width: 0.01,
  height: 3,
  rotation: 180,
  opacity: 0,
  zIndex: 100,
})
assert.deepEqual(resolveTheaterTransformStyle({
  x: 0.05,
  y: 0.69,
  width: 0.9,
  height: 0.28,
  rotation: 12,
  opacity: 0.8,
  zIndex: 5,
}), {
  position: 'absolute',
  left: '5%',
  top: '69%',
  width: '90%',
  height: '28%',
  transform: 'rotate(12deg)',
  transformOrigin: 'center center',
  opacity: '0.8',
  zIndex: '5',
})

console.log('theater presentation runtime tests passed')
