// Run with node scripts/media-fx-multipass-regression.cjs.
// Media FX V3.6 bloom / glow light pipeline: structural checks of the fixed multi-pass
// processor against a state-tracking mock WebGL2 (texture units, framebuffer attachments,
// per-draw program / viewport / bindings). Real shader pixels are not checked here; the
// headless Chrome smoke test covers them.
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

const programKind = (fragment) => {
  if (fragment.includes('#define MEDIA_FX_LIGHT')) return 'composite'
  if (fragment.includes('uBloomWeight')) return 'extract'
  if (fragment.includes('uDirection')) return 'blur'
  return 'main'
}

// State-tracking WebGL2 mock. Every draw records the state it ran with.
const createMock = () => ({
  contexts: 0, canvases: [], ids: 0, textures: [], framebuffers: [], programs: [], deleted: [],
  draws: [], reads: 0, uploads: [], allocations: [], error: 0, lost: false, maxTextureSize: 8192,
  failCompile: null, failDrawAt: -1, readError: false, incomplete: false, feedback: 0, warnings: [],
  listeners: [],
})

const createStatefulGl = (mock, canvas) => {
  const state = { unit: 0, units: [null, null], framebuffer: null, program: null, viewport: [0, 0, 0, 0] }
  const constants = {
    VERTEX_SHADER: 1, FRAGMENT_SHADER: 2, COMPILE_STATUS: 3, LINK_STATUS: 4, MAX_TEXTURE_SIZE: 5, NO_ERROR: 0,
    TEXTURE_2D: 6, RGBA8: 7, RGBA: 8, UNSIGNED_BYTE: 9, TRIANGLES: 10, TEXTURE0: 100, TEXTURE1: 101,
    FRAMEBUFFER: 20, COLOR_ATTACHMENT0: 21, FRAMEBUFFER_COMPLETE: 22, LINEAR: 23, NEAREST: 24,
  }
  const gl = {
    ...constants,
    state,
    get drawingBufferWidth() { return canvas.width },
    get drawingBufferHeight() { return canvas.height },
    createShader: (type) => ({ type }),
    shaderSource: (shader, source) => { shader.source = source },
    compileShader: () => {},
    getShaderParameter: (shader) => !(mock.failCompile && mock.failCompile(shader.source)),
    getShaderInfoLog: () => 'mock compile failure',
    deleteShader: () => {},
    createProgram: () => { const program = { id: ++mock.ids, shaders: [] }; mock.programs.push(program); return program },
    attachShader: (program, shader) => { program.shaders.push(shader) },
    linkProgram: (program) => {
      program.fragment = program.shaders.find(shader => shader.type === 2).source
      program.kind = programKind(program.fragment)
    },
    getProgramParameter: () => true,
    deleteProgram: (program) => mock.deleted.push(program),
    useProgram: (program) => { state.program = program },
    getUniformLocation: (program, name) => ({ program, name }),
    uniform1f: (location, value) => { location.program[location.name] = value },
    uniform1i: (location, value) => { location.program[location.name] = value },
    uniform2f: (location, x, y) => { location.program[location.name] = [x, y] },
    uniform2i: (location, x, y) => { location.program[location.name] = [x, y] },
    createTexture: () => { const texture = { id: ++mock.ids, width: 0, height: 0 }; mock.textures.push(texture); return texture },
    deleteTexture: (texture) => mock.deleted.push(texture),
    activeTexture: (unit) => { state.unit = unit - constants.TEXTURE0 },
    bindTexture: (_target, texture) => { state.units[state.unit] = texture },
    texParameteri: () => {},
    texImage2D: (...args) => {
      const texture = state.units[state.unit]
      if (args.length === 6) {
        mock.uploads.push({ texture, source: args[5] })
        return
      }
      texture.width = args[3]
      texture.height = args[4]
      mock.allocations.push({ texture, width: args[3], height: args[4], data: args[8] })
    },
    createFramebuffer: () => { const framebuffer = { id: ++mock.ids, attachment: null }; mock.framebuffers.push(framebuffer); return framebuffer },
    deleteFramebuffer: (framebuffer) => mock.deleted.push(framebuffer),
    // A pass must set its own viewport / program after choosing its target, so the mock
    // forgets both on every framebuffer bind instead of letting a pass inherit them.
    bindFramebuffer: (_target, framebuffer) => { state.framebuffer = framebuffer; state.viewport = ['stale']; state.program = { kind: 'stale' } },
    framebufferTexture2D: (_target, _attachment, _textarget, texture) => { state.framebuffer.attachment = texture },
    checkFramebufferStatus: () => (mock.incomplete ? 0 : constants.FRAMEBUFFER_COMPLETE),
    viewport: (...args) => { state.viewport = args },
    drawArrays: () => {
      const target = state.framebuffer?.attachment ?? null
      if (target && state.units.includes(target)) mock.feedback += 1
      const program = state.program
      mock.draws.push({
        kind: program.kind,
        program,
        framebuffer: state.framebuffer,
        target,
        targetSize: target ? [target.width, target.height] : [canvas.width, canvas.height],
        viewport: [...state.viewport],
        unit0: state.units[0],
        unit1: state.units[1],
        uniforms: { ...program },
      })
      if (mock.failDrawAt === mock.draws.length - 1) mock.error = 1282
    },
    readPixels: (_x, _y, _w, _h, _format, _type, target) => {
      mock.reads += 1
      if (mock.readError) { mock.error = 1282; return }
      target.fill(200)
    },
    getError: () => { const error = mock.error; mock.error = 0; return error },
    isContextLost: () => mock.lost,
    getParameter: () => mock.maxTextureSize,
  }
  return new Proxy(gl, { get: (target, key) => (key in target ? target[key] : () => {}) })
}

