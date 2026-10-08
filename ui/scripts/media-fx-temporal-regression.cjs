// Run with node scripts/media-fx-temporal-regression.cjs.
// Media FX V3.5 temporal: MediaFxSpec v4 core / schema, the shared temporal clock, the
// live GPU session (mock WebGL2), the temporal player and the Konva live output. No
// browser or real WebGL: the shader itself is covered by the headless Chrome smoke test.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')

const featureDir = path.join(__dirname, '../src/features/media-fx')
const compile = source => ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
}).outputText

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

// Mock WebGL2 recording every call that matters for the per-frame budget.
const createMockGl = (mock) => {
  const constants = {
    VERTEX_SHADER: 1, FRAGMENT_SHADER: 2, COMPILE_STATUS: 3, LINK_STATUS: 4, MAX_TEXTURE_SIZE: 5, NO_ERROR: 0,
    TEXTURE_2D: 6, RGBA8: 7, RGBA: 8, UNSIGNED_BYTE: 9, TRIANGLES: 10, TEXTURE0: 11,
  }
  const gl = {
    ...constants,
    createShader: () => ({}),
    shaderSource: (_shader, source) => mock.sources.push(source),
    getShaderParameter: () => true,
    getProgramParameter: () => true,
    createProgram: () => ({}),
    createTexture: () => { const texture = { id: ++mock.textureIds }; mock.textures.push(texture); return texture },
    deleteTexture: (texture) => mock.deleted.push(texture),
    bindTexture: (_target, texture) => { mock.bound = texture },
    getParameter: () => mock.maxTextureSize,
    getUniformLocation: (_program, name) => name,
    uniform1f: (name, value) => { mock.uniforms[name] = value },
    uniform2f: (name, x, y) => { mock.uniforms[name] = [x, y] },
    uniform1i: (name, value) => { mock.uniforms[name] = value },
    texImage2D: (...args) => {
      const source = args.length === 6 ? args[5] : args[8]
      mock.uploads.push({ texture: mock.bound, source })
    },
    viewport: (...args) => { mock.viewport = args },
    drawArrays: () => {
      mock.draws += 1
      if (mock.drawError) mock.error = 1282
    },
    readPixels: () => { mock.reads += 1 },
    getError: () => { const error = mock.error; mock.error = 0; return error },
    isContextLost: () => mock.lost,
  }
  return new Proxy(gl, { get: (target, key) => (key in target ? target[key] : () => {}) })
}

const installMockWebGl = (mock) => {
  const listeners = []
  global.WebGL2RenderingContext = function WebGL2RenderingContext() {}
  global.document = {
    createElement: () => {
      const canvas = { width: 1, height: 1 }
      const gl = createMockGl(mock)
      Object.defineProperty(gl, 'drawingBufferWidth', { get: () => canvas.width })
      Object.defineProperty(gl, 'drawingBufferHeight', { get: () => canvas.height })
      canvas.getContext = () => { mock.contexts += 1; mock.canvases.push(canvas); return gl }
      canvas.addEventListener = (type, listener) => { if (type === 'webglcontextlost') listeners.push({ canvas, listener }) }
      return canvas
    },
  }
  return {
    loseContext: () => {
      mock.lost = true
      const current = mock.canvases.at(-1)
      listeners.filter(entry => entry.canvas === current).forEach(entry => entry.listener())
    },
  }
}

const createMock = () => ({
  sources: [], uniforms: {}, draws: 0, reads: 0, error: 0, contexts: 0, canvases: [], textures: [], textureIds: 0,
  deleted: [], uploads: [], bound: null, viewport: null, lost: false, drawError: false, maxTextureSize: 4096,
})

// Manual clock environment: frames run only when the test fires them.
const createClockEnv = () => {
  const env = {
    hidden: false,
    pending: new Map(),
    nextHandle: 0,
    requests: 0,
    cancels: 0,
    visibilityListeners: new Set(),
    requestFrame(callback) { env.requests += 1; const handle = ++env.nextHandle; env.pending.set(handle, callback); return handle },
    cancelFrame(handle) { if (env.pending.delete(handle)) env.cancels += 1 },
    isHidden: () => env.hidden,
    onVisibilityChange(listener) { env.visibilityListeners.add(listener); return () => env.visibilityListeners.delete(listener) },
    fire(timestamp) {
      const callbacks = [...env.pending.values()]
      env.pending.clear()
      callbacks.forEach(callback => callback(timestamp))
    },
    setHidden(hidden) { env.hidden = hidden; env.visibilityListeners.forEach(listener => listener()) },
  }
  return env
}

