import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { compileScript, parse } from '@vue/compiler-sfc'
import { createRenderer, h, nextTick, shallowRef, type Component } from 'vue'
import ts from 'typescript'
import { createDefaultMediaFxSpec } from '../src/features/media-fx/media-fx'
import { createDefaultTheaterPresentation, normalizeTheaterPresentation, type TheaterMediaRef } from '../src/types/theaterPresentation'
import { createTheaterPresentationEditorState, createTheaterVisualLayer, dispatchTheaterEditorCommand } from '../src/components/theater-presentation/theaterPresentationEditorState'
import { createTheaterMediaFxAdvancedController, type TheaterMediaFxRasterJob } from '../src/components/theater-presentation/theaterMediaFxAdvanced'

const still: TheaterMediaRef = { assetId: 'host-still', resourceAttachmentId: 'host-primary', mimeType: 'image/png', kind: 'static_image', width: 400, height: 800 }
const frame = createTheaterVisualLayer(still, 'dialogue', 'frame')
const night = () => ({ ...createDefaultMediaFxSpec(), filter: { ...createDefaultMediaFxSpec().filter, brightness: 0.6 } })
const glitch = () => ({ ...night(), advanced: { ...createDefaultMediaFxSpec().advanced, rgbSplit: 0.4, edge: 0.3, grain: 0.2 } })
const settle = () => new Promise(resolve => setImmediate(resolve))

