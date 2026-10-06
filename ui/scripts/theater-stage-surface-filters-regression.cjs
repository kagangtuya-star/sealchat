// Run with node scripts/theater-stage-surface-filters-regression.cjs.
// Exercises StageApp's actual update functions with real Konva filters, without a browser.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')
const { parse } = require('@vue/compiler-sfc')

const compile = source => ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText

async function run() {
  const Konva = (await import('konva')).default
  const filename = path.join(__dirname, '../src/views/theater/stage/StageApp.vue')
  const script = parse(fs.readFileSync(filename, 'utf8')).descriptor.scriptSetup.content
  const ast = ts.createSourceFile(filename + '.ts', script, ts.ScriptTarget.Latest, true)
  const names = [
    'surfaceMediaFxFilterCss', 'surfaceFilterCss', 'surfaceMediaFxCanvasFilter',
    'useDirectSurfaceImage', 'clearDirectSurfaceImage', 'updateDirectSurfaceImage',
  ]
  const declarations = names.map(name => {
    const statement = ast.statements.find(node => ts.isVariableStatement(node)
      && node.declarationList.declarations.some(declaration => declaration.name.getText(ast) === name))
    assert.ok(statement, `missing StageApp function ${name}`)
    return statement.getText(ast)
  }).join('\n')
  const mediaFxModule = { exports: {} }
  new Function('exports', compile(fs.readFileSync(path.join(__dirname, '../src/features/media-fx/media-fx.ts'), 'utf8')))(mediaFxModule.exports)
  const { createDefaultMediaFxSpec, mediaFxFilterToCss } = mediaFxModule.exports
  let supported = true
  const cssDraws = []
  // Canvas calls are observed; browser pixel rendering is not simulated.
  const document = { createElement: () => {
    const canvas = {}
    const context = {
      putImageData: data => { canvas.data = new Uint8ClampedArray(data.data) },
      drawImage: source => {
        cssDraws.push({ css: context.filter, data: new Uint8ClampedArray(source.data) })
        canvas.data = source.data
      },
      getImageData: () => ({ data: canvas.data }),
    }
    canvas.getContext = () => context
    return canvas
  } }
  const { updateDirectSurfaceImage: update, clearDirectSurfaceImage: clear, surfaceFilterCss } = new Function(
    'Konva', 'document', 'mediaFxFilterToCss', 'canvasFilterSupported',
    compile(`
      const theaterMediaDebug = () => {}
      const isVideoSource = (source) => source.video === true
      const stageMediaDimensions = (source) => source
      const stageMediaObjectUrls = new WeakMap()
      const surfaceDrawRect = (_source, width, height) => ({ x: 0, y: 0, width, height })
      ${declarations}
    `) + '\nreturn { updateDirectSurfaceImage, clearDirectSurfaceImage, surfaceFilterCss }',
  )(Konva, document, mediaFxFilterToCss, () => supported)
  const image = new Konva.Image()
  let caches = 0, clears = 0
  image.cache = () => { caches++; return image }
  image.clearCache = () => { clears++; return image }
  const source = { width: 10, height: 10 }
  const box = { width: 10, height: 10 }
  const slot = {
    directImage: image, directImageSource: null, directImageSignature: '', source,
    animatedMedia: false,
    style: { brightness: 1.5, blurPx: 8, opacity: 1, zoom: 1, fit: 'cover' },
  }
  const legacy = [Konva.Filters.Brighten, Konva.Filters.Blur]
  update(slot, source, box)
  assert.deepEqual(image.filters(), legacy)
  assert.equal(image.brightness(), 0.5)
  assert.equal(image.blurRadius(), 8)
  assert.equal(caches, 1)
  const pixels = () => ({ width: 10, height: 10, data: new Uint8ClampedArray(400).fill(80) })
  const expected = pixels()
  legacy.forEach(filter => filter.call(image, expected))

  slot.style.mediaFx = createDefaultMediaFxSpec()
  slot.style.mediaFx.filter.grayscale = 0.5
  update(slot, source, box)
  assert.deepEqual(image.filters().slice(0, 2), legacy)
  assert.equal(image.filters().length, 3)
  assert.equal(typeof image.filters()[2], 'function', 'avoid Konva mixed-string fallback')
  assert.equal(caches, 2, 'only one cache per changed update')
  const actual = pixels()
  image.filters().forEach(filter => filter.call(image, actual))
  assert.deepEqual(cssDraws.at(-1).data, expected.data, 'CSS receives exactly the legacy-filtered pixels')
  assert.equal(cssDraws.at(-1).css, 'grayscale(0.5)')
  assert.equal(image.brightness(), 0.5)
  assert.equal(image.blurRadius(), 8)
  update(slot, source, box)
  assert.equal(caches, 2, 'unchanged signature reuses cache')

  Object.assign(slot.style.mediaFx.filter, { brightness: 0.8, contrast: 1.2, saturation: 0.6, sepia: 0.2, hueRotate: 10, blurPx: 2 })
  update(slot, source, box)
  assert.equal(caches, 3, 'Media FX filter changes invalidate cache')
  image.filters()[2].call(image, pixels())
  const fullCss = mediaFxFilterToCss(slot.style.mediaFx.filter)
  assert.equal(cssDraws.at(-1).css, fullCss, 'all CSS tokens reach the native canvas together')
  assert.equal(image.brightness(), 0.5, 'Media FX never mutates legacy brightness')
  assert.equal(image.blurRadius(), 8, 'Media FX never mutates legacy blur')
  assert.equal(surfaceFilterCss(slot.style, fullCss), `brightness(1.5) blur(8px) ${fullCss}`, 'Shape retains legacy CSS semantics')

  supported = false
  update(slot, source, box)
  assert.deepEqual(image.filters(), legacy, 'unsupported Canvas CSS keeps only legacy filters')
  assert.equal(caches, 4)
  supported = true
  delete slot.style.mediaFx
  update(slot, source, box)
  assert.deepEqual(image.filters(), legacy)
  assert.equal(caches, 4, 'removing an already skipped filter can reuse the legacy cache')
  slot.style.brightness = 1
  slot.style.blurPx = 0
  update(slot, source, box)
  assert.equal(image.brightness(), 0)
  assert.equal(image.blurRadius(), 0)
  assert.deepEqual(image.filters(), [])
  assert.equal(caches, 4, 'default filters need no cache')

  slot.style.mediaFx = createDefaultMediaFxSpec()
  slot.style.mediaFx.filter.grayscale = 0.5
  update(slot, source, box)
  assert.deepEqual(image.filters(), ['grayscale(0.5)'], 'CSS-only pipeline stays native')
  assert.equal(caches, 5)
  clear(slot)
  assert.deepEqual(image.filters(), [])
  assert.equal(image.brightness(), 0)
  assert.equal(image.blurRadius(), 0)
  assert.equal(image.image(), undefined)
  assert.equal(clears, 7, 'each changed update and clear invalidates exactly once')
  console.log('theater stage surface filter regression checks passed')
}

run().catch(error => { console.error(error); process.exitCode = 1 })
