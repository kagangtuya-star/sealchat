// Run with node scripts/media-fx-motion-bounds-regression.cjs.
// Pure motion bounds and StageApp content transforms; no Vue mount or browser.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')
const { parse } = require('@vue/compiler-sfc')

const compile = source => ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText
const coreSource = fs.readFileSync(path.join(__dirname, '../src/features/media-fx/media-fx.ts'), 'utf8')
const mediaFx = {}
new Function('exports', compile(coreSource))(mediaFx)
const { createDefaultMediaFxSpec, mediaFxHasContent, mediaFxMotionPresets, resolveMediaFxMotionTrack, resolveMediaFxMotionOverscanScale } = mediaFx
const motion = (preset, intensity = 1) => ({ preset, intensity, durationMs: 4000, loop: true })

for (const [preset, expected] of Object.entries({ none: 1, shake: 1.048, 'shake-x': 1.048, 'shake-y': 1.048, float: 1.08, drift: 1.06, breathe: 1, zoom: 1 })) {
  assert.equal(resolveMediaFxMotionOverscanScale(motion(preset)), expected, preset)
  assert.equal(resolveMediaFxMotionOverscanScale(motion(preset, 0)), 1, `${preset}: zero intensity`)
}
for (const preset of mediaFxMotionPresets) {
  for (const intensity of [0.001, 0.125, 0.333, 0.5, 0.999, 1]) {
    const spec = motion(preset, intensity)
    const overscan = resolveMediaFxMotionOverscanScale(spec)
    assert.ok(overscan >= 1)
    assert.equal(overscan, Number(overscan.toFixed(4)), 'normalized to four decimal places')
    for (const frame of resolveMediaFxMotionTrack(spec)?.frames ?? []) {
      assert.ok(overscan * frame.scale / 2 + 1e-12 >= 0.5 + Math.abs(frame.x), `${preset}: horizontal coverage at ${intensity}`)
      assert.ok(overscan * frame.scale / 2 + 1e-12 >= 0.5 + Math.abs(frame.y), `${preset}: vertical coverage at ${intensity}`)
    }
  }
}

// An isolated future track verifies compensation for motion scales below one.
const shrinkingFx = {}
new Function('exports', compile(coreSource + `
  mediaFxMotionShapes.zoom.frames = () => [
    { x: 0, y: 0, scale: 1 },
    { x: 0.05, y: -0.03, scale: 0.5 },
  ]
`))(shrinkingFx)
assert.equal(shrinkingFx.resolveMediaFxMotionOverscanScale(motion('zoom')), 2.2)