const installMock = (mock) => {
  global.WebGL2RenderingContext = function WebGL2RenderingContext() {}
  global.document = {
    createElement: () => {
      const canvas = { width: 1, height: 1 }
      canvas.getContext = () => {
        mock.contexts += 1
        mock.canvases.push(canvas)
        canvas.gl = createStatefulGl(mock, canvas)
        return canvas.gl
      }
      canvas.addEventListener = (type, listener) => { if (type === 'webglcontextlost') mock.listeners.push({ canvas, listener }) }
      return canvas
    },
  }
}

const image = (width = 8, height = 4, fill = 7) => ({ width, height, data: new Uint8ClampedArray(width * height * 4).fill(fill) })

async function run() {
  const originalDocument = global.document
  const originalWebGL2 = global.WebGL2RenderingContext
  const originalWarn = console.warn
  const mock = createMock()
  console.warn = (...args) => mock.warnings.push(args.join(' '))
  installMock(mock)
  const core = loadModule('./media-fx')
  const gpu = loadModule('./media-fx-gpu', {}, {})
  const off = core.createDefaultMediaFxAdvanced()
  const adv = patch => ({ ...off, ...patch })

  // --- renderer mapping and pixel budget (pure) -----------------------------------
  const lightParams = gpu.resolveMediaFxGpuLightParams
  assert.equal(lightParams(adv({ pixelate: 1, edge: 1 })).active, false, 'the nine original effects never activate the light pipeline')
  assert.equal(lightParams(off).active, false)
  const bloomParams = lightParams(adv({ bloom: 1 }))
  assert.equal(bloomParams.active, true)
  assert.deepEqual([bloomParams.bloomWeight, bloomParams.glowWeight, bloomParams.glowAlpha], [1, 0, 0], 'bloom alone extracts highlights only, no halo coverage')
  assert.ok(lightParams(adv({ bloom: 1 })).threshold < lightParams(adv({ bloom: 0.1 })).threshold, 'stronger bloom lowers the threshold')
  assert.ok(lightParams(adv({ bloom: 1 })).sigmaPx > lightParams(adv({ bloom: 0.1 })).sigmaPx, 'stronger bloom widens the blur')
  const glowParams = lightParams(adv({ glow: 0.5 }))
  assert.deepEqual([glowParams.bloomWeight, glowParams.glowWeight], [0, 1], 'glow alone uses every visible color')
  assert.ok(glowParams.glowAlpha > 0, 'glow may lift alpha inside the raster')
  const both = lightParams(adv({ bloom: 0.5, glow: 0.5 }))
  assert.ok(Math.abs(both.bloomWeight + both.glowWeight - 1) < 1e-9, 'extraction weights split the 0..1 range')
  assert.equal(lightParams(adv({ bloom: 0.5 }), { pixelRatio: 2 }).sigmaPx, lightParams(adv({ bloom: 0.5 })).sigmaPx * 2, 'blur size follows the raster pixel ratio')
  for (const value of Object.values(lightParams(adv({ bloom: 0.37, glow: 0.81 }), { pixelRatio: 1.5 }))) {
    assert.ok(typeof value === 'boolean' || Number.isFinite(value), 'light params are always finite')
  }

  const lightRaster = gpu.resolveMediaFxGpuLightRaster
  const small = lightRaster(64, 32, 4)
  assert.deepEqual([small.width, small.height], [32, 16], 'small inputs use half resolution')
  const k4 = lightRaster(3840, 2160, bloomParams.sigmaPx)
  assert.ok(k4.width * k4.height <= gpu.MEDIA_FX_GPU_LIGHT_MAX_PIXELS, '4K light targets stay within the pixel budget')
  assert.ok(k4.width < 3840 / 2 && k4.height < 2160 / 2, '4K never allocates a full-resolution scratch target')
  const k8 = lightRaster(8192, 8192, 4)
  assert.ok(k8.width * k8.height <= gpu.MEDIA_FX_GPU_LIGHT_MAX_PIXELS)
  const wide = lightRaster(2000, 2000, 200)
  assert.ok(wide.radius <= 16 && wide.sigma * 3 <= 16.0001, 'a wide blur downsamples until it fits the fixed kernel')
  assert.ok(lightRaster(1, 1, 4).width === 1 && lightRaster(1, 1, 4).height === 1, 'tiny inputs keep a 1x1 light target')
  assert.equal(lightRaster(0, 4, 4), null)
  assert.ok(lightRaster(10, 10, Number.NaN).sigma > 0, 'invalid sigma never produces NaN')

  // --- single-pass fast path: bloom / glow off ----------------------------------------
  const legacy = image()
  assert.equal(gpu.applyMediaFxGpu(legacy, adv({ pixelate: 0.4, rgbSplit: 0.2, scanline: 0.3, vignette: 0.4, grain: 0.2, posterize: 0.3, negative: 0.1, sharpen: 0.2, edge: 0.1 })), true)
  assert.equal(mock.contexts, 1)
  assert.equal(mock.draws.length, 1, 'the nine original effects keep the single pass')
  assert.equal(mock.draws[0].kind, 'main')
  assert.equal(mock.draws[0].target, null, 'single pass renders to the default framebuffer')
  assert.equal(mock.framebuffers.length, 0, 'bloom / glow off never creates a framebuffer')
  assert.deepEqual(mock.programs.map(program => program.kind), ['main'], 'bloom / glow off never compiles the light programs')
  assert.equal(mock.textures.length, 1, 'bloom / glow off allocates only the source texture')
  assert.equal(mock.reads, 1)

  // --- bloom: fixed multi-pass chain ------------------------------------------------------
  mock.draws = []
  const bloomImage = image(64, 32)
  assert.equal(gpu.applyMediaFxGpu(bloomImage, adv({ bloom: 0.6 })), true, 'bloom runs')
  assert.deepEqual(mock.draws.map(draw => draw.kind), ['extract', 'blur', 'blur', 'composite'], 'extract -> blur H -> blur V -> composite')
  assert.equal(mock.contexts, 1, 'the light pipeline shares the page-wide context')
  assert.equal(mock.framebuffers.length, 2, 'two scratch framebuffers (ping-pong)')
  assert.deepEqual(mock.programs.map(program => program.kind).sort(), ['blur', 'composite', 'extract', 'main'])
  const [extractDraw, blurH, blurV, compositeDraw] = mock.draws
  const [targetA, targetB] = mock.framebuffers.map(framebuffer => framebuffer.attachment)
  const expected = lightRaster(64, 32, lightParams(adv({ bloom: 0.6 })).sigmaPx)
  for (const draw of [extractDraw, blurH, blurV]) {
    assert.deepEqual(draw.viewport, [0, 0, expected.width, expected.height], `${draw.kind}: viewport is the light raster`)
    assert.deepEqual(draw.targetSize, [expected.width, expected.height], `${draw.kind}: target is allocated at the light raster size`)
    assert.equal(draw.unit1, null, `${draw.kind}: unit 1 is empty`)
  }
  assert.equal(extractDraw.target, targetA, 'extract writes A')
  assert.equal(extractDraw.unit0, mock.textures[0], 'extract reads the source texture')
  assert.deepEqual(extractDraw.uniforms.uSourceSize, [64, 32])
  assert.deepEqual(extractDraw.uniforms.uLightSize, [expected.width, expected.height])
  assert.equal(blurH.target, targetB, 'blur H writes B')
  assert.equal(blurH.unit0, targetA, 'blur H reads A')
  assert.deepEqual(blurH.uniforms.uDirection, [1, 0])
  assert.equal(blurV.target, targetA, 'blur V writes A')
  assert.equal(blurV.unit0, targetB, 'blur V reads B')
  assert.deepEqual(blurV.uniforms.uDirection, [0, 1])
  assert.equal(blurV.uniforms.uRadius, expected.radius)
  assert.equal(compositeDraw.target, null, 'composite renders to the default framebuffer')
  assert.deepEqual(compositeDraw.viewport, [0, 0, 64, 32], 'composite viewport is the full raster')
  assert.equal(compositeDraw.unit0, mock.textures[0], 'composite reads the source on unit 0')
  assert.equal(compositeDraw.unit1, targetA, 'composite reads the blurred light on unit 1')
  assert.equal(compositeDraw.uniforms.uLight, 1)
  assert.equal(compositeDraw.uniforms.uTexture, 0)
  assert.ok(compositeDraw.uniforms.uLightGain > 0)
  assert.equal(compositeDraw.uniforms.uGlowAlpha, 0, 'bloom alone has no glow halo coverage')
  assert.equal(compositeDraw.uniforms.uFlipY, 0, 'static composite never flips')
  assert.equal(mock.feedback, 0, 'no texture is read while it is the render target')
  assert.equal(mock.reads, 2, 'one readPixels per static pass')
  assert.ok(bloomImage.data.every(value => value === 200), 'the complete result is read back')
  // Scratch storage is released after the pass; the objects are kept.
  assert.deepEqual([targetA.width, targetA.height, targetB.width, targetB.height], [1, 1, 1, 1], 'scratch targets shrink to 1x1 after the pass')
  assert.deepEqual([mock.textures[0].width, mock.textures[0].height], [1, 1], 'the source texture shrinks too')
  assert.deepEqual([mock.canvases[0].width, mock.canvases[0].height], [1, 1], 'the drawing buffer shrinks too')
  assert.equal(mock.canvases[0].gl.state.framebuffer, null, 'the default framebuffer is bound again')
  assert.equal(mock.canvases[0].gl.state.units[1], null, 'unit 1 is released again')

  // --- glow, bloom + glow: same pipeline, objects reused ------------------------------------
  const programsBefore = mock.programs.length
  const texturesBefore = mock.textures.length
  mock.draws = []
  assert.equal(gpu.applyMediaFxGpu(image(64, 32), adv({ glow: 0.5 })), true, 'glow runs')
  assert.deepEqual(mock.draws.map(draw => draw.kind), ['extract', 'blur', 'blur', 'composite'], 'glow uses the same fixed chain')
  assert.ok(mock.draws[3].uniforms.uGlowAlpha > 0, 'glow sets its halo coverage')
  mock.draws = []
  // Poison the GL state between passes: every pass must set what it needs itself.
  const poisoned = mock.canvases[0].gl
  poisoned.useProgram(mock.programs.find(program => program.kind === 'blur'))
  poisoned.bindFramebuffer(poisoned.FRAMEBUFFER, mock.framebuffers[1])
  poisoned.activeTexture(poisoned.TEXTURE1)
  poisoned.bindTexture(poisoned.TEXTURE_2D, targetB)
  poisoned.viewport(0, 0, 3, 3)
  assert.equal(gpu.applyMediaFxGpu(image(96, 48), adv({ bloom: 0.4, glow: 0.4, negative: 0.5 }), { pixelRatio: 2 }), true, 'bloom + glow + an original effect')
  assert.deepEqual(mock.draws.map(draw => draw.kind), ['extract', 'blur', 'blur', 'composite'], 'bloom + glow share one chain')
  assert.equal(mock.programs.length, programsBefore, 'programs are compiled once and reused')
  assert.equal(mock.textures.length, texturesBefore, 'scratch textures are reused')
  assert.equal(mock.framebuffers.length, 2, 'scratch framebuffers are reused')
  assert.equal(mock.contexts, 1, 'still one WebGL2 context')
  assert.equal(mock.feedback, 0)
  assert.equal(mock.draws[0].unit1, null, 'a stale unit 1 binding is cleared before the light passes')
  assert.equal(mock.draws[0].unit0, mock.textures[0])
  const resized = lightRaster(96, 48, lightParams(adv({ bloom: 0.4, glow: 0.4 }), { pixelRatio: 2 }).sigmaPx)
  assert.deepEqual(mock.draws[1].viewport, [0, 0, resized.width, resized.height], 'a new input size resizes the scratch targets')
  assert.deepEqual(mock.draws[3].viewport, [0, 0, 96, 48])
  assert.equal(mock.draws[3].uniforms.uNegative, 0.5, 'original effects run in the composite pass')
  assert.equal(mock.draws[3].uniforms.uTime, 0)

  // Over budget: a 4K input only allocates budget-sized scratch targets.
  mock.allocations = []
  mock.draws = []
  assert.equal(gpu.applyMediaFxGpu(image(3840, 2160, 1), adv({ bloom: 1 })), true)
  const scratch = mock.allocations.filter(entry => entry.texture === targetA || entry.texture === targetB)
  const largest = Math.max(...scratch.map(entry => entry.width * entry.height))
  assert.ok(largest <= gpu.MEDIA_FX_GPU_LIGHT_MAX_PIXELS, '4K scratch targets stay within the budget')
  assert.equal(scratch.filter(entry => entry.width * entry.height > 1).length, 2, 'exactly two scratch allocations per pass')
  assert.deepEqual(mock.draws[3].viewport, [0, 0, 3840, 2160], 'the output keeps the full raster size')
  assert.equal(gpu.applyMediaFxGpu(image(9000, 8, 1), adv({ bloom: 1 })), false, 'MAX_TEXTURE_SIZE is respected')

  // --- failures never commit a partial result ------------------------------------------
  const assertUntouched = (name, setup, teardown) => {
    const input = image(32, 16, 9)
    const readsBefore = mock.reads
    setup()
    assert.equal(gpu.applyMediaFxGpu(input, adv({ bloom: 0.5, glow: 0.2 })), false, `${name}: reports failure`)
    teardown()
    assert.ok(input.data.every(value => value === 9), `${name}: the source ImageData is untouched`)
    assert.equal(mock.reads, readsBefore, `${name}: nothing is read back`)
    assert.deepEqual([targetA.width, targetA.height], [1, 1], `${name}: scratch storage is released`)
    assert.equal(mock.canvases.at(-1).gl.state.framebuffer, null, `${name}: the default framebuffer is restored`)
  }
  for (const [index, pass] of ['extract', 'blur H', 'blur V', 'composite'].entries()) {
    assertUntouched(`${pass} GL error`, () => { mock.draws = []; mock.failDrawAt = index }, () => { mock.failDrawAt = -1 })
  }
  const readFailure = image(32, 16, 9)
  mock.readError = true
  assert.equal(gpu.applyMediaFxGpu(readFailure, adv({ bloom: 0.5 })), false, 'readPixels failure reports no-op')
  mock.readError = false
  assert.ok(readFailure.data.every(value => value === 9), 'readPixels failure keeps the source pixels')
  assert.equal(gpu.mediaFxGpuSupported(), true, 'light failures never disable the GPU path')
  mock.draws = []
  assert.equal(gpu.applyMediaFxGpu(image(), adv({ bloom: 0.5 })), true, 'a later pass works again')

  // --- context loss: lazy recovery with a fresh pipeline ------------------------------------
  mock.lost = true
  mock.listeners.filter(entry => entry.canvas === mock.canvases.at(-1)).forEach(entry => entry.listener())
  mock.lost = false
  mock.draws = []
  assert.equal(gpu.applyMediaFxGpu(image(), adv({ glow: 0.5 })), true, 'a lost context is rebuilt lazily')
  assert.equal(mock.contexts, 2)
  assert.deepEqual(mock.draws.map(draw => draw.kind), ['extract', 'blur', 'blur', 'composite'])
  assert.ok(mock.draws.every(draw => !draw.target || mock.canvases[1].gl && mock.framebuffers.slice(2).some(framebuffer => framebuffer.attachment === draw.target)), 'the new context uses its own scratch targets')
  assert.equal(mock.framebuffers.length, 4, 'the new context builds its light pipeline once')

  // --- live frames never touch the light pipeline -------------------------------------------
  const session = gpu.createMediaFxTemporalGpuSession()
  session.setBaseline({ source: { id: 'baseline-with-bloom' }, width: 32, height: 16, pixelRatio: 1 })
  const temporal = { ...core.createDefaultMediaFxTemporal(), glitch: 0.5, flicker: 0.3 }
  mock.draws = []
  const allocationsBefore = mock.allocations.length
  for (let index = 0; index < 30; index += 1) assert.ok(session.render(temporal, index / 30))
  assert.equal(mock.draws.length, 30, 'one draw per live frame')
  assert.ok(mock.draws.every(draw => draw.kind === 'main' && draw.target === null), 'live frames use the single-pass program on the default framebuffer')
  assert.ok(mock.draws.every(draw => draw.unit0 !== null && !mock.framebuffers.some(framebuffer => framebuffer.attachment === draw.unit0)), 'live frames never sample a light target')
  assert.equal(mock.uploads.length, 1, 'the bloom baseline is uploaded once')
  assert.equal(mock.allocations.length, allocationsBefore, 'live frames never allocate scratch storage')
  // A static rebuild between frames does not disturb the live session.
  assert.equal(gpu.applyMediaFxGpu(image(), adv({ bloom: 0.3 })), true)
  mock.draws = []
  assert.ok(session.render(temporal, 2))
  assert.equal(mock.draws[0].kind, 'main')
  assert.equal(mock.draws[0].target, null, 'a live frame after a light pass renders to the default framebuffer')
  assert.equal(mock.uploads.length, 1, 'a static pass never forces a live re-upload')
  session.dispose()

  // --- light build failure only disables bloom / glow ---------------------------------------
  const failing = createMock()
  installMock(failing)
  console.warn = (...args) => failing.warnings.push(args.join(' '))
  failing.failCompile = source => source.includes('uDirection')
  const isolated = loadModule('./media-fx-gpu', {}, {})
  const kept = image(8, 4, 3)
  assert.equal(isolated.applyMediaFxGpu(kept, adv({ bloom: 0.5 })), false, 'a light program failure fails the bloom pass')
  assert.ok(kept.data.every(value => value === 3), 'and keeps the source pixels')
  assert.equal(isolated.mediaFxGpuSupported(), true, 'the GPU path stays available')
  const lightAvailability = []
  const stopLightAvailability = isolated.subscribeMediaFxGpuLightAvailability(available => lightAvailability.push(available))
  assert.equal(isolated.mediaFxGpuLightSupported(), false, 'a deterministic light build failure is exposed separately from the main GPU path')
  assert.deepEqual(lightAvailability, [], 'subscribing does not synthesize a duplicate initial event')
  assert.equal(isolated.applyMediaFxGpu(image(), adv({ edge: 0.5 })), true, 'the original effects keep working')
  const compiledPrograms = failing.programs.length
  assert.equal(isolated.applyMediaFxGpu(image(), adv({ glow: 0.5 })), false)
  assert.equal(isolated.applyMediaFxGpu(image(), adv({ bloom: 0.9 })), false)
  assert.equal(failing.programs.length, compiledPrograms, 'a failed light pipeline is not rebuilt on every pass')
  assert.equal(failing.warnings.filter(text => text.includes('光效')).length, 1, 'the failure warns once')
  assert.ok(failing.deleted.length > 0, 'partially created light objects are deleted')
  assert.equal(failing.framebuffers.length, 0, 'no framebuffer survives a failed build')
  stopLightAvailability()

  // An incomplete RGBA8 framebuffer is also deterministic for the current context: latch
  // only the light path, notify consumers once, and retry after context loss with a fresh
  // processor. Per-draw / readback errors above remain transient and do not latch.
  const incomplete = createMock()
  installMock(incomplete)
  console.warn = (...args) => incomplete.warnings.push(args.join(' '))
  const fboIsolated = loadModule('./media-fx-gpu', {}, {})
  const fboAvailability = []
  const stopFboAvailability = fboIsolated.subscribeMediaFxGpuLightAvailability(available => fboAvailability.push(available))
  assert.equal(fboIsolated.mediaFxGpuLightSupported(), true)
  incomplete.incomplete = true
  assert.equal(fboIsolated.applyMediaFxGpu(image(), adv({ bloom: 0.5 })), false)
  assert.equal(fboIsolated.mediaFxGpuSupported(), true, 'framebuffer failure does not disable the old advanced path')
  assert.equal(fboIsolated.mediaFxGpuLightSupported(), false, 'framebuffer failure latches only the light path')
  assert.deepEqual(fboAvailability, [false], 'the light capability downgrade is reported once')
  const fboPrograms = incomplete.programs.length
  incomplete.incomplete = false
  assert.equal(fboIsolated.applyMediaFxGpu(image(), adv({ glow: 0.5 })), false, 'later light edits do not repeat a deterministic failure')
  assert.equal(incomplete.programs.length, fboPrograms, 'latched light failure does not rebuild programs')
  assert.equal(fboIsolated.applyMediaFxGpu(image(), adv({ edge: 0.5 })), true, 'single-pass advanced remains usable after light latch')

  incomplete.lost = true
  incomplete.listeners.filter(entry => entry.canvas === incomplete.canvases.at(-1)).forEach(entry => entry.listener())
  incomplete.lost = false
  assert.equal(fboIsolated.mediaFxGpuLightSupported(), true, 'context loss returns light support to optimistic state for the replacement context')
  assert.deepEqual(fboAvailability, [false, true], 'consumers are told when a fresh context may recover light effects')
  assert.equal(fboIsolated.applyMediaFxGpu(image(), adv({ glow: 0.5 })), true, 'a fresh context retries and can recover the light pipeline')
  stopFboAvailability()

  // --- shader structure: the light variant only inserts the composite --------------------------
  const sources = mock.programs.map(program => program.fragment)
  const main = sources.find(source => programKind(source) === 'main')
  const composite = sources.find(source => programKind(source) === 'composite')
  assert.equal(composite, main.replace('#version 300 es\n', '#version 300 es\n#define MEDIA_FX_LIGHT 1\n'), 'the light variant is the main shader plus one define')
  const order = ['uPixelate > 1.0', 'uRgbSplit > 0.0', 'uSharpen > 0.0 || uEdge > 0.0', 'uPosterizeLevels > 1.0', 'uNegative > 0.0', 'uGrain > 0.0', 'uScanline > 0.0', 'uVignette > 0.0']
  const positions = order.map(token => main.indexOf(`if (${token})`))
  assert.deepEqual([...positions].sort((a, b) => a - b), positions, 'the original effect order is unchanged')
  const lightBlock = main.indexOf('vec4 light = texture(uLight')
  assert.ok(lightBlock > positions[4] && lightBlock < positions[5], 'light composites after negative and before grain / scanline / vignette')
  assert.ok(main.lastIndexOf('#ifdef MEDIA_FX_LIGHT') < lightBlock && main.indexOf('#endif', lightBlock) < positions[5], 'the composite is compiled only into the light variant')
  assert.equal((main.match(/uniform sampler2D/g) || []).length, 2, 'uTexture plus the light-variant-only uLight')
  const blurShader = sources.find(source => programKind(source) === 'blur')
  assert.ok(/for \(int i = -16; i <= 16; i\+\+\)/.test(blurShader), 'the blur kernel has a constant bound')
  const gpuSource = fs.readFileSync(path.join(featureDir, 'media-fx-gpu.ts'), 'utf8')
  assert.equal(/requestAnimationFrame|setTimeout|setInterval|new Worker/.test(gpuSource), false, 'no RAF / timer / worker in the GPU adapter')
  assert.equal(/getImageData|putImageData/.test(gpuSource), false, 'the blur never runs over CPU ImageData')

  console.warn = originalWarn
  if (originalDocument === undefined) delete global.document
  else global.document = originalDocument
  if (originalWebGL2 === undefined) delete global.WebGL2RenderingContext
  else global.WebGL2RenderingContext = originalWebGL2
  console.log('media-fx multipass regression passed')
}

run().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
