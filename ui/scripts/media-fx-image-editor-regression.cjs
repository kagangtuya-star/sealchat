// Run with node scripts/media-fx-image-editor-regression.cjs.
// Media FX image editor: full static bake helper, preview scheduler / signatures and
// the bake / preserve export contract of useMessageImageEditor. No Vue mount, browser
// or real WebGL: canvas / GPU / vue-paint are stubbed to observe ordering and data.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')

const srcDir = path.join(__dirname, '../src')
const compile = source => ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
}).outputText

// Minimal CommonJS loader for src TS modules with `@/` and relative specifiers.
const createLoader = (overrides = {}) => {
  const cache = {}
  const resolveFile = (specifier, fromDir) => {
    const base = specifier.startsWith('@/') ? path.join(srcDir, specifier.slice(2)) : path.resolve(fromDir, specifier)
    return base.endsWith('.ts') ? base : `${base}.ts`
  }
  const load = (file) => {
    if (cache[file]) return cache[file].exports
    const module = { exports: {} }
    cache[file] = module
    const localRequire = (specifier) => {
      if (overrides[specifier]) return overrides[specifier]
      if (specifier.startsWith('@/') || specifier.startsWith('.')) {
        const target = resolveFile(specifier, path.dirname(file))
        const key = path.relative(srcDir, target).split(path.sep).join('/').replace(/\.ts$/, '')
        if (overrides[`@/${key}`]) return overrides[`@/${key}`]
        return load(target)
      }
      return require(specifier)
    }
    new Function('exports', 'require', 'module', compile(fs.readFileSync(file, 'utf8')))(module.exports, localRequire, module)
    return module.exports
  }
  return specifier => load(resolveFile(specifier, srcDir))
}

// Canvas stub: records the CSS filter active at draw time and keeps one ImageData.
const createCanvasEnvironment = () => {
  const draws = []
  const document = {
    createElement: () => {
      const canvas = { width: 0, height: 0 }
      const context = {
        filter: 'none',
        drawImage: (source) => {
          draws.push({ filter: context.filter, source })
          canvas.data = new Uint8ClampedArray(Math.max(1, canvas.width * canvas.height) * 4).fill(100)
        },
        getImageData: (_x, _y, width, height) => ({ width, height, data: new Uint8ClampedArray(canvas.data) }),
        putImageData: (imageData) => { canvas.data = new Uint8ClampedArray(imageData.data); canvas.put = (canvas.put || 0) + 1 },
      }
      canvas.getContext = () => context
      return canvas
    },
  }
  return { document, draws }
}

