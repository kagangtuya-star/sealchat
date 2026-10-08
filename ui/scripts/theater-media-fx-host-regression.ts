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
const rolling = (patch = {}) => ({ ...night(), temporal: { ...createDefaultMediaFxSpec().temporal, scanlineRoll: 0.5, ...patch } })
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
    getContext(): { drawImage(output: unknown): void } | null
  }
  const control = {
    gate: null as Promise<void> | null,
    failSource: false,
    failRaster: false,
    failPresent: false,
    fit: 'cover' as 'cover' | null,
    maxTextureSize: 4096 as number | null,
    gpuAvailable: true,
  }
  let notifyGpu: ((available: boolean) => void) | null = null
  const presented: unknown[] = []
  const node = (tag: string): Node => ({
    tag, props: {}, children: [], style: { display: '' }, width: 300, height: 150,
    getBoundingClientRect: () => ({ width: 300, height: 150 }),
    getContext: () => control.failPresent ? null : ({ drawImage: output => { presented.push(output) } }),
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
  // Temporal player stub: the real player / clock are covered by media-fx-temporal-regression.
  type TemporalPlayerStub = {
    options: { present: (frame: unknown) => boolean, onLiveChange?: (live: boolean) => void }
    baselines: Array<{ source: unknown, width: number, height: number, pixelRatio: number } | null>
    updates: Array<{ temporal: { glitch: number, scanlineRoll: number }, active: boolean }>
    live: boolean
    disposed: boolean
  }
  const temporalPlayers: TemporalPlayerStub[] = []
  const hostProps = shallowRef<Record<string, unknown>>({})
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
      mediaFxGpuSupported: () => control.gpuAvailable,
      mediaFxGpuMaxTextureSize: () => control.maxTextureSize,
      subscribeMediaFxGpuAvailability: (listener: (available: boolean) => void) => {
        notifyGpu = listener
        return () => { notifyGpu = null }
      },
    }
    if (id === '@/features/media-fx/media-fx-temporal') return {
      createMediaFxTemporalPlayer: (options: TemporalPlayerStub['options']) => {
        const player: TemporalPlayerStub & Record<string, unknown> = {
          options, baselines: [], updates: [], live: false, disposed: false,
          setBaseline(baseline: TemporalPlayerStub['baselines'][number]) { player.baselines.push(baseline) },
          update(input: TemporalPlayerStub['updates'][number]) { player.updates.push(input) },
          dispose() { player.disposed = true },
        }
        temporalPlayers.push(player)
        return player
      },
      presentMediaFxTemporalFrame: () => true,
    }
    if (id === './TheaterPresentationMedia.vue') return mediaComponent
    if (id === './theaterPresentationMedia') return require('../src/components/theater-presentation/theaterPresentationMedia')
    if (id === './theaterMediaFxAdvanced') return {
      ...advancedModule,
      readTheaterMediaFit: () => control.fit,
      loadTheaterMediaFxSource: async (image: { attachmentId: string }) => {
        loads.push(image.attachmentId)
        const fail = control.failSource
        if (control.gate) await control.gate
        return fail ? null : { source: image.attachmentId, width: 400, height: 800 }
      },
      rasterizeTheaterMediaFx: (_source: unknown, job: TheaterMediaFxRasterJob) => {
        rasters.push(job)
        return control.failRaster ? null : { width: job.raster.width, height: job.raster.height }
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
  const createApp = () => renderer.createApp({ render: () => h(visualComponent.default, { media: layer().media, mediaFx: layer().mediaFx, ...hostProps.value }) })
  let app = createApp()
  const originalDocument = globalThis.document
  const originalWindow = globalThis.window
  globalThis.document = { createElement: () => ({ canPlayType: () => 'probably', width: 0, height: 0, getContext: () => ({ drawImage: () => undefined }) }) } as unknown as Document
  globalThis.window = { devicePixelRatio: 1 } as unknown as Window & typeof globalThis
  const root = node('root')
  const image = () => root.children[0].children.find(child => child.tag === 'img')!
  const canvas = () => root.children[0].children.find(child => child.tag === 'canvas')!
  const originalHidden = () => (image().props.style as { visibility?: string } | undefined)?.visibility === 'hidden'
  const changeSource = (id: string) => dispatch({ type: 'set-media', target: { kind: 'dialogue-frame' }, media: { ...layer().media, resourceAttachmentId: id } })
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
    assert.equal(originalHidden(), false, 'basic-only layers display normally')
    loadImage()
    await flush()
    const initialImage = image()
    const initialMedia = layer().media
    dispatch({ type: 'set-media-fx', target: { kind: 'dialogue-frame' }, mediaFx: glitch() })
    await nextTick()
    assert.equal(originalHidden(), true, 'pending Advanced never flashes the basic-only image')
    assert.equal(canvas().style.display, 'none')
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
    await nextTick()
    assert.equal(canvas().style.display, '', 'same-source edits retain the previous processed output while pending')
    await flush()
    assert.equal(rasters.length, 2, 'later slider edits still rasterize without a load event')
    assert.equal(rasters[1].spec.advanced.edge, 0.8)
    assert.equal(loads.length, 1, 'unchanged source retains its decoded pixels')

    dispatch({ type: 'set-media', target: { kind: 'dialogue-frame' }, media: { ...layer().media, resourceAttachmentId: 'host-new-primary' } })
    await flush()
    assert.notEqual(image(), initialImage)
    assert.equal(canvas().width, 0, 'actual primary change clears the previous output before loading')
    assert.equal(canvas().style.display, 'none')
    assert.equal(originalHidden(), true, 'a new primary waits without flashing its original')
    loadImage()
    await flush()
    assert.equal(rasters.at(-1)?.attachmentId, 'host-new-primary')

    ;(image().props.onError as () => void)()
    await nextTick()
    assert.equal(canvas().width, 0, 'candidate switch clears stale output before fallback load')
    assert.equal(originalHidden(), true, 'fallback loading also suppresses the unprocessed image')
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

    // Initial mount, including a long decode/proxy wait, must never expose original pixels.
    changeSource('host-first-appearance')
    await nextTick()
    app.unmount()
    app = createApp()
    app.mount(root)
    assert.equal(originalHidden(), true, 'first Advanced appearance is hidden before image load')
    let release!: () => void
    control.gate = new Promise(resolve => { release = resolve })
    loadImage()
    await flush()
    assert.equal(originalHidden(), true, 'slow source acquisition keeps original pixels hidden')
    assert.equal(canvas().style.display, 'none')
    control.gate = null
    release()
    await settle()
    await nextTick()
    assert.equal(canvas().style.display, '')
    assert.equal(originalHidden(), true)

    // Every terminal failure must reveal the fallback even before the first commit.
    for (const failure of ['source', 'raster', 'present', 'fit', 'texture'] as const) {
      control.failSource = failure === 'source'
      control.failRaster = failure === 'raster'
      control.failPresent = failure === 'present'
      control.fit = failure === 'fit' ? null : 'cover'
      control.maxTextureSize = failure === 'texture' ? null : 4096
      changeSource(`host-failure-${failure}`)
      await nextTick()
      assert.equal(originalHidden(), true, `${failure}: a new source starts hidden`)
      loadImage()
      await flush()
      assert.equal(originalHidden(), false, `${failure}: processing failure reveals the DOM fallback`)
      assert.equal(canvas().style.display, 'none')
      if (failure === 'source') {
        const loadCount = loads.length
        app.unmount()
        app = createApp()
        app.mount(root)
        loadImage()
        await flush()
        assert.equal(loads.length, loadCount, 'a source in cooldown is not fetched again')
        assert.equal(originalHidden(), false, 'cooldown remount reveals fallback instead of staying hidden')
      }
    }
    control.failPresent = false
    control.maxTextureSize = 4096

    changeSource('host-retry-raster')
    control.failRaster = true
    await nextTick()
    loadImage()
    await flush()
    assert.equal(originalHidden(), false)
    dispatch({ type: 'set-media-fx', target: { kind: 'dialogue-frame' }, mediaFx: glitch() })
    await nextTick()
    assert.equal(originalHidden(), false, 'retrying a failed source does not blink the visible fallback')
    control.failRaster = false
    await flush()
    assert.equal(canvas().style.display, '', 'a later successful pass replaces the fallback')

    notifyGpu!(false)
    await nextTick()
    assert.equal(originalHidden(), false, 'GPU availability loss reveals the DOM fallback')
    assert.equal(canvas(), undefined)
    notifyGpu!(true)
    await nextTick()
    assert.equal(originalHidden(), true, 'GPU recovery waits for processed output')
    await flush()
    assert.equal(canvas().style.display, '')

    // Failure from a replaced source must not reveal the new source's original pixels.
    let releaseOld!: () => void
    let releaseNew!: () => void
    control.gate = new Promise(resolve => { releaseOld = resolve })
    control.failSource = true
    changeSource('host-stale-failure')
    await nextTick()
    loadImage()
    await flush()
    control.failSource = false
    control.gate = new Promise(resolve => { releaseNew = resolve })
    changeSource('host-current-pending')
    await nextTick()
    loadImage()
    await flush()
    releaseOld()
    await settle()
    await nextTick()
    assert.equal(originalHidden(), true, 'stale failure cannot expose the current pending image')
    assert.equal(canvas().style.display, 'none')
    control.gate = null
    releaseNew()
    await settle()
    await nextTick()
    assert.equal(canvas().style.display, '')

    dispatch({ type: 'set-media-fx', target: { kind: 'dialogue-frame' }, mediaFx: night() })
    await flush()
    assert.equal(originalHidden(), false, 'turning Advanced off restores the basic DOM path')
    assert.equal(canvas(), undefined)

    // --- V3.5 Temporal over the static baseline ---------------------------------------
    const player = temporalPlayers.at(-1)!
    assert.ok(player.updates.every(update => !update.active), 'without temporal content the player never runs')
    let temporalRasters = rasters.length
    dispatch({ type: 'set-media-fx', target: { kind: 'dialogue-frame' }, mediaFx: rolling() })
    await nextTick()
    assert.equal(originalHidden(), true, 'temporal-only waits for its static baseline instead of flashing')
    await flush()
    assert.equal(rasters.length, temporalRasters + 1, 'temporal-only rasterizes the basic look once as the baseline')
    assert.deepEqual(rasters.at(-1)!.spec.temporal, createDefaultMediaFxSpec().temporal, 'temporal never reaches the static raster')
    assert.deepEqual(rasters.at(-1)!.spec.filter, night().filter, 'basic filter is baked into the baseline')
    assert.equal(canvas().style.display, '', 'the baseline is shown')
    const baseline = player.baselines.at(-1)!
    assert.ok(baseline && baseline.width === 300 && baseline.pixelRatio === 1, 'the presented output becomes the baseline')
    assert.equal(player.updates.at(-1)!.active, true, 'the player runs once the baseline is ready')
    assert.equal(player.updates.at(-1)!.temporal.scanlineRoll, 0.5)

    temporalRasters = rasters.length
    const baselineCount = player.baselines.length
    dispatch({ type: 'set-media-fx', target: { kind: 'dialogue-frame' }, mediaFx: rolling({ glitch: 0.3 }) })
    await flush()
    assert.equal(rasters.length, temporalRasters, 'temporal edits never re-raster')
    assert.equal(player.baselines.length, baselineCount, 'temporal edits keep the uploaded baseline')
    assert.equal(player.updates.at(-1)!.temporal.glitch, 0.3)
    assert.equal(player.updates.at(-1)!.active, true)

    // A live failure (or stop) repaints the static baseline; the visual never disappears.
    const presentedBefore = presented.length
    player.options.onLiveChange?.(false)
    assert.equal(presented.at(-1), baseline.source, 'live stop repaints the baseline')
    assert.ok(presented.length > presentedBefore)
    assert.equal(canvas().style.display, '')
    assert.equal(originalHidden(), true)

    // Static edits invalidate the baseline; a new output replaces it.
    dispatch({ type: 'set-media-fx', target: { kind: 'dialogue-frame' }, mediaFx: { ...rolling({ glitch: 0.3 }), advanced: glitch().advanced } })
    await flush()
    assert.equal(rasters.length, temporalRasters + 1, 'advanced change rebuilds the baseline once')
    assert.notEqual(player.baselines.at(-1), baseline, 'a new static output is a new baseline')

    // Reduced motion: temporal stops, the static look stays (advanced keeps its output).
    hostProps.value = { reducedMotion: true }
    await flush()
    assert.equal(player.updates.at(-1)!.active, false, 'reduced motion stops the live tick')
    assert.equal(canvas().style.display, '', 'advanced output stays under reduced motion')
    dispatch({ type: 'set-media-fx', target: { kind: 'dialogue-frame' }, mediaFx: rolling() })
    await flush()
    assert.equal(canvas(), undefined, 'temporal-only under reduced motion keeps the plain DOM path')
    assert.equal(originalHidden(), false)
    hostProps.value = { active: false }
    await flush()
    assert.equal(player.updates.at(-1)!.active, false, 'an inactive layer does no live work')
    hostProps.value = {}
    await flush()
    assert.equal(player.updates.at(-1)!.active, true)
    app.unmount()
    assert.equal(player.disposed, true, 'unmount disposes the player')
    app = createApp()
    app.mount(root)
  } finally {
    app.unmount()
    globalThis.document = originalDocument
    globalThis.window = originalWindow
  }
}