async function run() {
  const core = loadModule('./media-fx')
  const {
    MEDIA_FX_VERSION, createDefaultMediaFxSpec, createDefaultMediaFxTemporal, normalizeMediaFxSpec,
    normalizeMediaFxTemporal, mediaFxTemporalHasContent, mediaFxHasContent, compactMediaFxSpec, mediaFxSpecsEqual,
    resolveMediaFxCapabilities, MEDIA_FX_TEMPORAL_RANGES,
  } = core
  const off = createDefaultMediaFxTemporal()
  const temporal = (patch = {}) => ({ ...off, ...patch })

  // --- MediaFxSpec v4 core ---------------------------------------------------
  assert.equal(MEDIA_FX_VERSION, 4)
  assert.deepEqual(off, { grain: 0, flicker: 0, glitch: 0, scanlineRoll: 0, speed: 1 })
  assert.deepEqual(MEDIA_FX_TEMPORAL_RANGES.speed, { min: 0.25, max: 3, step: 0.05, defaultValue: 1 })
  const motion = { preset: 'float', intensity: 0.6, durationMs: 4000, loop: true }
  const filter = { ...createDefaultMediaFxSpec().filter, sepia: 0.3 }
  const advanced = { ...createDefaultMediaFxSpec().advanced, pixelate: 0.2, grain: 0.4, edge: 0.1 }
  const v1 = { version: 1, motion, filter }
  const v2 = { version: 2, motion, filter, advanced: { pixelate: 0.2, rgbSplit: 0.1, scanline: 0 } }
  const v3 = { version: 3, motion, filter, advanced }
  const v4 = { version: 4, motion, filter, advanced, temporal: temporal({ grain: 0.5, glitch: 0.25, speed: 1.5 }) }
  assert.deepEqual(normalizeMediaFxSpec(v1), { version: 4, motion, filter, advanced: createDefaultMediaFxSpec().advanced, temporal: off })
  assert.deepEqual(normalizeMediaFxSpec(v2).advanced, { ...createDefaultMediaFxSpec().advanced, pixelate: 0.2, rgbSplit: 0.1 })
  assert.deepEqual(normalizeMediaFxSpec(v2).temporal, off)
  assert.deepEqual(normalizeMediaFxSpec(v3), { version: 4, motion, filter, advanced, temporal: off }, 'v3 keeps all nine advanced effects')
  assert.deepEqual(normalizeMediaFxSpec(v4), v4)
  assert.deepEqual(normalizeMediaFxSpec({ ...v3, temporal: temporal({ glitch: 1 }) }).temporal, off, 'v3 never reads temporal')
  assert.deepEqual(normalizeMediaFxSpec({ ...v4, version: 5 }), createDefaultMediaFxSpec())
  assert.deepEqual(
    normalizeMediaFxTemporal({ grain: 4, flicker: -1, glitch: Number.NaN, scanlineRoll: '1', speed: 9, seed: 3, uniforms: {} }),
    { grain: 1, flicker: 0, glitch: 0, scanlineRoll: 0, speed: 3 },
  )
  assert.equal(normalizeMediaFxTemporal({ speed: 0.1 }).speed, 0.25)
  assert.equal(normalizeMediaFxTemporal({ speed: 1.234 }).speed, 1.23)
  assert.equal('seed' in normalizeMediaFxSpec({ ...v4, temporal: { ...v4.temporal, seed: 1 } }).temporal, false)

  // advanced.grain and temporal.grain are separate effects.
  assert.equal(normalizeMediaFxSpec({ ...v4, temporal: off }).advanced.grain, 0.4)
  assert.equal(normalizeMediaFxSpec({ ...v4, advanced: createDefaultMediaFxSpec().advanced }).temporal.grain, 0.5)

  // content / compact / equality: speed alone is never an effect.
  const speedOnly = { ...createDefaultMediaFxSpec(), temporal: temporal({ speed: 2.5 }) }
  assert.equal(mediaFxTemporalHasContent(speedOnly.temporal), false)
  assert.equal(mediaFxHasContent(speedOnly), false)
  assert.equal(compactMediaFxSpec(speedOnly), null)
  assert.deepEqual(compactMediaFxSpec({ ...speedOnly, motion }).temporal, off, 'a leftover speed is not stored')
  for (const key of ['grain', 'flicker', 'glitch', 'scanlineRoll']) {
    const only = { ...createDefaultMediaFxSpec(), temporal: temporal({ [key]: 0.3 }) }
    assert.equal(mediaFxHasContent(only), true, `${key} counts as content`)
    assert.deepEqual(compactMediaFxSpec(only), only, `${key}-only survives compaction`)
  }
  assert.equal(mediaFxTemporalHasContent(temporal({ grain: 0.0004 })), false, 'near-zero temporal is off')
  assert.equal(mediaFxSpecsEqual(normalizeMediaFxSpec(v3), normalizeMediaFxSpec({ ...v3, version: 4, temporal: temporal({ speed: 2 }) })), true, 'v3 equals v4 with temporal off, any speed')
  assert.equal(mediaFxSpecsEqual(normalizeMediaFxSpec(v1), normalizeMediaFxSpec({ ...v1, version: 4, advanced: createDefaultMediaFxSpec().advanced, temporal: off })), true)
  assert.equal(mediaFxSpecsEqual(normalizeMediaFxSpec(v3), normalizeMediaFxSpec(v4)), false)
  assert.equal(mediaFxSpecsEqual(normalizeMediaFxSpec(v4), normalizeMediaFxSpec({ ...v4, temporal: { ...v4.temporal, speed: 2 } })), false, 'speed matters while an effect is on')

  // capabilities: opt-in, never for animated media or bake.
  assert.equal(resolveMediaFxCapabilities('konva').temporal, false, 'existing callers never gain temporal')
  assert.equal(resolveMediaFxCapabilities('dom', false, true).temporal, false)
  assert.equal(resolveMediaFxCapabilities('konva', false, true, true).temporal, true)
  assert.equal(resolveMediaFxCapabilities('dom', true, true, true).temporal, false, 'animated media never runs temporal')
  assert.equal(resolveMediaFxCapabilities('bake', false, true, true).temporal, false, 'bake output never carries temporal')

  // --- strict zod schema ------------------------------------------------------
  const { mediaFxSpecSchema, mediaFxV3SpecSchema, mediaFxV4SpecSchema } = loadModule('./media-fx-schema')
  assert.deepEqual(mediaFxSpecSchema.parse(v1), normalizeMediaFxSpec(v1), 'v1 -> v4')
  assert.deepEqual(mediaFxSpecSchema.parse(v2), normalizeMediaFxSpec(v2), 'v2 -> v4')
  assert.deepEqual(mediaFxSpecSchema.parse(v3), normalizeMediaFxSpec(v3), 'v3 -> v4 losslessly')
  assert.deepEqual(mediaFxSpecSchema.parse(v4), v4)
  assert.equal(mediaFxV3SpecSchema.safeParse(v4).success, false)
  assert.equal(mediaFxV4SpecSchema.safeParse(v3).success, false)
  const rejected = {
    'v3 with temporal': { ...v3, temporal: off },
    'v2 with temporal': { ...v2, temporal: off },
    'v1 with temporal': { ...v1, temporal: off },
    'v4 without temporal': { ...v3, version: 4 },
    'v4 null temporal': { ...v4, temporal: null },
    'v4 missing speed': { ...v4, temporal: { grain: 0, flicker: 0, glitch: 0, scanlineRoll: 0 } },
    'v4 missing grain': { ...v4, temporal: { flicker: 0, glitch: 0, scanlineRoll: 0, speed: 1 } },
    'v4 unknown temporal field': { ...v4, temporal: { ...v4.temporal, seed: 7 } },
    'v4 temporal shader': { ...v4, temporal: { ...v4.temporal, shader: 'void main(){}' } },
    'v4 temporal uniforms': { ...v4, temporal: { ...v4.temporal, uniforms: { uTime: 1 } } },
    'v4 glitch > 1': { ...v4, temporal: { ...v4.temporal, glitch: 1.01 } },
    'v4 flicker NaN': { ...v4, temporal: { ...v4.temporal, flicker: Number.NaN } },
    'v4 speed < 0.25': { ...v4, temporal: { ...v4.temporal, speed: 0.2 } },
    'v4 speed > 3': { ...v4, temporal: { ...v4.temporal, speed: 3.5 } },
    'v4 speed string': { ...v4, temporal: { ...v4.temporal, speed: '1' } },
    'v4 missing v3 advanced field': { ...v4, advanced: { pixelate: 0, rgbSplit: 0, scanline: 0 } },
    'v4 unknown field': { ...v4, framebuffer: {} },
    'unknown version': { ...v4, version: 5 },
  }
  for (const [name, value] of Object.entries(rejected)) {
    assert.equal(mediaFxSpecSchema.safeParse(value).success, false, `schema must reject ${name}`)
  }

  // --- shared temporal clock ----------------------------------------------------
  const temporalModule = loadModule('./media-fx-temporal', { './media-fx-gpu': { mediaFxGpuSupported: () => true, createMediaFxTemporalGpuSession: () => { throw new Error('unexpected default session') } } }, {})
  const { createMediaFxTemporalClock, MEDIA_FX_TEMPORAL_MAX_FPS, resolveMediaFxTemporalRaster, MEDIA_FX_TEMPORAL_MAX_RASTER_PIXELS } = temporalModule
  assert.equal(MEDIA_FX_TEMPORAL_MAX_FPS, 30)
  const env = createClockEnv()
  const clock = createMediaFxTemporalClock(env)
  assert.equal(env.requests, 0, 'no subscriber, no RAF')
  assert.equal(env.visibilityListeners.size, 0, 'no subscriber, no visibility listener')
  const ticksA = []
  const ticksB = []
  const offA = clock.subscribe(tick => ticksA.push(tick))
  const offB = clock.subscribe(tick => ticksB.push(tick))
  assert.equal(env.requests, 1, 'many consumers share one RAF request')
  assert.equal(env.pending.size, 1)
  env.fire(1000)
  assert.deepEqual(ticksA, [{ time: 1, delta: 0 }], 'the first tick carries no elapsed time')
  assert.deepEqual(ticksB, ticksA, 'every subscriber receives the same tick')
  env.fire(1016.7)
  assert.equal(ticksA.length, 1, '60 Hz frames are throttled to ~30 fps')
  env.fire(1033.4)
  assert.equal(ticksA.length, 2)
  assert.ok(Math.abs(ticksA[1].delta - 0.0334) < 1e-9, 'delta is monotonic elapsed time, not a frame count')
  env.fire(1100)
  assert.ok(Math.abs(ticksA.at(-1).delta - 0.0666) < 1e-9, 'a dropped frame advances by real time without catching up')
  env.fire(4100)
  assert.equal(ticksA.at(-1).delta, 0.25, 'a long stall is capped, never replayed')
  assert.equal(env.pending.size, 1, 'exactly one frame request is pending')
  const ticksBeforeHidden = ticksA.length
  env.setHidden(true)
  assert.equal(env.pending.size, 0, 'hidden page cancels the RAF')
  assert.equal(clock.running, false)
  env.fire(4200)
  assert.equal(ticksA.length, ticksBeforeHidden)
  env.setHidden(false)
  assert.equal(env.pending.size, 1, 'visible page resumes')
  env.fire(9000)
  assert.equal(ticksA.at(-1).delta, 0, 'hidden time is not added to the effects')
  offA()
  assert.equal(env.pending.size, 1, 'the clock keeps running for the remaining subscriber')
  offB()
  assert.equal(env.pending.size, 0, 'the last subscriber cancels the RAF')
  assert.equal(clock.subscribers, 0)
  assert.equal(env.visibilityListeners.size, 0, 'the visibility listener is released')
  offB()
  assert.equal(clock.subscribers, 0, 'unsubscribing twice is harmless')
  const throwing = clock.subscribe(() => { throw new Error('consumer failure') })
  const survivor = []
  const offSurvivor = clock.subscribe(tick => survivor.push(tick))
  env.fire(20000)
  assert.equal(survivor.length, 1, 'a failing consumer does not stop the others')
  throwing()
  offSurvivor()
  const source = fs.readFileSync(path.join(featureDir, 'media-fx-temporal.ts'), 'utf8')
  assert.equal(/setInterval|setTimeout/.test(source), false, 'the clock never uses timers')
  assert.equal((source.match(/requestAnimationFrame\(/g) || []).length, 1, 'exactly one RAF owner')

  // Live raster follows the displayed size x DPR with a pixel budget.
  assert.deepEqual(resolveMediaFxTemporalRaster(300, 600, 2), { width: 600, height: 1200, pixelRatio: 2 })
  const big = resolveMediaFxTemporalRaster(4000, 3000, 3)
  assert.ok(big.width * big.height <= MEDIA_FX_TEMPORAL_MAX_RASTER_PIXELS * 1.01, '4K never runs at natural resolution')
  assert.equal(resolveMediaFxTemporalRaster(0, 10, 1), null)

  // --- live GPU session (mock WebGL2) ---------------------------------------------
  const originalDocument = global.document
  const originalWebGL2 = global.WebGL2RenderingContext
  const mock = createMock()
  const control = installMockWebGl(mock)
  const gpu = loadModule('./media-fx-gpu', {}, {})
  const baseline = { source: { id: 'baseline-a' }, width: 64, height: 32, pixelRatio: 2 }
  const session = gpu.createMediaFxTemporalGpuSession()
  assert.equal(session.render(temporal({ grain: 0.5 }), 1), null, 'no baseline, nothing to draw')
  session.setBaseline(baseline)
  assert.equal(mock.contexts, 0, 'setting a baseline never touches WebGL by itself')
  assert.equal(session.render(off, 1), null, 'all-off temporal never runs a live pass')
  assert.equal(mock.contexts, 0, 'all-off temporal never creates a context')
  assert.equal(mock.draws, 0)
  const frame = session.render(temporal({ grain: 0.5, flicker: 0.4 }), 1.25)
  assert.ok(frame, 'a live frame is drawn')
  assert.equal(mock.contexts, 1, 'the page-wide processor context is reused')
  assert.equal(mock.uploads.length, 1, 'the baseline uploads once')
  assert.equal(mock.uploads[0].source, baseline.source)
  assert.equal(mock.uniforms.uFlipY, 1, 'live frames read rows top-down for canvas presentation')
  assert.equal(mock.uniforms.uTime, 1.25)
  assert.equal(mock.uniforms.uTemporalScale, 2)
  assert.ok(mock.uniforms.uTemporalGrain > 0 && mock.uniforms.uFlicker > 0)
  assert.equal(mock.uniforms.uGlitch, 0)
  for (const name of ['uRgbSplit', 'uSharpen', 'uEdge', 'uPosterizeLevels', 'uNegative', 'uGrain', 'uScanline', 'uVignette']) {
    assert.equal(mock.uniforms[name], 0, `${name} is off in live passes (already in the baseline)`)
  }
  assert.equal(mock.uniforms.uPixelate, 1, 'pixelate is off in live passes')
  assert.equal(frame.canvas, mock.canvases[0], 'the frame is the shared canvas, copied by the caller')
  assert.deepEqual([frame.x, frame.y, frame.width, frame.height], [0, frame.canvas.height - 32, 64, 32])
  assert.deepEqual(mock.viewport, [0, 0, 64, 32])
  for (let index = 0; index < 30; index += 1) session.render(temporal({ grain: 0.5, flicker: 0.4 }), 2 + index / 30)
  assert.equal(mock.uploads.length, 1, 'steady frames never re-upload the source texture')
  assert.equal(mock.reads, 0, 'live frames never call readPixels')
  assert.equal(mock.draws, 31, 'one draw per frame')
  session.render(temporal({ glitch: 1, scanlineRoll: 0.7, speed: 3 }), 3)
  assert.equal(mock.uploads.length, 1, 'temporal parameter changes never re-upload')
  assert.equal(mock.uniforms.uGlitch, 1)

  // A second session shares the context, owns its own texture.
  const sessionB = gpu.createMediaFxTemporalGpuSession()
  sessionB.setBaseline({ source: { id: 'baseline-b' }, width: 128, height: 16, pixelRatio: 1 })
  const frameB = sessionB.render(temporal({ grain: 0.2 }), 1)
  assert.equal(mock.contexts, 1, 'one shared WebGL2 context for every live session')
  assert.equal(mock.textures.length, 3, 'the static texture plus one texture per live session')
  assert.ok(frameB.canvas.width >= 128 && frameB.canvas.height >= 32, 'the shared canvas grows to the largest live raster')
  assert.equal(frameB.y, frameB.canvas.height - 16)

  // A static pass afterwards keeps the exact static uniforms (no flip, temporal off).
  const pixels = { width: 2, height: 1, data: new Uint8ClampedArray([1, 2, 3, 4, 5, 6, 7, 8]) }
  assert.equal(gpu.applyMediaFxGpu(pixels, { ...createDefaultMediaFxSpec().advanced, negative: 1 }), true)
  for (const name of ['uFlipY', 'uTime', 'uTemporalGrain', 'uFlicker', 'uGlitch', 'uScanlineRoll']) {
    assert.equal(mock.uniforms[name], 0, `static passes reset ${name}`)
  }
  assert.equal(mock.reads, 1, 'only the static pass reads pixels back')

  // A new baseline uploads once more into the same texture.
  const uploadsBefore = mock.uploads.length
  session.setBaseline({ ...baseline, source: { id: 'baseline-a2' } })
  session.render(temporal({ grain: 0.5 }), 4)
  session.render(temporal({ grain: 0.5 }), 4.1)
  assert.equal(mock.uploads.length, uploadsBefore + 1, 'a changed baseline uploads exactly once')

  // Context loss: frames fail gracefully, the next frame lazily rebuilds and re-uploads.
  control.loseContext()
  mock.lost = false
  const recovered = session.render(temporal({ grain: 0.5 }), 5)
  assert.ok(recovered, 'a lost context is rebuilt lazily')
  assert.equal(mock.contexts, 2)
  assert.equal(mock.uploads.at(-1).source.id, 'baseline-a2', 'the baseline re-uploads into the new context')
  assert.equal(gpu.mediaFxGpuSupported(), true, 'context loss never disables the GPU path')

  // GL errors never throw and never spam.
  mock.drawError = true
  assert.equal(session.render(temporal({ grain: 0.5 }), 6), null, 'a failed draw returns no frame')
  mock.drawError = false
  // Oversized rasters are refused instead of crashing.
  const oversized = gpu.createMediaFxTemporalGpuSession()
  oversized.setBaseline({ source: {}, width: 5000, height: 10, pixelRatio: 1 })
  assert.equal(oversized.render(temporal({ grain: 0.5 }), 1), null, 'MAX_TEXTURE_SIZE is respected')
  oversized.dispose()

  const deletedBefore = mock.deleted.length
  session.dispose()
  sessionB.dispose()
  assert.equal(mock.deleted.length, deletedBefore + 2, 'disposing sessions frees their textures')
  assert.equal(mock.canvases.at(-1).width, 1, 'the shared drawing buffer shrinks when no live session is left')
  assert.equal(session.render(temporal({ grain: 0.5 }), 7), null, 'a disposed session never draws')

  const gpuSource = fs.readFileSync(path.join(featureDir, 'media-fx-gpu.ts'), 'utf8')
  for (const forbidden of [/createFramebuffer|bindFramebuffer|createRenderbuffer/, /uTexture2/, /requestAnimationFrame/, /getImageData/, /Math\.random/]) {
    assert.equal(forbidden.test(gpuSource), false, `GPU adapter must not contain ${forbidden}`)
  }
  const liveSection = gpuSource.slice(gpuSource.indexOf('export const createMediaFxTemporalGpuSession'))
  assert.equal(/readPixels/.test(liveSection), false, 'the live session never reads pixels back')
  assert.equal((liveSection.match(/texImage2D/g) || []).length, 1, 'the live session has a single, guarded upload')
  const shader = mock.sources.find(text => text.includes('outColor'))
  const order = ['uGlitch > 0.0', 'uFlicker > 0.0', 'uTemporalGrain > 0.0', 'uScanlineRoll > 0.0']
  const positions = order.map(token => shader.indexOf(`if (${token})`))
  assert.ok(positions.every(position => position > shader.indexOf('if (uVignette > 0.0)')), 'temporal runs after every static effect')
  assert.deepEqual([...positions].sort((a, b) => a - b), positions, 'temporal order is fixed')
  assert.equal((shader.match(/uniform sampler2D/g) || []).length, 1, 'single texture, single pass')
  assert.equal(/random\s*\(|uniform \w+ \w*[Ss]eed/.test(shader), false, 'no random seed: same time, same frame')

  if (originalDocument === undefined) delete global.document
  else global.document = originalDocument
  if (originalWebGL2 === undefined) delete global.WebGL2RenderingContext
  else global.WebGL2RenderingContext = originalWebGL2

  // --- temporal player ---------------------------------------------------------------
  const createSessionStub = () => {
    const stub = {
      baselines: [], renders: [], fail: false, disposed: false,
      setBaseline(value) { stub.baselines.push(value) },
      render(value, time) {
        stub.renders.push({ temporal: value, time })
        return stub.fail ? null : { canvas: {}, x: 0, y: 0, width: 4, height: 4 }
      },
      dispose() { stub.disposed = true },
    }
    return stub
  }
  const playerEnv = createClockEnv()
  const sharedClock = createMediaFxTemporalClock(playerEnv)
  let gpuOk = true
  const makePlayer = (extra = {}) => {
    const sessionStub = createSessionStub()
    const events = { presents: 0, live: [] }
    const player = temporalModule.createMediaFxTemporalPlayer({
      clock: sharedClock,
      session: sessionStub,
      gpuSupported: () => gpuOk,
      present: () => { events.presents += 1; return true },
      onLiveChange: live => events.live.push(live),
      ...extra,
    })
    return { player, sessionStub, events }
  }
  const a = makePlayer()
  a.player.update({ temporal: off, active: true })
  a.player.setBaseline({ source: {}, width: 4, height: 4, pixelRatio: 1 })
  assert.equal(playerEnv.requests, 0, 'temporal all-off: no RAF')
  assert.equal(a.player.running, false)
  a.player.update({ temporal: temporal({ speed: 3 }), active: true })
  assert.equal(playerEnv.requests, 0, 'speed alone never starts the clock')
  a.player.update({ temporal: temporal({ grain: 0.5, speed: 2 }), active: true })
  assert.equal(a.player.running, true)
  const b = makePlayer()
  b.player.setBaseline({ source: {}, width: 4, height: 4, pixelRatio: 1 })
  b.player.update({ temporal: temporal({ flicker: 0.5 }), active: true })
  assert.equal(sharedClock.subscribers, 2)
  assert.equal(playerEnv.pending.size, 1, 'multiple active consumers share one clock frame')
  playerEnv.fire(0)
  playerEnv.fire(100)
  assert.equal(a.sessionStub.renders.length, 2)
  assert.equal(b.sessionStub.renders.length, 2)
  assert.ok(Math.abs(a.sessionStub.renders[1].time - 0.2) < 1e-9, 'speed scales elapsed temporal time')
  assert.ok(Math.abs(b.sessionStub.renders[1].time - 0.1) < 1e-9)
  assert.deepEqual(a.events.live, [true], 'live after the first presented frame')
  assert.equal(a.sessionStub.baselines.length, 1, 'ticks never touch the baseline')

  // Pause / reduced motion / visibility are the consumer's `active` gate.
  a.player.update({ temporal: temporal({ grain: 0.5, speed: 2 }), active: false })
  assert.equal(a.player.running, false, 'inactive (pause / reduced motion / hidden) stops the tick')
  assert.deepEqual(a.events.live, [true, false], 'stopping asks the consumer to show its baseline')
  playerEnv.fire(200)
  assert.equal(a.sessionStub.renders.length, 2, 'no GPU work while inactive')
  a.player.update({ temporal: temporal({ grain: 0.5, speed: 2 }), active: true })
  playerEnv.fire(300)
  assert.ok(Math.abs(a.sessionStub.renders.at(-1).time - 0.4) < 1e-9, 'paused time is not added on resume')

  // canRender gate (e.g. hidden Konva node): ticks do no GPU work.
  let visible = false
  const c = makePlayer({ canRender: () => visible })
  c.player.setBaseline({ source: {}, width: 4, height: 4, pixelRatio: 1 })
  c.player.update({ temporal: temporal({ glitch: 0.5 }), active: true })
  playerEnv.fire(400)
  assert.equal(c.sessionStub.renders.length, 0, 'an invisible target does no GPU work')
  visible = true
  playerEnv.fire(500)
  assert.equal(c.sessionStub.renders.length, 1)

  // Failure keeps the static baseline and stops; a new baseline may retry.
  c.sessionStub.fail = true
  playerEnv.fire(600)
  assert.equal(c.player.running, false, 'a failed frame stops the live loop')
  assert.equal(c.events.live.at(-1), false, 'failure falls back to the static baseline')
  playerEnv.fire(700)
  assert.equal(c.sessionStub.renders.length, 2, 'no retry storm after a failure')
  c.sessionStub.fail = false
  c.player.setBaseline({ source: {}, width: 4, height: 4, pixelRatio: 1 })
  assert.equal(c.player.running, true, 'a new baseline retries')
  const presentFail = makePlayer({ present: () => false })
  presentFail.player.setBaseline({ source: {}, width: 4, height: 4, pixelRatio: 1 })
  presentFail.player.update({ temporal: temporal({ grain: 0.2 }), active: true })
  playerEnv.fire(800)
  assert.equal(presentFail.player.running, false, 'a target that cannot present stops too')

  // GPU unavailable: never subscribes.
  gpuOk = false
  const d = makePlayer()
  d.player.setBaseline({ source: {}, width: 4, height: 4, pixelRatio: 1 })
  d.player.update({ temporal: temporal({ grain: 0.2 }), active: true })
  assert.equal(d.player.running, false, 'GPU unavailable keeps the static look')
  gpuOk = true
  d.player.setBaseline(null)
  d.player.update({ temporal: temporal({ grain: 0.2 }), active: true })
  assert.equal(d.player.running, false, 'no baseline, no live loop')

  for (const entry of [a, b, c, presentFail, d]) entry.player.dispose()
  assert.equal(sharedClock.subscribers, 0, 'disposed players leave the clock')
  assert.equal(playerEnv.pending.size, 0, 'the last player stops the RAF')
  assert.ok(a.sessionStub.disposed && d.sessionStub.disposed, 'players release their GPU sessions')

  // --- Konva live output ----------------------------------------------------------------
  const Konva = (await import('konva')).default
  const playerStubs = []
  const playerStub = (options) => {
    const stub = {
      options, baselines: [], updates: [], live: false, disposed: false,
      setBaseline(value) { stub.baselines.push(value) },
      update(input) { stub.updates.push(input) },
      dispose() { stub.disposed = true },
      get running() { return false },
    }
    playerStubs.push(stub)
    return stub
  }
  const fakeCanvas = () => {
    const canvas = { width: 0, height: 0, draws: [] }
    canvas.getContext = () => ({ drawImage: (...args) => canvas.draws.push(args), clearRect: () => {} })
    return canvas
  }
  const konvaModule = loadModule('./media-fx-konva', {
    konva: { __esModule: true, default: Konva },
    './media-fx-gpu': { mediaFxGpuSupported: () => true, createMediaFxGpuFilter: () => null },
    './media-fx-canvas': { canvasFilterSupported: () => true },
    './media-fx-temporal': { ...temporalModule, createMediaFxTemporalPlayer: playerStub },
  })
  // A Layer needs a real canvas; the motion wrapper reports a recording layer instead.
  let draws = 0
  const layer = { batchDraw: () => { draws += 1 } }
  const motionNode = new Konva.Group()
  motionNode.getLayer = () => layer
  const imageNode = new Konva.Image({ image: { width: 200, height: 100 }, width: 200, height: 100 })
  motionNode.add(imageNode)
  // Konva nodes exist; from here on canvases are the recording fakes.
  global.document = { createElement: fakeCanvas }
  let caches = 0
  let cacheCanvas = { pixelRatio: 2, width: 400, height: 200, _canvas: { id: 'cache-1' } }
  imageNode.cache = () => { caches += 1; imageNode._cache.set('canvas', { scene: cacheCanvas, filter: cacheCanvas, x: 0, y: 0 }); return imageNode }
  imageNode.clearCache = () => { imageNode._cache.delete('canvas'); return imageNode }
  imageNode._getCachedSceneCanvas = () => cacheCanvas
  const controller = konvaModule.createKonvaMediaFxController({ motionNode, imageNode })
  const context = { width: 200, height: 100, motion: false, filters: true, advanced: true, temporal: true, reducedMotion: false }
  const withTemporal = patch => ({ ...createDefaultMediaFxSpec(), temporal: temporal(patch) })

  controller.update(createDefaultMediaFxSpec(), context)
  assert.equal(playerStubs.length, 0, 'temporal all-off never creates a live player')
  assert.equal(caches, 0)
  controller.update(withTemporal({ grain: 0.5 }), context)
  assert.equal(caches, 1, 'temporal-only builds one static cache as its baseline')
  const konvaPlayer = playerStubs[0]
  assert.equal(konvaPlayer.baselines.length, 1, 'the cached static look becomes the baseline')
  const firstBaseline = konvaPlayer.baselines[0]
  assert.equal(firstBaseline.pixelRatio, Math.min(2, Konva.pixelRatio || 1))
  assert.equal(firstBaseline.source.draws[0][0], cacheCanvas._canvas, 'the baseline is a copy of the Konva cache canvas')
  assert.equal(konvaPlayer.updates.at(-1).active, true)

  // Frames: no cache rebuild, only the own layer redraws, drawn where the cache is drawn.
  const drawsBefore = draws
  for (let index = 0; index < 10; index += 1) {
    assert.equal(konvaPlayer.options.present({ canvas: { id: 'gl' }, x: 0, y: 0, width: 200, height: 100 }), true)
  }
  assert.equal(caches, 1, 'temporal frames never rebuild the Konva cache')
  assert.equal(draws, drawsBefore + 10, 'each frame only requests its own layer redraw')
  assert.ok(Object.prototype.hasOwnProperty.call(imageNode, '_drawCachedSceneCanvas'), 'live output replaces the cached draw')
  assert.equal(imageNode.listening(), true, 'hit testing is untouched')
  assert.equal(motionNode.children.length, 1, 'no extra node / layer is created')
  const sceneDraws = []
  const fakeContext = {
    save() {}, restore() {}, translate() {}, _applyOpacity() {}, _applyGlobalCompositeOperation() {},
    drawImage: (...args) => sceneDraws.push(args),
  }
  imageNode._drawCachedSceneCanvas(fakeContext)
  assert.deepEqual(sceneDraws[0].slice(1), [0, 0, 200, 100], 'live frame covers the cached area in layout units')

  assert.equal(konvaPlayer.options.canRender(), true)
  motionNode.visible(false)
  assert.equal(konvaPlayer.options.canRender(), false, 'a node hidden by an ancestor does no GPU work')
  controller.update(withTemporal({ grain: 0.5 }), context)
  assert.equal(konvaPlayer.updates.at(-1).active, false, 'a hidden Konva target leaves the shared temporal clock')
  motionNode.visible(true)
  controller.update(withTemporal({ grain: 0.5 }), context)
  assert.equal(konvaPlayer.updates.at(-1).active, true, 'a visible target can subscribe again without rebuilding its baseline')
  assert.equal(caches, 1)

  // Temporal value changes: no cache rebuild, no new baseline.
  controller.update(withTemporal({ grain: 0.9, glitch: 0.3, speed: 2 }), context)
  assert.equal(caches, 1)
  assert.equal(konvaPlayer.baselines.length, 1, 'temporal edits keep the baseline')
  assert.equal(konvaPlayer.updates.at(-1).temporal.glitch, 0.3)

  // Static changes invalidate the baseline exactly once.
  cacheCanvas = { pixelRatio: 2, width: 400, height: 200, _canvas: { id: 'cache-2' } }
  controller.update({ ...withTemporal({ grain: 0.9 }), filter: { ...createDefaultMediaFxSpec().filter, sepia: 0.5 } }, context)
  assert.equal(caches, 2, 'a filter change rebuilds the static cache')
  assert.equal(konvaPlayer.baselines.length, 2)
  assert.equal(konvaPlayer.baselines[1].source.draws[0][0].id, 'cache-2')
  assert.equal(Object.prototype.hasOwnProperty.call(imageNode, '_drawCachedSceneCanvas'), false, 'a rebuilt cache never shows a stale live frame')

  // Pause and reduced motion stop temporal, the static cache stays.
  controller.update({ ...withTemporal({ grain: 0.9 }), filter: { ...createDefaultMediaFxSpec().filter, sepia: 0.5 } }, { ...context, paused: true })
  assert.equal(konvaPlayer.updates.at(-1).active, false, 'pause stops temporal')
  assert.equal(caches, 2)
  controller.update({ ...withTemporal({ grain: 0.9 }), filter: { ...createDefaultMediaFxSpec().filter, sepia: 0.5 } }, { ...context, reducedMotion: true })
  assert.equal(konvaPlayer.updates.at(-1).active, false, 'reduced motion stops temporal')
  controller.update(withTemporal({ grain: 0.9 }), { ...context, filters: false })
  assert.equal(konvaPlayer.updates.at(-1).active, false, 'animated media never runs temporal')
  controller.update(withTemporal({ grain: 0.9 }), { ...context, temporal: false })
  assert.equal(konvaPlayer.updates.at(-1).active, false, 'consumers without the capability never run temporal')

  // The stop callback restores the static cached draw and redraws only its layer.
  konvaPlayer.options.present({ canvas: {}, x: 0, y: 0, width: 200, height: 100 })
  const drawsBeforeStop = draws
  konvaPlayer.options.onLiveChange(false)
  assert.equal(Object.prototype.hasOwnProperty.call(imageNode, '_drawCachedSceneCanvas'), false)
  assert.equal(draws, drawsBeforeStop + 1)
  assert.equal(konvaPlayer.options.canRender(), false, 'an uncached node (temporal off) does no GPU work')
  controller.dispose()
  assert.equal(konvaPlayer.disposed, true, 'dispose releases the player')

  console.log('media-fx temporal regression passed')
}

run().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