// Mount the real host and media component together for source-lifecycle regression.
// Pixel work is controlled, but
// Vue owns prop replacement, source watchers, keyed images, refs and output visibility.
export const runAdvancedHostTests = async () => {
  type Node = {
    tag: string
    props: Record<string, unknown>
    children: Node[]
    parent?: Node
    style: { display: string }
    width: number
    height: number
    getBoundingClientRect(): { width: number, height: number }
    getContext(): { drawImage(output: unknown): void }
  }
  const presented: unknown[] = []
  const node = (tag: string): Node => ({
    tag, props: {}, children: [], style: { display: '' }, width: 300, height: 150,
    getBoundingClientRect: () => ({ width: 300, height: 150 }),
    getContext: () => ({ drawImage: output => { presented.push(output) } }),
  })
  const renderer = createRenderer<Node, Node>({
    createElement: node,
    createText: () => node('#text'),
    createComment: () => node('#comment'),
    setText: () => undefined,
    setElementText: () => undefined,
    patchProp: (el, key, _old, value) => { el.props[key] = value },
    insert: (el, parent, anchor) => {
      if (el.parent) el.parent.children = el.parent.children.filter(child => child !== el)
      el.parent = parent
      const index = anchor ? parent.children.indexOf(anchor) : -1
      if (index < 0) parent.children.push(el)
      else parent.children.splice(index, 0, el)
    },
    remove: (el) => { if (el.parent) el.parent.children = el.parent.children.filter(child => child !== el) },
    parentNode: el => el.parent || null,
    nextSibling: el => el.parent?.children[el.parent.children.indexOf(el) + 1] || null,
  })
  const timers: Array<() => void> = []
  const rasters: TheaterMediaFxRasterJob[] = []
  const loads: string[] = []
  const advancedModule = require('../src/components/theater-presentation/theaterMediaFxAdvanced')
  let mediaComponent: { default: Component }
  const localRequire = (id: string): unknown => {
    if (id === '@/stores/display') return { useDisplayStore: () => ({ settings: { preferStaticAvatarDecoration: false } }) }
    if (id === '@/composables/useAttachmentResolver') return { resolveAttachmentUrl: (id: string) => `https://cdn.test/${id}.png` }
    if (id === '@/features/media-fx/media-fx-dom') return { vMediaFx: {} }
    if (id === '@/features/media-fx/media-fx-gpu') return {
      mediaFxGpuSupported: () => true,
      mediaFxGpuMaxTextureSize: () => 4096,
      subscribeMediaFxGpuAvailability: () => () => undefined,
    }
    if (id === './TheaterPresentationMedia.vue') return mediaComponent
    if (id === './theaterPresentationMedia') return require('../src/components/theater-presentation/theaterPresentationMedia')
    if (id === './theaterMediaFxAdvanced') return {
      ...advancedModule,
      readTheaterMediaFit: () => 'cover',
      loadTheaterMediaFxSource: async (image: { attachmentId: string }) => {
        loads.push(image.attachmentId)
        return { source: image.attachmentId, width: 400, height: 800 }
      },
      rasterizeTheaterMediaFx: (_source: unknown, job: TheaterMediaFxRasterJob) => {
        rasters.push(job)
        return { width: job.raster.width, height: job.raster.height }
      },
      createTheaterMediaFxAdvancedController: (deps: Parameters<typeof createTheaterMediaFxAdvancedController>[0]) => createTheaterMediaFxAdvancedController({
        ...deps,
        setTimer: callback => { timers.push(callback); return callback },
        clearTimer: handle => {
          const index = timers.indexOf(handle as () => void)
          if (index >= 0) timers.splice(index, 1)
        },
      }),
    }
    return require(id)
  }
  const compileComponent = (file: string) => {
    const filename = path.join(__dirname, '../src/components/theater-presentation', file)
    const { descriptor } = parse(fs.readFileSync(filename, 'utf8'))
    const compiled = compileScript(descriptor, { id: 'advanced-host-regression', inlineTemplate: true })
    const module = { exports: {} as { default: Component } }
    new Function('exports', 'require', 'module', ts.transpileModule(compiled.content, {
      compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
    }).outputText)(module.exports, localRequire, module)
    return module.exports
  }
  mediaComponent = compileComponent('TheaterPresentationMedia.vue')
  const visualComponent = compileComponent('TheaterMediaFxVisual.vue')
  const presentation = createDefaultTheaterPresentation()
  presentation.dialogue.frame = { ...frame, media: { ...still, fallbackAttachmentId: 'host-fallback' }, mediaFx: night() }
  const state = shallowRef(createTheaterPresentationEditorState({ mode: 'base', presentation }))
  const layer = () => state.value.draft.dialogue.frame!
  const dispatch = (command: Parameters<typeof dispatchTheaterEditorCommand>[1]) => {
    state.value = dispatchTheaterEditorCommand(state.value, command, { recordHistory: false })
  }
  const app = renderer.createApp({ render: () => h(visualComponent.default, { media: layer().media, mediaFx: layer().mediaFx }) })
  const originalDocument = globalThis.document
  const originalWindow = globalThis.window
  globalThis.document = { createElement: () => ({ canPlayType: () => 'probably' }) } as unknown as Document
  globalThis.window = { devicePixelRatio: 1 } as unknown as Window & typeof globalThis
  const root = node('root')
  const image = () => root.children[0].children.find(child => child.tag === 'img')!
  const canvas = () => root.children[0].children.find(child => child.tag === 'canvas')!
  const loadImage = () => {
    const img = image()
    const element = { complete: true, naturalWidth: 400, naturalHeight: 800, currentSrc: img.props.src, dataset: { attachmentId: img.props['data-attachment-id'] } }
    ;(img.props.onLoad as (event: { target: unknown }) => void)({ target: element })
  }
  const flush = async () => {
    await nextTick()
    for (let round = 0; round < 10; round += 1) {
      const callback = timers.shift()
      if (!callback) break
      callback()
      await settle()
      await nextTick()
    }
  }
  try {
    app.mount(root)
    loadImage()
    await flush()
    const initialImage = image()
    const initialMedia = layer().media
    dispatch({ type: 'set-media-fx', target: { kind: 'dialogue-frame' }, mediaFx: glitch() })
    await flush()
    assert.notEqual(layer().media, initialMedia, 'real editor dispatch replaces the media object')
    assert.equal(image(), initialImage, 'unchanged attachment reuses the image without another load')
    assert.equal(rasters.length, 1, 'enabling Advanced after an editor clone must rasterize the loaded image')
    assert.deepEqual(rasters[0].spec.advanced, glitch().advanced)
    assert.equal(canvas().style.display, '', 'successful output is visible')
    assert.equal((image().props.style as { visibility?: string }).visibility, 'hidden')

    // The overlay's async presentation replacement has the same new-object / same-ID shape.
    state.value = { ...state.value, draft: normalizeTheaterPresentation(JSON.parse(JSON.stringify(state.value.draft))) }
    await flush()
    assert.equal(image(), initialImage)
    assert.equal(rasters.length, 1, 'equivalent live presentation keeps the existing output')
    assert.equal(canvas().width, 300)
    assert.equal(canvas().style.display, '')

    dispatch({ type: 'set-media-fx', target: { kind: 'dialogue-frame' }, mediaFx: { ...glitch(), advanced: { ...glitch().advanced, edge: 0.8 } } })
    await flush()
    assert.equal(rasters.length, 2, 'later slider edits still rasterize without a load event')
    assert.equal(rasters[1].spec.advanced.edge, 0.8)
    assert.equal(loads.length, 1, 'unchanged source retains its decoded pixels')

    dispatch({ type: 'set-media', target: { kind: 'dialogue-frame' }, media: { ...layer().media, resourceAttachmentId: 'host-new-primary' } })
    await flush()
    assert.notEqual(image(), initialImage)
    assert.equal(canvas().width, 0, 'actual primary change clears the previous output before loading')
    assert.equal(canvas().style.display, 'none')
    loadImage()
    await flush()
    assert.equal(rasters.at(-1)?.attachmentId, 'host-new-primary')

    ;(image().props.onError as () => void)()
    await nextTick()
    loadImage()
    await flush()
    const fallbackImage = image()
    const rasterCount = rasters.length
    assert.equal(rasters.at(-1)?.attachmentId, 'host-fallback')
    state.value = { ...state.value, draft: normalizeTheaterPresentation(JSON.parse(JSON.stringify(state.value.draft))) }
    await flush()
    assert.equal(image(), fallbackImage, 'equivalent presentation must retain the active fallback candidate')
    assert.equal(rasters.length, rasterCount)
    assert.equal(canvas().style.display, '')

    dispatch({ type: 'set-media', target: { kind: 'dialogue-frame' }, media: { ...layer().media, fallbackAttachmentId: 'host-new-fallback' } })
    await flush()
    assert.equal(canvas().width, 0, 'actual fallback change still invalidates the loaded source')
    assert.equal(canvas().style.display, 'none')
    assert.equal(image().props['data-attachment-id'], 'host-new-primary')
    assert.ok(presented.length >= 4)
  } finally {
    app.unmount()
    globalThis.document = originalDocument
    globalThis.window = originalWindow
  }
}