async function run() {
  global.window = undefined
  const env = createCanvasEnvironment()
  global.document = env.document

  // --- full static bake helper -------------------------------------------
  const gpu = { available: true, calls: [] }
  const gpuStub = {
    applyMediaFxGpu: (imageData, advanced, options) => {
      gpu.calls.push({ advanced, options, data: new Uint8ClampedArray(imageData.data) })
      if (!gpu.available) return false
      imageData.data.fill(7)
      return true
    },
  }
  const load = createLoader({ '@/features/media-fx/media-fx-gpu': gpuStub })
  const core = load('@/features/media-fx/media-fx')
  const canvasModule = load('@/features/media-fx/media-fx-canvas')
  const { createDefaultMediaFxSpec, compactMediaFxSpec, mediaFxHasContent } = core
  const {
    bakeMediaFxToCanvas, bakeMediaFxFilterToCanvas, bakeMediaFxToBlob, MediaFxAdvancedBakeError,
    MEDIA_FX_ADVANCED_BAKE_ERROR_MESSAGE,
  } = canvasModule
  const withFx = (patch) => ({ ...createDefaultMediaFxSpec(), ...patch })
  const filter = (patch) => ({ ...createDefaultMediaFxSpec().filter, ...patch })
  const advanced = (patch) => ({ ...createDefaultMediaFxSpec().advanced, ...patch })
  const motion = { preset: 'shake', intensity: 0.8, durationMs: 480, loop: true }
  const size = { width: 4, height: 2 }
  const source = { id: 'source' }

  // CSS filter support is probed through the stub document.
  let canvas = bakeMediaFxToCanvas(source, size, withFx({ filter: filter({ grayscale: 0.5, blurPx: 2 }), advanced: advanced({ rgbSplit: 0.3 }) }))
  assert.equal(env.draws.at(-1).filter, 'grayscale(0.5) blur(2px)', 'basic filter is drawn first')
  assert.equal(gpu.calls.length, 1, 'advanced runs once after the basic filter')
  assert.ok(gpu.calls[0].data.every(value => value === 100), 'GPU receives the basic-filtered canvas pixels')
  assert.equal(canvas.put, 1, 'GPU output is written back to the canvas')
  assert.ok(canvas.data.every(value => value === 7))
  assert.equal(gpu.calls[0].options.pixelRatio, 1)

  bakeMediaFxToCanvas(source, size, withFx({ filter: filter({ blurPx: 4 }), advanced: advanced({ pixelate: 0.5 }) }), { pixelRatio: 0.25 })
  assert.equal(env.draws.at(-1).filter, 'blur(1px)', 'preview pixel ratio scales blur')
  assert.equal(gpu.calls.at(-1).options.pixelRatio, 0.25, 'preview pixel ratio reaches the GPU mapping')
  bakeMediaFxToCanvas(source, size, withFx({ advanced: advanced({ posterize: 0.6, edge: 0.4 }) }))
  assert.deepEqual(gpu.calls.at(-1).advanced, advanced({ posterize: 0.6, edge: 0.4 }), 'v3 effects reach the shared GPU bake')

  const callsBefore = gpu.calls.length
  canvas = bakeMediaFxToCanvas(source, size, withFx({ motion }))
  assert.equal(gpu.calls.length, callsBefore, 'motion-only never touches the GPU')
  assert.equal(env.draws.at(-1).filter, 'none', 'motion is never baked')
  assert.equal(canvas.put, undefined)
  bakeMediaFxFilterToCanvas(source, size, filter({ sepia: 0.4 }))
  assert.equal(env.draws.at(-1).filter, 'sepia(0.4)', 'filter-only helper is unchanged')

  gpu.available = false
  canvas = bakeMediaFxToCanvas(source, size, withFx({ advanced: advanced({ scanline: 0.5 }) }))
  assert.ok(canvas.data.every(value => value === 100), 'optional advanced failure keeps the basic pixels')
  assert.throws(
    () => bakeMediaFxToCanvas(source, size, withFx({ advanced: advanced({ scanline: 0.5 }) }), { requireAdvanced: true }),
    (error) => error instanceof MediaFxAdvancedBakeError && error.message === MEDIA_FX_ADVANCED_BAKE_ERROR_MESSAGE,
    'required advanced failure throws a clear error',
  )
  assert.doesNotThrow(() => bakeMediaFxToCanvas(source, size, withFx({ filter: filter({ contrast: 1.2 }) }), { requireAdvanced: true }), 'no advanced content means nothing is required')
  gpu.available = true

  const blob = { type: 'image/png' }
  assert.equal(await bakeMediaFxToBlob(blob, withFx({ motion })), blob, 'motion-only blob is returned unchanged')
  assert.equal(await bakeMediaFxToBlob(blob, createDefaultMediaFxSpec()), blob)

  // --- compact keeps motion-only specs ------------------------------------
  assert.equal(mediaFxHasContent(withFx({ motion })), true)
  assert.deepEqual(compactMediaFxSpec(withFx({ motion })).motion, motion, 'motion-only must not compact to null')

  // --- preview signatures --------------------------------------------------
  const { mediaFxStaticSignature, mediaFxStaticPreviewSpec, createMediaFxPreviewScheduler } = load('@/features/media-fx/media-fx-preview')
  const base = withFx({ filter: filter({ brightness: 1.2 }), advanced: advanced({ pixelate: 0.3 }) })
  const signature = mediaFxStaticSignature(base)
  assert.equal(mediaFxStaticSignature({ ...base, motion }), signature, 'motion edits never re-rasterize')
  assert.equal(mediaFxStaticSignature({ ...base, motion: { ...motion, intensity: 0.2, durationMs: 900, loop: false } }), signature)
  assert.notEqual(mediaFxStaticSignature({ ...base, filter: filter({ brightness: 1.3 }) }), signature)
  assert.notEqual(mediaFxStaticSignature({ ...base, advanced: advanced({ pixelate: 0.31 }) }), signature)
  assert.notEqual(mediaFxStaticSignature({ ...base, advanced: advanced({ pixelate: 0.3, sharpen: 0.2 }) }), signature, 'v3 effects re-rasterize the preview')
  assert.equal(mediaFxStaticPreviewSpec({ ...base, advanced: advanced({ negative: 0.7 }) }, { filters: true, advanced: true }).advanced.negative, 0.7)
  const staticOnly = mediaFxStaticPreviewSpec({ ...base, motion }, { filters: true, advanced: true })
  assert.equal(staticOnly.motion.preset, 'none', 'static preview spec never carries motion')
  assert.equal(staticOnly.advanced.pixelate, 0.3)
  assert.equal(mediaFxStaticPreviewSpec(base, { filters: true, advanced: false }).advanced.pixelate, 0, 'unsupported advanced is not previewed')
  assert.equal(mediaFxStaticPreviewSpec(base, { filters: false, advanced: true }).filter.brightness, 1)
  assert.equal(base.advanced.pixelate, 0.3, 'preview never mutates the edited spec')

  // --- preview scheduler: coalescing, single-flight and stale results ------
  const timers = []
  const flushTimers = () => timers.splice(0).forEach(timer => { if (!timer.cancelled) timer.callback() })
  const pending = []
  const committed = []
  const discarded = []
  const errors = []
  const scheduler = createMediaFxPreviewScheduler({
    delayMs: 100,
    render: () => new Promise((resolve, reject) => pending.push({ resolve, reject })),
    commit: result => committed.push(result),
    discard: result => discarded.push(result),
    onError: error => errors.push(error),
    setTimer: (callback) => { const timer = { callback, cancelled: false }; timers.push(timer); return timer },
    clearTimer: (timer) => { timer.cancelled = true },
  })
  const tick = () => new Promise(resolve => setImmediate(resolve))
  scheduler.request('a')
  scheduler.request('a')
  scheduler.request('b')
  assert.equal(timers.filter(timer => !timer.cancelled).length, 1, 'rapid requests coalesce into one job')
  flushTimers()
  assert.equal(pending.length, 1)
  scheduler.request('c')
  flushTimers()
  assert.equal(pending.length, 1, 'a newer request never starts a second raster job while one is running')
  pending[0].resolve('result-b')
  await tick()
  assert.deepEqual(discarded, ['result-b'], 'the stale in-flight result is released')
  assert.equal(pending.length, 2, 'the newest queued request starts after the running job settles')
  pending[1].resolve('result-c')
  await tick()
  assert.deepEqual(committed, ['result-c'], 'only the newest result commits')
  scheduler.request('c')
  assert.equal(timers.length, 0, 'same committed key is not re-rendered')

  scheduler.request('error-key')
  flushTimers()
  assert.equal(pending.length, 3)
  pending[2].reject(new Error('preview failed'))
  await tick()
  assert.equal(errors.length, 1)
  scheduler.request('error-key')
  assert.equal(timers.filter(timer => !timer.cancelled).length, 1, 'a failed key can be retried without changing parameters')
  flushTimers()
  assert.equal(pending.length, 4)
  scheduler.invalidate()
  pending[3].resolve('result-invalidated')
  await tick()
  assert.deepEqual(discarded, ['result-b', 'result-invalidated'], 'invalidate discards in-flight output')

  scheduler.request('c')
  flushTimers()
  assert.equal(pending.length, 5, 'invalidate forgets the last key (e.g. preview reopened)')
  scheduler.dispose()
  pending[4].resolve('result-late')
  await tick()
  assert.deepEqual(committed, ['result-c'], 'disposed scheduler never commits')
  assert.deepEqual(discarded, ['result-b', 'result-invalidated', 'result-late'])

  // --- useMessageImageEditor: bake / preserve export contract --------------
  const vue = require('vue')
  global.Image = class {
    set src(_value) { this.naturalWidth = 4; this.naturalHeight = 2; setImmediate(() => this.onload && this.onload()) }
  }
  global.URL.createObjectURL = () => 'blob:stub'
  global.URL.revokeObjectURL = () => {}
  const bakeCalls = []
  const realBake = canvasModule.bakeMediaFxToCanvas
  const editorLoad = createLoader({
    'vue-paint': {
      canvasToBlob: async () => ({ type: 'image/png', size: 1 }),
      exportSvg: () => 'data:image/svg+xml;base64,',
      exportToCanvas: async ({ canvas }) => { canvas.width = 4; canvas.height = 2 },
      useBackground: () => ({}), useCrop: () => ({}), useFreehand: () => ({}), useMove: () => ({}), useRectangle: () => ({}),
    },
    '@/composables/useImageCompressor': { compressImage: async file => file },
    '@/features/media-fx/media-fx-gpu': gpuStub,
    '@/features/media-fx/media-fx-canvas': {
      ...canvasModule,
      bakeMediaFxToCanvas: (...args) => { bakeCalls.push(args); return realBake(...args) },
    },
  })
  const { useMessageImageEditor } = editorLoad('@/composables/useMessageImageEditor')
  const originalWarn = console.warn
  console.warn = () => {} // onUnmounted outside a component only warns.
  const createEditor = (mode, initial) => {
    const file = vue.ref(new File([new Uint8Array([1])], 'a.png', { type: 'image/png' }))
    return useMessageImageEditor(file, { effectMode: () => mode, initialMediaFx: () => initial })
  }
  const stored = withFx({ motion, filter: filter({ grayscale: 0.4 }), advanced: advanced({ rgbSplit: 0.2 }) })
  const preserve = createEditor('preserve', core.normalizeMediaFxSpec(stored))
  await tick(); await tick()
  assert.deepEqual(preserve.mediaFx.value, core.normalizeMediaFxSpec(stored), 'preserve restores the initial spec after the file reset')
  let result = await preserve.exportEditedFile({})
  assert.equal(bakeCalls.length, 0, 'preserve never bakes filter / advanced / motion into the file')
  assert.deepEqual(result.mediaFx, compactMediaFxSpec(stored), 'preserve returns the compacted spec')
  preserve.setMediaFx(withFx({ motion }))
  result = await preserve.exportEditedFile({})
  assert.deepEqual(result.mediaFx.motion, motion, 'motion-only survives preserve export')
  preserve.setMediaFx(createDefaultMediaFxSpec())
  assert.equal((await preserve.exportEditedFile({})).mediaFx, null, 'all-default preserve returns null')

  const bake = createEditor('bake', createDefaultMediaFxSpec())
  await tick(); await tick()
  assert.equal(mediaFxHasContent(bake.mediaFx.value), false, 'bake starts from the default spec')
  bake.setMediaFx(withFx({ motion }))
  result = await bake.exportEditedFile({})
  assert.equal(bakeCalls.length, 0, 'motion alone is never baked')
  assert.equal(result.mediaFx, null, 'bake never returns a spec')
  bake.setMediaFx(withFx({ motion, filter: filter({ sepia: 0.5 }), advanced: advanced({ pixelate: 0.2 }) }))
  result = await bake.exportEditedFile({})
  assert.equal(bakeCalls.length, 1)
  assert.equal(bakeCalls[0][3].requireAdvanced, true, 'bake confirm requires advanced')
  assert.equal(result.mediaFx, null)
  bake.setMediaFx(withFx({ advanced: advanced({ vignette: 0.5, grain: 0.3 }) }))
  await bake.exportEditedFile({})
  assert.equal(bakeCalls.length, 2, 'a v3-only effect is baked')
  assert.equal(bakeCalls[1][2].advanced.grain, 0.3)
  assert.equal(bakeCalls[1][3].requireAdvanced, true, 'v3 effects are required in a bake too')
  gpu.available = false
  await assert.rejects(() => bake.exportEditedFile({}), MediaFxAdvancedBakeError, 'GPU failure never silently drops advanced')
  assert.equal(bake.isSaving.value, false, 'saving state is released after a failed bake')
  gpu.available = true
  console.warn = originalWarn

  console.log('media-fx image editor regression passed')
}

run().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
