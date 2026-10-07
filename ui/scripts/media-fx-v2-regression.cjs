// Run with node scripts/media-fx-v2-regression.cjs.
// MediaFxSpec v2 core / strict schema / GPU parameter mapping and the Konva filter
// composition. Real WebGL is not available in Node; the GPU shader itself is covered
// by the browser smoke test, here the GPU step is stubbed to check ordering and caching.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')

const featureDir = path.join(__dirname, '../src/features/media-fx')
const compile = source => ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
}).outputText

// Minimal CommonJS loader for the feature's TS modules; `overrides` replaces modules
// by specifier so the Konva adapter can run against stubbed capabilities.
const loadModule = (name, overrides = {}, cache = {}) => {
  if (overrides[name]) return overrides[name]
  if (cache[name]) return cache[name].exports
  const module = { exports: {} }
  cache[name] = module
  const localRequire = (specifier) => {
    if (overrides[specifier]) return overrides[specifier]
    if (specifier.startsWith('./')) return loadModule(specifier, overrides, cache)
    return require(specifier)
  }
  const source = fs.readFileSync(path.join(featureDir, `${name.slice(2)}.ts`), 'utf8')
  new Function('exports', 'require', 'module', compile(source))(module.exports, localRequire, module)
  return module.exports
}

async function run() {
  const core = loadModule('./media-fx')
  const {
    MEDIA_FX_VERSION, LEGACY_MEDIA_FX_VERSION, createDefaultMediaFxSpec, createDefaultMediaFxAdvanced,
    normalizeMediaFxSpec, normalizeMediaFxAdvanced, mediaFxAdvancedHasContent, mediaFxHasContent,
    compactMediaFxSpec, mediaFxSpecsEqual, resolveMediaFxCapabilities, mediaFxAdvancedFromPreset,
    matchMediaFxAdvancedPreset,
  } = core
  assert.equal(MEDIA_FX_VERSION, 2)
  assert.equal(LEGACY_MEDIA_FX_VERSION, 1)

  // --- core: v1 -> v2 -------------------------------------------------------
  const motion = { preset: 'breathe', intensity: 0.6, durationMs: 4000, loop: false }
  const filter = { ...createDefaultMediaFxSpec().filter, brightness: 0.7, sepia: 0.3 }
  const v1 = { version: 1, motion, filter }
  const upgraded = normalizeMediaFxSpec(v1)
  assert.deepEqual(upgraded, { version: 2, motion, filter, advanced: { pixelate: 0, rgbSplit: 0, scanline: 0 } })
  assert.deepEqual(normalizeMediaFxSpec({ motion, filter }), upgraded, 'version-less data reads as legacy v1')
  assert.deepEqual(normalizeMediaFxSpec({ ...v1, advanced: { pixelate: 1 } }).advanced, createDefaultMediaFxAdvanced(), 'v1 never carries advanced')
  assert.deepEqual(normalizeMediaFxSpec({ ...v1, version: 3 }), createDefaultMediaFxSpec(), 'unknown version falls back to default')
  assert.deepEqual(normalizeMediaFxSpec({ ...v1, version: '2' }), createDefaultMediaFxSpec())

  const v2 = { version: 2, motion, filter, advanced: { pixelate: 0.45, rgbSplit: 0.123456, scanline: 0.5 } }
  assert.deepEqual(normalizeMediaFxSpec(v2).advanced, { pixelate: 0.45, rgbSplit: 0.123, scanline: 0.5 })
  assert.deepEqual(normalizeMediaFxAdvanced({ pixelate: 4, rgbSplit: -1, scanline: Number.NaN, shader: 'x' }), { pixelate: 1, rgbSplit: 0, scanline: 0 })
  assert.deepEqual(normalizeMediaFxAdvanced({ pixelate: Infinity, rgbSplit: '0.5', scanline: 0.0004 }), { pixelate: 0, rgbSplit: 0, scanline: 0 })
  assert.equal('shader' in normalizeMediaFxSpec({ ...v2, shader: 'x' }), false)

  // --- core: content / compact / equality ----------------------------------
  const advancedOnly = { ...createDefaultMediaFxSpec(), advanced: { pixelate: 0, rgbSplit: 0.2, scanline: 0 } }
  assert.equal(mediaFxAdvancedHasContent(advancedOnly.advanced), true)
  assert.equal(mediaFxHasContent(advancedOnly), true)
  assert.deepEqual(compactMediaFxSpec(advancedOnly), advancedOnly, 'advanced-only spec must survive compaction')
  assert.equal(compactMediaFxSpec(createDefaultMediaFxSpec()), null, 'all-default v2 compacts to null')
  assert.equal(compactMediaFxSpec({ version: 1, motion: createDefaultMediaFxSpec().motion, filter: createDefaultMediaFxSpec().filter }), null)
  assert.equal(mediaFxSpecsEqual(upgraded, { ...upgraded, advanced: createDefaultMediaFxAdvanced() }), true)
  assert.equal(mediaFxSpecsEqual(normalizeMediaFxSpec(v1), normalizeMediaFxSpec({ ...v1, version: 2, advanced: { pixelate: 0, rgbSplit: 0, scanline: 0 } })), true)
  assert.equal(mediaFxSpecsEqual(upgraded, { ...upgraded, advanced: { ...upgraded.advanced, scanline: 0.1 } }), false)

  // --- presets fill values only --------------------------------------------
  assert.deepEqual(mediaFxAdvancedFromPreset('glitch'), { pixelate: 0, rgbSplit: 0.35, scanline: 0.18 })
  assert.equal(matchMediaFxAdvancedPreset({ pixelate: 0.45, rgbSplit: 0, scanline: 0 }), 'low-res')
  assert.equal(matchMediaFxAdvancedPreset(createDefaultMediaFxAdvanced()), 'none')
  assert.equal(mediaFxAdvancedFromPreset('missing'), null)

  // --- capabilities ---------------------------------------------------------
  assert.equal(resolveMediaFxCapabilities('konva').advanced, false, 'existing callers never gain advanced')
  assert.equal(resolveMediaFxCapabilities('dom', false).advanced, false)
  assert.equal(resolveMediaFxCapabilities('bake').advanced, false)
  assert.equal(resolveMediaFxCapabilities('konva', false, true).advanced, true)
  assert.equal(resolveMediaFxCapabilities('konva', true, true).advanced, false, 'animated media never runs advanced')

  // --- strict zod schema ----------------------------------------------------
  const { mediaFxSpecSchema } = loadModule('./media-fx-schema')
  assert.deepEqual(mediaFxSpecSchema.parse(v1), upgraded, 'strict v1 parses into canonical v2')
  const canonicalV2 = normalizeMediaFxSpec(v2)
  assert.deepEqual(mediaFxSpecSchema.parse(canonicalV2), canonicalV2)
  const rejected = {
    'v2 missing advanced': { version: 2, motion, filter },
    'pixelate > 1': { ...canonicalV2, advanced: { ...canonicalV2.advanced, pixelate: 1.01 } },
    'rgbSplit < 0': { ...canonicalV2, advanced: { ...canonicalV2.advanced, rgbSplit: -0.01 } },
    'scanline NaN': { ...canonicalV2, advanced: { ...canonicalV2.advanced, scanline: Number.NaN } },
    'advanced.shader': { ...canonicalV2, advanced: { ...canonicalV2.advanced, shader: 'void main(){}' } },
    'v1 with advanced': { ...v1, advanced: createDefaultMediaFxAdvanced() },
    'v1 unknown field': { ...v1, glsl: 'x' },
    'v2 unknown field': { ...canonicalV2, uniforms: {} },
    'filter unknown field': { ...canonicalV2, filter: { ...filter, fragmentShader: 'x' } },
    'unknown version': { ...canonicalV2, version: 3 },
    'missing version': { motion, filter },
  }
  for (const [name, value] of Object.entries(rejected)) {
    assert.equal(mediaFxSpecSchema.safeParse(value).success, false, `schema must reject ${name}`)
  }

  // --- GPU module in a context without WebGL --------------------------------
  const gpu = loadModule('./media-fx-gpu')
  assert.equal(gpu.mediaFxGpuSupported(), false)
  assert.equal(gpu.createMediaFxGpuFilter({ pixelate: 0.5, rgbSplit: 0, scanline: 0 }), null)
  const pixels = { width: 1, height: 1, data: new Uint8ClampedArray([10, 20, 30, 40]) }
  assert.equal(gpu.applyMediaFxGpu(pixels, { pixelate: 0, rgbSplit: 1, scanline: 0 }), false)
  assert.deepEqual([...pixels.data], [10, 20, 30, 40], 'failure keeps the source pixels')
  const params = gpu.resolveMediaFxGpuParams
  assert.equal(params({ pixelate: 0, rgbSplit: 0, scanline: 0 }).blockPx, 1)
  assert.equal(params({ pixelate: 1, rgbSplit: 0, scanline: 0 }).blockPx, 48)
  assert.equal(params({ pixelate: 0.5, rgbSplit: 0, scanline: 0 }).blockPx, 13)
  assert.equal(params({ pixelate: 0.5, rgbSplit: 0, scanline: 0 }, { pixelRatio: 2 }).blockPx, 26)
  assert.equal(params({ pixelate: 1, rgbSplit: 0, scanline: 0 }, { pixelRatio: 2 }).blockPx, 96)
  assert.equal(params({ pixelate: 0, rgbSplit: 1, scanline: 0 }).rgbSplitPx, 12)
  assert.equal(params({ pixelate: 0, rgbSplit: 0.25, scanline: 0 }, { pixelRatio: 2 }).rgbSplitPx, 6)
  assert.equal(params({ pixelate: 0, rgbSplit: 0, scanline: 1 }).scanlineDarken, 0.25)

  // Lazy capability starts optimistic when WebGL2 exists, then becomes unavailable
  // and notifies consumers if actual context creation fails.
  const originalDocument = global.document
  const originalWebGL2 = global.WebGL2RenderingContext
  global.WebGL2RenderingContext = function WebGL2RenderingContext() {}
  global.document = { createElement: () => ({ width: 0, height: 0, getContext: () => null }) }
  const failingGpu = loadModule('./media-fx-gpu', {}, {})
  assert.equal(failingGpu.mediaFxGpuSupported(), true)
  const availability = []
  const unsubscribeAvailability = failingGpu.subscribeMediaFxGpuAvailability(value => availability.push(value))
  const failingStep = failingGpu.createMediaFxGpuFilter({ pixelate: 0.5, rgbSplit: 0, scanline: 0 })
  assert.equal(typeof failingStep, 'function')
  failingStep({ width: 1, height: 1, data: new Uint8ClampedArray([1, 2, 3, 4]) })
  assert.deepEqual(availability, [false])
  assert.equal(failingGpu.mediaFxGpuSupported(), false)
  unsubscribeAvailability()
  if (originalDocument === undefined) delete global.document
  else global.document = originalDocument
  if (originalWebGL2 === undefined) delete global.WebGL2RenderingContext
  else global.WebGL2RenderingContext = originalWebGL2

  // --- Konva adapter composition -------------------------------------------
  const Konva = (await import('konva')).default
  const capability = { canvas: true, gpu: true }
  const gpuCalls = []
  const gpuStub = {
    mediaFxGpuSupported: () => capability.gpu,
    createMediaFxGpuFilter: (advanced, options) => {
      if (!capability.gpu || !mediaFxAdvancedHasContent(advanced)) return null
      gpuCalls.push({ advanced, options })
      const step = () => {}
      step.gpu = advanced
      return step
    },
  }
  const { createKonvaMediaFxController, mediaFxAdvancedSignature, composeKonvaMediaFxFilters } = loadModule('./media-fx-konva', {
    konva: { __esModule: true, default: Konva },
    './media-fx-gpu': gpuStub,
    './media-fx-canvas': { canvasFilterSupported: () => capability.canvas },
  })
  assert.equal(mediaFxAdvancedSignature({ pixelate: 0.1, rgbSplit: 0.2, scanline: 0.3 }), '0.1|0.2|0.3')
  const basicStep = () => {}
  const gpuStep = () => {}
  assert.deepEqual(composeKonvaMediaFxFilters([basicStep], gpuStep), [basicStep, gpuStep])
  assert.deepEqual(composeKonvaMediaFxFilters([basicStep], null), [basicStep])

  const motionNode = new Konva.Group()
  const imageNode = new Konva.Image({ image: { width: 200, height: 100 }, width: 200, height: 100 })
  motionNode.add(imageNode)
  let caches = 0
  imageNode.cache = () => { caches += 1; return imageNode }
  const controller = createKonvaMediaFxController({ motionNode, imageNode })
  const context = { width: 200, height: 100, motion: false, filters: true, advanced: true }
  const withFx = (patch) => ({ ...createDefaultMediaFxSpec(), ...patch })
  const isGpu = (step) => typeof step === 'function' && Boolean(step.gpu)

  controller.update(createDefaultMediaFxSpec(), context)
  assert.deepEqual(imageNode.filters() ?? [], [], 'all-default v2 adds no filters')
  assert.equal(caches, 0, 'all-default v2 builds no cache')
  assert.equal(gpuCalls.length, 0, 'all-default v2 never touches the GPU')

  const grayscaleSplit = withFx({ filter: { ...createDefaultMediaFxSpec().filter, grayscale: 0.5 }, advanced: { pixelate: 0, rgbSplit: 0.3, scanline: 0 } })
  controller.update(grayscaleSplit, context)
  let filters = imageNode.filters()
  assert.equal(filters.length, 2)
  assert.equal(typeof filters[0], 'function', 'basic CSS runs as a function step when a GPU step follows')
  assert.equal(isGpu(filters[0]), false)
  assert.equal(isGpu(filters[1]), true, 'GPU step is last')
  assert.equal(caches, 1)
  assert.ok(gpuCalls.at(-1).options.pixelRatio > 0, 'GPU step receives the cache pixel ratio')

  controller.update(grayscaleSplit, context)
  assert.equal(caches, 1, 'same parameters do not rebuild the cache')
  controller.update(withFx({ ...grayscaleSplit, advanced: { pixelate: 0, rgbSplit: 0.31, scanline: 0 } }), context)
  assert.equal(caches, 2, 'advanced change rebuilds the cache')

  controller.update(withFx({ advanced: { pixelate: 0.4, rgbSplit: 0, scanline: 0 } }), context)
  filters = imageNode.filters()
  assert.equal(filters.length, 1)
  assert.equal(isGpu(filters[0]), true, 'advanced-only uses only the GPU step')

  capability.canvas = false
  controller.update(withFx({ filter: { ...createDefaultMediaFxSpec().filter, brightness: 1.4 }, advanced: { pixelate: 0, rgbSplit: 0, scanline: 0.5 } }), context)
  filters = imageNode.filters()
  assert.equal(filters[0], Konva.Filters.Brightness, 'canvas-filter fallback still applies')
  assert.equal(isGpu(filters.at(-1)), true, 'GPU still runs after fallback filters')
  capability.canvas = true

  capability.gpu = false
  const blurPixelate = withFx({ filter: { ...createDefaultMediaFxSpec().filter, blurPx: 2 }, advanced: { pixelate: 0.5, rgbSplit: 0, scanline: 0 } })
  controller.update(blurPixelate, context)
  assert.deepEqual(imageNode.filters(), ['blur(2px)'], 'without GPU the basic native filter path is unchanged')
  const cachesBefore = caches
  controller.update(blurPixelate, context)
  assert.equal(caches, cachesBefore)
  controller.update(withFx({ advanced: { pixelate: 0.5, rgbSplit: 0, scanline: 0 } }), context)
  assert.deepEqual(imageNode.filters(), [], 'advanced-only without GPU leaves the image unfiltered')
  capability.gpu = true

  const callsBefore = gpuCalls.length
  controller.update(blurPixelate, { ...context, advanced: false })
  assert.deepEqual(imageNode.filters(), ['blur(2px)'], 'consumers without advanced never run the GPU')
  controller.update(blurPixelate, { ...context, filters: false })
  assert.deepEqual(imageNode.filters(), [], 'animated media (filters off) never runs the GPU')
  assert.equal(gpuCalls.length, callsBefore)

  controller.dispose()
  console.log('media-fx v2 regression passed')
}

run().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