async function run() {
  const Konva = (await import('konva')).default
  const filename = path.join(__dirname, '../src/views/theater/stage/StageApp.vue')
  const script = parse(fs.readFileSync(filename, 'utf8')).descriptor.scriptSetup.content
  const ast = ts.createSourceFile(filename + '.ts', script, ts.ScriptTarget.Latest, true)
  const declarations = ['surfaceMediaFxOverscanScale', 'syncSurfaceMediaFx', 'surfaceMediaFxTemporal', 'useDirectSurfaceImage'].map(name => {
    const statement = ast.statements.find(node => ts.isVariableStatement(node)
      && node.declarationList.declarations.some(declaration => declaration.name.getText(ast) === name))
    assert.ok(statement, `missing StageApp function ${name}`)
    return statement.getText(ast)
  }).join('\n')
  let reducedMotion = false
  const { syncSurfaceMediaFx: sync } = new Function(
    'resolveMediaFxMotionOverscanScale', 'mediaFxHasContent', 'resolveTheaterReducedMotion',
    'mediaFxTemporalHasContent', 'mediaFxGpuSupported', 'isVideoSource',
    compile(declarations) + '\nreturn { syncSurfaceMediaFx }',
  )(
    resolveMediaFxMotionOverscanScale, mediaFxHasContent, () => ({ effectiveReducedMotion: reducedMotion }),
    mediaFx.mediaFxTemporalHasContent, () => true, source => source.video === true,
  )
  const temporalUpdates = []
  const updates = []
  let clears = 0
  let motionActive = false
  let motionSignature = ''
  const slot = {
    target: 'background', source: {}, animatedMedia: false,
    mediaContentGroup: new Konva.Group(),
    mediaFxController: {
      update: (spec, context) => {
        updates.push({ spec, context })
        const nextSignature = context.reducedMotion === true || spec.motion.preset === 'none' || spec.motion.intensity <= 0
          ? ''
          : `${spec.motion.preset}|${spec.motion.intensity}|${spec.motion.durationMs}|${spec.motion.loop}`
        if (nextSignature === motionSignature) return
        motionSignature = nextSignature
        motionActive = Boolean(nextSignature)
      },
      isMotionActive: () => motionActive,
      clear: () => { clears++; motionActive = false; motionSignature = '' },
    },
    mediaFxTemporal: { update: (temporal, active) => temporalUpdates.push({ temporal, active }) },
    directImage: { visible: () => true },
    style: { fit: 'cover', zoom: 1.2, mediaFx: { ...createDefaultMediaFxSpec(), motion: motion('drift') } },
  }
  const box = { width: 800, height: 600 }
  const expectScale = (expected, size = box) => {
    sync(slot, size)
    assert.deepEqual(slot.mediaContentGroup.scale(), { x: expected, y: expected })
    assert.deepEqual(slot.mediaContentGroup.position(), { x: size.width / 2, y: size.height / 2 })
    assert.deepEqual(slot.mediaContentGroup.offset(), { x: size.width / 2, y: size.height / 2 })
    assert.equal(slot.style.zoom, 1.2, 'overscan never changes the base zoom')
  }
  expectScale(1.06)
  assert.equal(updates.at(-1).context.filters, false, 'Surface filter pipeline stays separate')
  slot.style.fit = 'fill'
  expectScale(1.06)
  for (const fit of ['contain', 'center', 'tile']) {
    slot.style.fit = fit
    expectScale(1)
  }
  slot.style.fit = 'cover'
  slot.target = 'foreground'
  expectScale(1)
  slot.target = 'background'
  reducedMotion = true
  expectScale(1)
  assert.equal(updates.at(-1).context.reducedMotion, true)
  reducedMotion = false
  expectScale(1.06, { width: 1200, height: 900 })
  for (const preset of ['shake', 'shake-x', 'shake-y', 'float', 'drift', 'breathe', 'zoom', 'none']) {
    slot.style.mediaFx.motion = motion(preset)
    expectScale(resolveMediaFxMotionOverscanScale(slot.style.mediaFx.motion))
  }
  slot.style.mediaFx.motion = motion('drift', 0)
  expectScale(1)
  slot.style.mediaFx.motion = motion('drift', 0.5)
  expectScale(1.03)
  // A completed one-shot stays at base scale on later syncs instead of reapplying overscan.
  slot.style.mediaFx.motion = { ...motion('drift'), loop: false }
  expectScale(1.06)
  motionActive = false
  slot.mediaContentGroup.scale({ x: 1, y: 1 })
  expectScale(1)

  // Temporal never changes the motion overscan, and only runs on static non-tile images.
  slot.style.mediaFx.motion = motion('drift')
  slot.style.mediaFx.temporal = { ...createDefaultMediaFxSpec().temporal, grain: 0.5 }
  expectScale(1.06)
  assert.equal(temporalUpdates.at(-1).active, true, 'static cover surface runs temporal')
  assert.equal(temporalUpdates.at(-1).temporal.grain, 0.5)
  reducedMotion = true
  expectScale(1)
  assert.equal(temporalUpdates.at(-1).active, false, 'reduced motion stops temporal')
  reducedMotion = false
  slot.style.fit = 'tile'
  expectScale(1)
  assert.equal(temporalUpdates.at(-1).active, false, 'tile never runs temporal')
  slot.style.fit = 'cover'
  slot.style.mediaFx.motion = motion('none')
  expectScale(1)
  assert.equal(temporalUpdates.at(-1).active, true, 'temporal-only surfaces keep base scale')
  slot.style.mediaFx.temporal = createDefaultMediaFxSpec().temporal
  slot.style.mediaFx.motion = motion('drift')
  expectScale(1.06)
  assert.equal(temporalUpdates.at(-1).active, false)

  for (const source of [{ video: true }, { animated: true }]) {
    slot.source = source
    slot.animatedMedia = true
    slot.style.mediaFx.temporal = { ...createDefaultMediaFxSpec().temporal, glitch: 0.5 }
    expectScale(1.06)
    assert.equal(updates.at(-1).context.filters, false, 'animated media never enables cached filters')
    assert.equal(temporalUpdates.at(-1).active, false, 'animated media / video never run temporal')
    assert.equal(slot.style.mediaFx.temporal.glitch, 0.5, 'unsupported media keeps the temporal data')
    slot.style.mediaFx.temporal = createDefaultMediaFxSpec().temporal
  }
  slot.source = null
  expectScale(1)
  const before = clears
  slot.source = {}
  delete slot.style.mediaFx
  expectScale(1, { width: 1000, height: 700 })
  assert.equal(clears, before + 1, 'reset clears motion as well as overscan')
  console.log('media fx motion bounds regression checks passed')
}

run().catch(error => { console.error(error); process.exitCode = 1 })
