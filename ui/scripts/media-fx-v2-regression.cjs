// Run with node scripts/media-fx-v2-regression.cjs.
// MediaFxSpec v1 / v2 / v3 / v4 -> v5 core / strict schema / GPU parameter mapping and the Konva filter
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
    matchMediaFxAdvancedPreset, createDefaultMediaFxTemporal, mediaFxV3AdvancedKeys, MEDIA_FX_V4_VERSION,
  } = core
  assert.equal(MEDIA_FX_VERSION, 5)
  assert.equal(MEDIA_FX_V4_VERSION, 4)
  const temporalOff = createDefaultMediaFxTemporal()
  assert.equal(LEGACY_MEDIA_FX_VERSION, 1)
  const off = createDefaultMediaFxAdvanced()
  const v3Keys = ['vignette', 'grain', 'posterize', 'negative', 'sharpen', 'edge']
  const lightKeys = ['bloom', 'glow']
  assert.deepEqual(Object.keys(off), ['pixelate', 'rgbSplit', 'scanline', ...v3Keys, ...lightKeys])
  assert.deepEqual([...mediaFxV3AdvancedKeys], ['pixelate', 'rgbSplit', 'scanline', ...v3Keys], 'v3 / v4 carry the nine original effects')
  assert.ok(Object.values(off).every(value => value === 0), 'every advanced default means off')
  const adv = patch => ({ ...off, ...patch })

  // --- core: v1 -> v3 -------------------------------------------------------
  const motion = { preset: 'breathe', intensity: 0.6, durationMs: 4000, loop: false }
  const filter = { ...createDefaultMediaFxSpec().filter, brightness: 0.7, sepia: 0.3 }
  const v1 = { version: 1, motion, filter }
  const upgraded = normalizeMediaFxSpec(v1)
  assert.deepEqual(upgraded, { version: 5, motion, filter, advanced: off, temporal: temporalOff })
  assert.deepEqual(normalizeMediaFxSpec({ motion, filter }), upgraded, 'version-less data reads as legacy v1')
  assert.deepEqual(normalizeMediaFxSpec({ ...v1, advanced: { pixelate: 1 } }).advanced, off, 'v1 never carries advanced')
  assert.deepEqual(normalizeMediaFxSpec({ ...v1, version: 6 }), createDefaultMediaFxSpec(), 'unknown version falls back to default')
  assert.deepEqual(normalizeMediaFxSpec({ ...v1, version: '2' }), createDefaultMediaFxSpec())
  assert.deepEqual(normalizeMediaFxSpec({ ...v1, version: '3' }), createDefaultMediaFxSpec())

  // --- core: v2 -> v3 keeps the three v2 effects and nothing else -------------
  const v2 = { version: 2, motion, filter, advanced: { pixelate: 0.45, rgbSplit: 0.123456, scanline: 0.5 } }
  const v2Upgraded = normalizeMediaFxSpec(v2)
  assert.deepEqual(v2Upgraded, { version: 5, motion, filter, advanced: adv({ pixelate: 0.45, rgbSplit: 0.123, scanline: 0.5 }), temporal: temporalOff })
  assert.deepEqual(normalizeMediaFxSpec({ ...v2, advanced: { ...v2.advanced, grain: 0.8, edge: 1, bloom: 1 } }), v2Upgraded, 'v2 never reads v3+ fields')

  // --- core: v3 normalize / clamp / finite --------------------------------------
  const v3Advanced = adv({ pixelate: 0.2, vignette: 0.4, grain: 0.123456, posterize: 0.6, negative: 0.5, sharpen: 0.3, edge: 0.7 })
  assert.deepEqual(normalizeMediaFxSpec({ version: 3, motion, filter, advanced: v3Advanced }).advanced, { ...v3Advanced, grain: 0.123 })

  // --- core: v3 / v4 never read bloom / glow; v5 reads all eleven ---------------
  const temporalOn = { ...temporalOff, glitch: 0.3, speed: 2 }
  const v4 = { version: 4, motion, filter, advanced: v3Advanced, temporal: temporalOn }
  const v4Upgraded = normalizeMediaFxSpec(v4)
  assert.deepEqual(v4Upgraded, { version: 5, motion, filter, advanced: { ...v3Advanced, grain: 0.123 }, temporal: temporalOn }, 'v4 -> v5 keeps advanced and temporal')
  assert.equal(v4Upgraded.advanced.bloom, 0)
  assert.equal(v4Upgraded.advanced.glow, 0)
  assert.deepEqual(normalizeMediaFxSpec({ ...v4, advanced: { ...v3Advanced, bloom: 0.9, glow: 0.4 } }), v4Upgraded, 'v4 never reads bloom / glow')
  assert.deepEqual(normalizeMediaFxSpec({ version: 3, motion, filter, advanced: { ...v3Advanced, bloom: 1 } }).advanced.bloom, 0, 'v3 never reads bloom')
  const v5 = { version: 5, motion, filter, advanced: { ...v3Advanced, grain: 0.123, bloom: 0.6, glow: 0.25 }, temporal: temporalOn }
  assert.deepEqual(normalizeMediaFxSpec(v5), v5, 'canonical v5 is a fixed point')
  assert.deepEqual(normalizeMediaFxAdvanced({ bloom: 2, glow: -1, threshold: 0.5 }), adv({ bloom: 1 }), 'bloom / glow clamp to 0..1, renderer params are dropped')
  assert.equal(normalizeMediaFxAdvanced({ bloom: 0.0004, glow: Number.NaN }).bloom, 0, 'near-zero bloom is off')

  assert.deepEqual(
    normalizeMediaFxAdvanced({ pixelate: 4, rgbSplit: -1, scanline: Number.NaN, vignette: 2, grain: -0.5, posterize: Infinity, negative: '1', sharpen: 0.0004, edge: 1.0000001, shader: 'x' }),
    adv({ pixelate: 1, vignette: 1, edge: 1 }),
  )
  assert.equal('shader' in normalizeMediaFxSpec({ ...v2, shader: 'x' }), false)
  assert.equal('uniforms' in normalizeMediaFxSpec({ version: 3, motion, filter, advanced: { ...v3Advanced, uniforms: {} } }).advanced, false)

  // --- core: content / compact / equality ----------------------------------
  const advancedOnly = { ...createDefaultMediaFxSpec(), advanced: adv({ rgbSplit: 0.2 }) }
  assert.equal(mediaFxAdvancedHasContent(advancedOnly.advanced), true)
  assert.equal(mediaFxHasContent(advancedOnly), true)
  assert.deepEqual(compactMediaFxSpec(advancedOnly), advancedOnly, 'advanced-only spec must survive compaction')
  for (const key of [...v3Keys, ...lightKeys]) {
    const only = { ...createDefaultMediaFxSpec(), advanced: adv({ [key]: 0.3 }) }
    assert.equal(mediaFxHasContent(only), true, `${key} alone counts as content`)
    assert.deepEqual(compactMediaFxSpec(only), only, `${key}-only spec must survive compaction`)
  }
  assert.equal(compactMediaFxSpec({ ...createDefaultMediaFxSpec(), advanced: adv({ bloom: 0, glow: 0 }) }), null, 'bloom / glow at 0 never keep a spec alive')
  assert.equal(compactMediaFxSpec({ ...createDefaultMediaFxSpec(), advanced: adv({ glow: 0.0004 }) }), null, 'near-zero glow compacts away')
  assert.equal(mediaFxSpecsEqual(v4Upgraded, normalizeMediaFxSpec({ ...v5, advanced: { ...v5.advanced, bloom: 0, glow: 0 } })), true, 'v4 equals the v5 spec with bloom / glow at 0')
  assert.equal(mediaFxSpecsEqual(normalizeMediaFxSpec({ version: 3, motion, filter, advanced: v3Advanced }), normalizeMediaFxSpec({ ...v5, temporal: temporalOff, advanced: { ...v5.advanced, bloom: 0, glow: 0 } })), true, 'v3 equals the equivalent v5')
  assert.equal(mediaFxSpecsEqual(v4Upgraded, normalizeMediaFxSpec(v5)), false, 'bloom / glow make specs differ')
  assert.equal(mediaFxSpecsEqual(normalizeMediaFxSpec(v5), normalizeMediaFxSpec({ ...v5, advanced: { ...v5.advanced, glow: 0.26 } })), false)
  assert.equal(compactMediaFxSpec(createDefaultMediaFxSpec()), null, 'all-default v3 compacts to null')
  assert.equal(compactMediaFxSpec({ version: 3, motion: createDefaultMediaFxSpec().motion, filter: createDefaultMediaFxSpec().filter, advanced: adv({ grain: 0.0004 }) }), null)
  assert.equal(compactMediaFxSpec({ version: 1, motion: createDefaultMediaFxSpec().motion, filter: createDefaultMediaFxSpec().filter }), null)
  assert.equal(mediaFxSpecsEqual(upgraded, { ...upgraded, advanced: createDefaultMediaFxAdvanced() }), true)
  assert.equal(mediaFxSpecsEqual(normalizeMediaFxSpec(v1), normalizeMediaFxSpec({ ...v1, version: 2, advanced: { pixelate: 0, rgbSplit: 0, scanline: 0 } })), true)
  assert.equal(mediaFxSpecsEqual(normalizeMediaFxSpec(v2), normalizeMediaFxSpec({ ...v2, version: 3, advanced: adv(v2.advanced) })), true, 'v2 equals the equivalent v3')
  assert.equal(mediaFxSpecsEqual(upgraded, { ...upgraded, advanced: { ...upgraded.advanced, scanline: 0.1 } }), false)
  assert.equal(mediaFxSpecsEqual(v2Upgraded, { ...v2Upgraded, advanced: { ...v2Upgraded.advanced, negative: 0.1 } }), false)

  // --- presets fill values only --------------------------------------------
  assert.deepEqual(mediaFxAdvancedFromPreset('glitch'), adv({ rgbSplit: 0.35, scanline: 0.18 }))
  assert.deepEqual(mediaFxAdvancedFromPreset('old-tv'), adv({ rgbSplit: 0.08, scanline: 0.55 }), 'existing presets keep their values')
  assert.equal(matchMediaFxAdvancedPreset(adv({ pixelate: 0.45 })), 'low-res')
  assert.equal(matchMediaFxAdvancedPreset(createDefaultMediaFxAdvanced()), 'none')
  assert.equal(mediaFxAdvancedFromPreset('missing'), null)
  assert.deepEqual(mediaFxAdvancedFromPreset('neon'), adv({ bloom: 0.7, vignette: 0.3 }), 'bloom-led preset')
  assert.deepEqual(mediaFxAdvancedFromPreset('soft-glow'), adv({ glow: 0.55 }), 'glow-led preset')
  for (const id of ['film', 'comic', 'anomaly', 'psychic', 'neon', 'soft-glow']) {
    const preset = mediaFxAdvancedFromPreset(id)
    assert.ok(preset && mediaFxAdvancedHasContent(preset), `${id} preset fills advanced values`)
    assert.equal(matchMediaFxAdvancedPreset(preset), id)
    assert.equal(JSON.stringify(normalizeMediaFxSpec({ ...createDefaultMediaFxSpec(), advanced: preset })).includes(id), false, 'preset ids are never persisted')
  }

  // --- capabilities ---------------------------------------------------------
  assert.equal(resolveMediaFxCapabilities('konva').advanced, false, 'existing callers never gain advanced')
  assert.equal(resolveMediaFxCapabilities('dom', false).advanced, false)
  assert.equal(resolveMediaFxCapabilities('bake').advanced, false)
  assert.equal(resolveMediaFxCapabilities('konva', false, true).advanced, true)
  assert.equal(resolveMediaFxCapabilities('konva', true, true).advanced, false, 'animated media never runs advanced')

  // --- strict zod schema ----------------------------------------------------
  const { mediaFxSpecSchema, mediaFxV1SpecSchema, mediaFxV2SpecSchema, mediaFxV3SpecSchema, mediaFxV4SpecSchema, mediaFxV5SpecSchema } = loadModule('./media-fx-schema')
  assert.deepEqual(mediaFxSpecSchema.parse(v1), upgraded, 'strict v1 parses into canonical v5')
  const strictV2 = { ...v2, advanced: { pixelate: 0.45, rgbSplit: 0.123, scanline: 0.5 } }
  assert.deepEqual(mediaFxSpecSchema.parse(strictV2), v2Upgraded, 'strict v2 parses into canonical v5')
  // Strict v3 / v4 documents carry exactly the nine original effects.
  const strictV3Advanced = Object.fromEntries(mediaFxV3AdvancedKeys.map(key => [key, key === 'grain' ? 0.123 : v3Advanced[key]]))
  const strictV3 = { version: 3, motion, filter, advanced: strictV3Advanced }
  const canonicalV3 = normalizeMediaFxSpec(strictV3)
  assert.equal(canonicalV3.version, 5)
  assert.deepEqual(mediaFxSpecSchema.parse(strictV3), canonicalV3, 'strict v3 parses into canonical v5')
  assert.deepEqual(mediaFxSpecSchema.parse(canonicalV3), canonicalV3)
  const strictV4 = { version: 4, motion, filter, advanced: strictV3Advanced, temporal: temporalOn }
  assert.deepEqual(mediaFxSpecSchema.parse(strictV4), v4Upgraded, 'strict v4 parses into canonical v5, temporal intact')
  assert.deepEqual(mediaFxSpecSchema.parse(v5), v5, 'strict v5 keeps bloom / glow and temporal')
  assert.equal(mediaFxV1SpecSchema.safeParse(strictV2).success, false)
  assert.equal(mediaFxV2SpecSchema.safeParse(canonicalV3).success, false)
  assert.equal(mediaFxV3SpecSchema.safeParse(strictV2).success, false)
  assert.equal(mediaFxV4SpecSchema.safeParse(v5).success, false, 'v4 schema rejects v5 data')
  assert.equal(mediaFxV5SpecSchema.safeParse(strictV4).success, false, 'v5 schema rejects v4 data')
  const v5Advanced = v5.advanced
  const rejected = {
    'v4 with bloom': { ...strictV4, advanced: { ...strictV3Advanced, bloom: 0 } },
    'v4 with glow': { ...strictV4, advanced: { ...strictV3Advanced, glow: 0.2 } },
    'v4 with bloom and glow': { ...strictV4, advanced: v5Advanced },
    'v3 with bloom': { ...strictV3, advanced: { ...strictV3Advanced, bloom: 0.5 } },
    'v2 with glow': { ...strictV2, advanced: { ...strictV2.advanced, glow: 0 } },
    'v5 missing bloom': { ...v5, advanced: (({ bloom, ...rest }) => rest)(v5Advanced) },
    'v5 missing glow': { ...v5, advanced: (({ glow, ...rest }) => rest)(v5Advanced) },
    'v5 missing both': { ...v5, advanced: strictV3Advanced },
    'v5 bloom > 1': { ...v5, advanced: { ...v5Advanced, bloom: 1.01 } },
    'v5 bloom < 0': { ...v5, advanced: { ...v5Advanced, bloom: -0.01 } },
    'v5 glow NaN': { ...v5, advanced: { ...v5Advanced, glow: Number.NaN } },
    'v5 glow string': { ...v5, advanced: { ...v5Advanced, glow: '0.5' } },
    'v5 unknown advanced field': { ...v5, advanced: { ...v5Advanced, threshold: 0.8 } },
    'v5 blur radius': { ...v5, advanced: { ...v5Advanced, blurRadius: 12 } },
    'v5 without temporal': { version: 5, motion, filter, advanced: v5Advanced },
    'v5 temporal missing speed': { ...v5, temporal: { grain: 0, flicker: 0, glitch: 0.3, scanlineRoll: 0 } },
    'v5 unknown field': { ...v5, framebuffer: {} },
    'v2 missing advanced': { version: 2, motion, filter },
    'v3 missing advanced': { version: 3, motion, filter },
    'v2 with v3-only field': { ...strictV2, advanced: { ...strictV2.advanced, grain: 0 } },
    'v2 with full v3 advanced': { ...canonicalV3, version: 2 },
    'v3 missing v3 field': { ...strictV2, version: 3 },
    'pixelate > 1': { ...strictV2, advanced: { ...strictV2.advanced, pixelate: 1.01 } },
    'rgbSplit < 0': { ...strictV2, advanced: { ...strictV2.advanced, rgbSplit: -0.01 } },
    'scanline NaN': { ...strictV2, advanced: { ...strictV2.advanced, scanline: Number.NaN } },
    'vignette > 1': { ...canonicalV3, advanced: { ...canonicalV3.advanced, vignette: 1.01 } },
    'grain < 0': { ...canonicalV3, advanced: { ...canonicalV3.advanced, grain: -0.01 } },
    'posterize Infinity': { ...canonicalV3, advanced: { ...canonicalV3.advanced, posterize: Infinity } },
    'negative NaN': { ...canonicalV3, advanced: { ...canonicalV3.advanced, negative: Number.NaN } },
    'sharpen string': { ...canonicalV3, advanced: { ...canonicalV3.advanced, sharpen: '0.5' } },
    'edge > 1': { ...canonicalV3, advanced: { ...canonicalV3.advanced, edge: 2 } },
    'v2 advanced.shader': { ...strictV2, advanced: { ...strictV2.advanced, shader: 'void main(){}' } },
    'v3 advanced.shader': { ...canonicalV3, advanced: { ...canonicalV3.advanced, shader: 'void main(){}' } },
    'v3 advanced.halo': { ...strictV3, advanced: { ...strictV3Advanced, halo: 0.5 } },
    'v1 with advanced': { ...v1, advanced: createDefaultMediaFxAdvanced() },
    'v1 unknown field': { ...v1, glsl: 'x' },
    'v2 unknown field': { ...strictV2, uniforms: {} },
    'v3 unknown field': { ...canonicalV3, uniforms: {} },
    'filter unknown field': { ...canonicalV3, filter: { ...filter, fragmentShader: 'x' } },
    'unknown version': { ...v5, version: 6 },
    'missing version': { motion, filter },
  }
  for (const [name, value] of Object.entries(rejected)) {
    assert.equal(mediaFxSpecSchema.safeParse(value).success, false, `schema must reject ${name}`)
  }

  // --- GPU module in a context without WebGL --------------------------------
  const gpu = loadModule('./media-fx-gpu')
  assert.equal(gpu.mediaFxGpuSupported(), false)
  assert.equal(gpu.createMediaFxGpuFilter(adv({ pixelate: 0.5 })), null)
  assert.equal(gpu.createMediaFxGpuFilter(adv({ grain: 0.5 })), null)
  const pixels = { width: 1, height: 1, data: new Uint8ClampedArray([10, 20, 30, 40]) }
  assert.equal(gpu.applyMediaFxGpu(pixels, adv({ rgbSplit: 1 })), false)
  assert.equal(gpu.applyMediaFxGpu(pixels, adv({ edge: 1 })), false)
  assert.deepEqual([...pixels.data], [10, 20, 30, 40], 'failure keeps the source pixels')
  const params = gpu.resolveMediaFxGpuParams
  // v2 effects keep their exact v2 mapping.
  assert.equal(params(off).blockPx, 1)
  assert.equal(params(adv({ pixelate: 1 })).blockPx, 48)
  assert.equal(params(adv({ pixelate: 0.5 })).blockPx, 13)
  assert.equal(params(adv({ pixelate: 0.5 }), { pixelRatio: 2 }).blockPx, 26)
  assert.equal(params(adv({ pixelate: 1 }), { pixelRatio: 2 }).blockPx, 96)
  assert.equal(params(adv({ rgbSplit: 1 })).rgbSplitPx, 12)
  assert.equal(params(adv({ rgbSplit: 0.25 }), { pixelRatio: 2 }).rgbSplitPx, 6)
  assert.equal(params(adv({ scanline: 1 })).scanlineDarken, 0.25)
  assert.equal(params(off).scanlinePeriodPx, 3)
  // All-default: every effect strength is 0, so every shader branch is skipped.
  const offParams = params(off)
  for (const key of ['rgbSplitPx', 'sharpenAmount', 'edgeMix', 'posterizeLevels', 'negativeMix', 'grainAmplitude', 'scanlineDarken', 'vignetteDarken']) {
    assert.equal(offParams[key], 0, `all-default ${key} is off`)
  }
  // v3 effects: bounded renderer mapping of the 0..1 strengths.
  assert.equal(params(adv({ vignette: 1 })).vignetteDarken, 0.85)
  assert.equal(params(adv({ vignette: 0.5 })).vignetteDarken, 0.425)
  assert.ok(Math.abs(params(adv({ grain: 1 })).grainAmplitude - 0.22) < 1e-9)
  assert.equal(params(adv({ grain: 0.5 }), { pixelRatio: 2 }).grainCellPx, 2, 'grain cell follows the cache pixel ratio')
  assert.equal(params(adv({ grain: 0.5 }), { pixelRatio: 0.25 }).grainCellPx, 1)
  assert.equal(params(adv({ posterize: 1 })).posterizeLevels, 2)
  assert.equal(params(adv({ posterize: 0.01 })).posterizeLevels, 24)
  assert.equal(params(adv({ posterize: 0.5 })).posterizeLevels, 13)
  assert.equal(params(adv({ negative: 0.4 })).negativeMix, 0.4, 'negative blends gradually')
  assert.equal(params(adv({ sharpen: 1 })).sharpenAmount, 2)
  assert.equal(params(adv({ edge: 0.6 })).edgeMix, 0.6)
  assert.equal(params(adv({ edge: 0.6 })).kernelStepPx, 1)
  assert.equal(params(adv({ sharpen: 0.6 }), { pixelRatio: 2 }).kernelStepPx, 2)
  assert.equal(params(adv({ sharpen: 0.6, pixelate: 0.5 })).kernelStepPx, 13, 'pixelated kernels compare neighboring blocks')
  const clamped = params({ ...off, vignette: 9, grain: -1, posterize: Number.NaN, negative: Infinity, sharpen: 5, edge: '1' })
  assert.deepEqual([clamped.vignetteDarken, clamped.grainAmplitude, clamped.posterizeLevels, clamped.negativeMix, clamped.sharpenAmount, clamped.edgeMix], [0.85, 0, 0, 0, 2, 0])

  // Static shader contract: no time input, no frame loop. V3.5 adds time uniforms for live
  // temporal passes (the clock lives elsewhere); V3.6 adds framebuffers, but only inside
  // the bloom / glow light pipeline (see media-fx-multipass-regression.cjs).
  const gpuSource = fs.readFileSync(path.join(featureDir, 'media-fx-gpu.ts'), 'utf8')
  for (const forbidden of [/requestAnimationFrame/, /setTimeout|setInterval/, /Date\.now|performance\.now|Math\.random/, /createRenderbuffer/, /three/i, /getImageData/]) {
    assert.equal(forbidden.test(gpuSource), false, `GPU adapter must not contain ${forbidden}`)
  }
  assert.equal((gpuSource.match(/createFramebuffer\(/g) || []).length, 1, 'framebuffers are only created for the light targets')
  assert.equal((gpuSource.match(/getContext\(/g) || []).length, 1, 'one WebGL2 context per page')

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

  // Mock WebGL2: records shader sources, uniforms and draws, so the uniform plumbing and
  // the all-default short circuit are checked without a real GPU.
  const createMockGl = (mock) => {
    const constants = { VERTEX_SHADER: 1, FRAGMENT_SHADER: 2, COMPILE_STATUS: 3, LINK_STATUS: 4, MAX_TEXTURE_SIZE: 5, NO_ERROR: 0, TEXTURE_2D: 6, RGBA8: 7, RGBA: 8, UNSIGNED_BYTE: 9, TRIANGLES: 10 }
    return new Proxy({
      ...constants,
      drawingBufferWidth: 0,
      drawingBufferHeight: 0,
      createShader: () => ({}),
      shaderSource: (_shader, source) => mock.sources.push(source),
      getShaderParameter: () => true,
      getProgramParameter: () => true,
      createProgram: () => ({}),
      createTexture: () => ({}),
      getParameter: () => 4096,
      getUniformLocation: (_program, name) => name,
      uniform1f: (name, value) => { mock.uniforms[name] = value },
      uniform2f: (name, x, y) => { mock.uniforms[name] = [x, y] },
      uniform1i: (name, value) => { mock.uniforms[name] = value },
      drawArrays: () => { mock.draws += 1 },
      readPixels: (_x, _y, _w, _h, _format, _type, target) => {
        mock.reads += 1
        if (mock.readError) { mock.error = 1282; return }
        target.set(target.map((value, index) => (index % 4 === 3 ? value : 255 - value)))
      },
      getError: () => { const error = mock.error; mock.error = 0; return error },
      isContextLost: () => false,
    }, { get: (target, key) => (key in target ? target[key] : () => {}) })
  }
  const mock = { sources: [], uniforms: {}, draws: 0, reads: 0, error: 0, readError: false, contexts: 0 }
  global.document = { createElement: () => {
    const canvas = { width: 1, height: 1, addEventListener: () => {} }
    const gl = createMockGl(mock)
    Object.defineProperty(gl, 'drawingBufferWidth', { get: () => canvas.width })
    Object.defineProperty(gl, 'drawingBufferHeight', { get: () => canvas.height })
    canvas.getContext = () => { mock.contexts += 1; return gl }
    return canvas
  } }
  const mockGpu = loadModule('./media-fx-gpu', {}, {})
  const image = () => ({ width: 2, height: 1, data: new Uint8ClampedArray([10, 20, 30, 0, 40, 50, 60, 128]) })
  const allDefault = image()
  assert.equal(mockGpu.applyMediaFxGpu(allDefault, off), false)
  assert.equal(mockGpu.createMediaFxGpuFilter(off), null)
  assert.equal(mock.contexts, 0, 'all-default never creates a WebGL context')
  assert.equal(mock.draws, 0, 'all-default never runs a GPU pass')

  // The single-pass program is the main shader without the light variant.
  const fragment = () => mock.sources.find(source => source.includes('uniform sampler2D uTexture'))
  for (const key of mediaFxV3AdvancedKeys) {
    const single = image()
    assert.equal(mockGpu.applyMediaFxGpu(single, adv({ [key]: 0.5 })), true, `${key} alone runs the GPU pass`)
    const expected = params(adv({ [key]: 0.5 }))
    const uniformByParam = { blockPx: 'uPixelate', rgbSplitPx: 'uRgbSplit', kernelStepPx: 'uKernelStep', sharpenAmount: 'uSharpen', edgeMix: 'uEdge', posterizeLevels: 'uPosterizeLevels', negativeMix: 'uNegative', grainAmplitude: 'uGrain', grainCellPx: 'uGrainCell', scanlineDarken: 'uScanline', scanlinePeriodPx: 'uScanlinePeriod', vignetteDarken: 'uVignette' }
    assert.deepEqual(Object.keys(uniformByParam).sort(), Object.keys(expected).sort(), 'every GPU param has a uniform')
    for (const [param, uniform] of Object.entries(uniformByParam)) {
      assert.equal(mock.uniforms[uniform], expected[param], `${key}: ${uniform} receives ${param}`)
      assert.ok(fragment().includes(`uniform float ${uniform};`), `shader declares ${uniform}`)
    }
    assert.deepEqual([single.data[3], single.data[7]], [0, 128], `${key}: alpha bytes survive the upload / readback plumbing`)
  }
  assert.equal(mock.contexts, 1, 'one lazy singleton context serves every pass')
  assert.equal(mock.sources.some(source => source.includes('#define MEDIA_FX_LIGHT')), false, 'bloom / glow off never builds the light pipeline')
  const combo = adv({ pixelate: 0.3, rgbSplit: 0.2, scanline: 0.4, vignette: 0.5, grain: 0.3, posterize: 0.5, negative: 0.2, sharpen: 0.4, edge: 0.3 })
  assert.equal(mockGpu.applyMediaFxGpu(image(), combo, { pixelRatio: 2 }), true)
  assert.equal(mock.uniforms.uKernelStep, params(combo, { pixelRatio: 2 }).kernelStepPx)
  assert.equal(mock.uniforms.uVignette, params(combo).vignetteDarken)
  assert.equal(mock.draws, mediaFxV3AdvancedKeys.length + 1, 'one draw per pass, even for a full combination')

  // The fixed order is in the single shader; the neighborhood is only read when needed.
  // The light composite block only exists in the light variant (MEDIA_FX_LIGHT).
  const fullShader = fragment()
  const lightBlocks = fullShader.match(/#ifdef MEDIA_FX_LIGHT[\s\S]*?#endif/g) || []
  assert.equal(lightBlocks.length, 2, 'the light variant only adds its uniforms and the composite block')
  const shader = lightBlocks.reduce((text, block) => text.replace(block, ''), fullShader)
  assert.equal(fullShader.indexOf('#define MEDIA_FX_LIGHT'), -1, 'the single-pass program never defines the light variant')
  const order = ['uPixelate > 1.0', 'uRgbSplit > 0.0', 'uSharpen > 0.0 || uEdge > 0.0', 'uPosterizeLevels > 1.0', 'uNegative > 0.0', 'uGrain > 0.0', 'uScanline > 0.0', 'uVignette > 0.0']
  const positions = order.map(token => shader.indexOf(`if (${token})`))
  assert.ok(positions.every(position => position > 0), 'every effect is guarded by its own branch')
  assert.deepEqual([...positions].sort((a, b) => a - b), positions, 'effect order is fixed')
  const neighborhood = shader.slice(positions[2], positions[3])
  const glitchBlock = shader.slice(shader.indexOf('if (uGlitch > 0.0)'), shader.indexOf('if (uFlicker > 0.0)'))
  const fetches = source => (source.match(/fetchPixel\(/g) || []).length
  assert.equal(fetches(shader) - fetches(neighborhood) - fetches(glitchBlock), 4, 'outside the guarded neighborhood / glitch only pixelate / rgbSplit fetch')
  assert.equal(fetches(neighborhood), 8, 'the neighborhood is a bounded 3x3 kernel')
  assert.equal(fetches(glitchBlock), 3, 'glitch reads one shifted pixel plus two channel offsets')
  assert.ok(shader.includes('float alpha = center.a;') && shader.includes('outColor = vec4(color.rgb, alpha);'), 'alpha comes from the source pixel')
  assert.equal((shader.match(/alpha = /g) || []).length, 2, 'only glitch moves a sampled alpha')

  const failing = image()
  mock.readError = true
  assert.equal(mockGpu.applyMediaFxGpu(failing, adv({ negative: 1 })), false, 'readPixels failure reports no-op')
  assert.deepEqual([...failing.data], [...image().data], 'readPixels failure keeps the source pixels')
  mock.readError = false

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
  assert.equal(mediaFxAdvancedSignature(adv({ pixelate: 0.1, rgbSplit: 0.2, scanline: 0.3 })), '0.1|0.2|0.3|0|0|0|0|0|0|0|0')
  assert.notEqual(mediaFxAdvancedSignature(adv({ grain: 0.1 })), mediaFxAdvancedSignature(adv({ edge: 0.1 })), 'v3 effects are part of the cache signature')
  assert.notEqual(mediaFxAdvancedSignature(adv({ bloom: 0.1 })), mediaFxAdvancedSignature(adv({ glow: 0.1 })), 'bloom / glow are part of the cache signature')
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

  const grayscaleSplit = withFx({ filter: { ...createDefaultMediaFxSpec().filter, grayscale: 0.5 }, advanced: adv({ rgbSplit: 0.3 }) })
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
  controller.update(withFx({ ...grayscaleSplit, advanced: adv({ rgbSplit: 0.31 }) }), context)
  assert.equal(caches, 2, 'advanced change rebuilds the cache')

  controller.update(withFx({ advanced: adv({ pixelate: 0.4 }) }), context)
  filters = imageNode.filters()
  assert.equal(filters.length, 1)
  assert.equal(isGpu(filters[0]), true, 'advanced-only uses only the GPU step')

  // v3-only effects reach the Konva cache through the same single GPU step.
  const grainOnly = withFx({ advanced: adv({ grain: 0.3 }) })
  controller.update(grainOnly, context)
  filters = imageNode.filters()
  assert.equal(filters.length, 1)
  assert.equal(isGpu(filters[0]), true, 'grain-only uses only the GPU step')
  assert.equal(gpuCalls.at(-1).advanced.grain, 0.3)
  const cachesNow = caches
  controller.update(grainOnly, context)
  assert.equal(caches, cachesNow, 'unchanged v3 effect reuses the cache')
  controller.update(withFx({ advanced: adv({ grain: 0.3, edge: 0.2 }) }), context)
  assert.equal(caches, cachesNow + 1, 'v3 effect change rebuilds the cache')
  assert.deepEqual(gpuCalls.at(-1).advanced, adv({ grain: 0.3, edge: 0.2 }))
  // Bloom / glow reach the cache through the same single GPU step; a change rebuilds once.
  const bloomOnly = withFx({ advanced: adv({ bloom: 0.5 }) })
  controller.update(bloomOnly, context)
  assert.equal(imageNode.filters().length, 1)
  assert.equal(isGpu(imageNode.filters()[0]), true, 'bloom-only uses only the GPU step')
  const cachesBloom = caches
  controller.update(bloomOnly, context)
  assert.equal(caches, cachesBloom, 'unchanged bloom reuses the cache')
  controller.update(withFx({ advanced: adv({ bloom: 0.5, glow: 0.3 }) }), context)
  assert.equal(caches, cachesBloom + 1, 'a glow change rebuilds the cache once')
  assert.deepEqual(gpuCalls.at(-1).advanced, adv({ bloom: 0.5, glow: 0.3 }))
  assert.equal(motionNode.children.length, 1, 'bloom / glow never add a node or layer')
  assert.deepEqual([imageNode.width(), imageNode.height()], [200, 100], 'bloom / glow never change the node geometry')
  // A stored v4 spec never renders bloom / glow it could not carry.
  controller.update({ version: 4, motion: createDefaultMediaFxSpec().motion, filter: createDefaultMediaFxSpec().filter, advanced: { ...adv({ rgbSplit: 0.2 }), bloom: 1 }, temporal: createDefaultMediaFxTemporal() }, context)
  assert.deepEqual(gpuCalls.at(-1).advanced, adv({ rgbSplit: 0.2 }))
  // A stored v2 spec renders its v2 effects with the v3 effects off.
  controller.update({ version: 2, motion: createDefaultMediaFxSpec().motion, filter: createDefaultMediaFxSpec().filter, advanced: { pixelate: 0, rgbSplit: 0.2, scanline: 0, grain: 1 } }, context)
  assert.deepEqual(gpuCalls.at(-1).advanced, adv({ rgbSplit: 0.2 }))

  capability.canvas = false
  controller.update(withFx({ filter: { ...createDefaultMediaFxSpec().filter, brightness: 1.4 }, advanced: adv({ scanline: 0.5 }) }), context)
  filters = imageNode.filters()
  assert.equal(filters[0], Konva.Filters.Brightness, 'canvas-filter fallback still applies')
  assert.equal(isGpu(filters.at(-1)), true, 'GPU still runs after fallback filters')
  capability.canvas = true

  capability.gpu = false
  const blurPixelate = withFx({ filter: { ...createDefaultMediaFxSpec().filter, blurPx: 2 }, advanced: adv({ pixelate: 0.5 }) })
  controller.update(blurPixelate, context)
  assert.deepEqual(imageNode.filters(), ['blur(2px)'], 'without GPU the basic native filter path is unchanged')
  const cachesBefore = caches
  controller.update(blurPixelate, context)
  assert.equal(caches, cachesBefore)
  controller.update(withFx({ advanced: adv({ pixelate: 0.5 }) }), context)
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
