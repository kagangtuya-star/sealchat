// Exercises the real preview setup with delayed prop echoes, as with the theater iframe.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const Module = require('node:module')
const ts = require('typescript')
const compiler = require('@vue/compiler-sfc')
const Vue = require('vue')
const root = path.resolve(__dirname, '..')
const filename = path.join(root, 'src/components/theater-presentation/TheaterPresentationPreview.vue')
const unmounted = []
const originalLoad = Module._load, originalResolve = Module._resolveFilename
const originalTsLoader = require.extensions['.ts']
const originalWindow = global.window
const originalRequestFrame = global.requestAnimationFrame, originalCancelFrame = global.cancelAnimationFrame
let frame
global.window = { addEventListener() {}, removeEventListener() {} }
global.requestAnimationFrame = callback => { frame = callback; return 1 }
global.cancelAnimationFrame = () => { frame = null }
Module._load = function (request, ...args) {
  if (request === 'vue') return { ...Vue, onBeforeUnmount: fn => unmounted.push(fn) }
  if (request === '@/services/font/platformFontRegistry') return { resolvePlatformFontFamily: async () => '' }
  if (request === '@/features/media-fx/media-fx-dom') return { vMediaFx: {} }
  if (request === '@/views/theater/shared/theater-reduced-motion') return { resolveTheaterReducedMotion: () => ({ effectiveReducedMotion: Vue.ref(false) }) }
  if (request.endsWith('.css') || request.endsWith('.vue')) return {}
  return originalLoad.call(this, request, ...args)
}
Module._resolveFilename = function (request, parent, ...args) {
  if (request.startsWith('@/')) request = path.join(root, 'src', request.slice(2))
  return originalResolve.call(this, request, parent, ...args)
}
const compile = source => ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true } }).outputText
require.extensions['.ts'] = (module, file) => module._compile(compile(fs.readFileSync(file, 'utf8')), file)

async function run() {
  const { createTheaterVisualLayer } = require('../src/components/theater-presentation/theaterPresentationEditorState.ts')
  const { createDefaultTheaterPresentation, createDefaultTheaterTransform, normalizeTheaterTransform } = require('../src/types/theaterPresentation.ts')
  const descriptor = compiler.parse(fs.readFileSync(filename, 'utf8'), { filename }).descriptor
  const compiled = new Module(filename, module)
  compiled.filename = filename; compiled.paths = Module._nodeModulePaths(path.dirname(filename))
  compiled._compile(compile(compiler.compileScript(descriptor, { id: 'preview-regression' }).content), filename)

  for (const mode of ['dialogue', 'portrait', 'controller', 'multiplayer']) {
    const presentation = createDefaultTheaterPresentation()
    presentation.portrait = createTheaterVisualLayer({ assetId: 'preview', resourceAttachmentId: 'preview', mimeType: 'image/png', kind: 'static_image', width: 100, height: 150 }, 'viewport', 'preview')
    const initial = { ...createDefaultTheaterTransform(), x: 0.2, y: 0.1, height: 0.5 }
    if (mode === 'dialogue') presentation.dialogue.transform = initial
    else presentation.portrait.transform = initial
    const target = { kind: mode === 'dialogue' ? 'dialogue' : 'portrait' }
    const props = Vue.reactive({ draft: presentation, selection: target, activeSection: target.kind,
      ...(mode === 'controller' ? { controllerArea: initial } : {}),
      ...(mode === 'multiplayer' ? { multiplayerPortraitTransform: initial } : {}),
    })
    const scope = Vue.effectScope(), commands = [], phases = []
    const preview = scope.run(() => compiled.exports.default.setup(props, { expose() {}, emit: (name, command) => {
      if (name === 'dispatch' && command.type === 'set-transform') commands.push(command)
      if (name !== 'dispatch') phases.push(name)
    } }))
    const container = { getBoundingClientRect: () => ({ left: 0, top: 0, width: 1000, height: 500 }) }
    preview.compositionRef.value = container
    const pointer = (x, y) => ({ clientX: x, clientY: y, currentTarget: { closest: () => container }, preventDefault() {}, stopPropagation() {} })
    const style = () => mode === 'dialogue' ? preview.dialogueStyle.value : mode === 'controller' ? preview.controllerAreaStyle.value : preview.portraitRootStyle.value
    const getTransform = () => mode === 'dialogue' ? props.draft.dialogue.transform : props.multiplayerPortraitTransform || props.controllerArea || props.draft.portrait.transform
    const echo = transform => {
      if (mode === 'controller') props.controllerArea = transform
      else if (mode === 'multiplayer') props.multiplayerPortraitTransform = transform
      else if (mode === 'dialogue') props.draft = { ...props.draft, dialogue: { ...props.draft.dialogue, transform } }
      else props.draft = { ...props.draft, portrait: { ...props.draft.portrait, transform } }
    }
    const move = (x, y) => { preview.handlePointerMove(pointer(x, y)); const callback = frame; frame = null; callback() }
    preview.beginGesture(pointer(0, 0), 'drag', target, initial)
    move(100, 50)
    assert.equal(style().top, '20%', `${mode} moves immediately before any echo`)
    assert.equal(getTransform().y, 0.1, `${mode} preview never mutates props`)
    move(200, 100)
    preview.endGesture()
    assert.equal(style().top, '30%', `${mode} holds its final position on release`)
    echo(normalizeTheaterTransform(commands[0].transform, initial)); await Vue.nextTick()
    assert.equal(style().top, '30%', `${mode} ignores stale echoes after release`)
    echo(normalizeTheaterTransform(commands.at(-1).transform, initial)); await Vue.nextTick()
    assert.equal(style().top, '30%', `${mode} final echo takes over without jumping`)
    echo(initial); await Vue.nextTick()
    assert.equal(style().top, '10%', `${mode} undo works after the final echo`)
    assert.deepEqual(phases, ['gestureStart', 'gestureEnd'], `${mode} keeps one edit transaction`)

    // Returning to the starting point still needs a new echo before clearing the preview.
    preview.beginGesture(pointer(0, 0), 'drag', target, initial)
    move(0, 50)
    const outwardCommand = commands.at(-1)
    move(0, 0)
    preview.endGesture()
    echo(normalizeTheaterTransform(outwardCommand.transform, initial)); await Vue.nextTick()
    assert.equal(style().top, '10%', `${mode} returning to the start ignores an outstanding move`)
    echo(normalizeTheaterTransform(commands.at(-1).transform, initial)); await Vue.nextTick()

    // A fresh drag starts from the displayed position even before its previous echo.
    preview.beginGesture(pointer(0, 0), 'drag', target, initial)
    move(0, 50)
    preview.endGesture()
    preview.beginGesture(pointer(0, 0), 'drag', target, initial)
    move(0, 50)
    assert.equal(style().top, '30%', `${mode} repeated gestures continue from the visible position`)
    echo(normalizeTheaterTransform(commands.at(-2).transform, initial)); await Vue.nextTick()
    assert.equal(style().top, '30%', `${mode} ignores stale echoes during drag`)
    preview.endGesture()
    echo(normalizeTheaterTransform(commands.at(-1).transform, initial)); await Vue.nextTick()

    preview.beginGesture(pointer(0, 0), 'resize', target, getTransform())
    move(0, 50)
    assert.equal(style().height, '60%', `${mode} resize also previews immediately`)
    preview.endGesture()
    echo(normalizeTheaterTransform(commands.at(-1).transform, getTransform())); await Vue.nextTick()

    // Release flushes the queued frame, and local bounds match authoritative limits.
    preview.beginGesture(pointer(0, 0), 'drag', target, getTransform())
    preview.handlePointerMove(pointer(0, 2000))
    preview.endGesture()
    assert.equal(style().top, mode === 'controller' || mode === 'multiplayer' ? '40%' : '200%', `${mode} clamps the final queued move`)
    unmounted.splice(0).forEach(fn => fn())
    scope.stop()
  }
  console.log('theater presentation preview regression tests passed')
}
run().catch(error => { console.error(error); process.exitCode = 1 }).finally(() => {
  Module._load = originalLoad; Module._resolveFilename = originalResolve
  if (originalTsLoader) require.extensions['.ts'] = originalTsLoader; else delete require.extensions['.ts']
  for (const [key, value] of Object.entries({ window: originalWindow, requestAnimationFrame: originalRequestFrame, cancelAnimationFrame: originalCancelFrame })) {
    if (value === undefined) delete global[key]; else global[key] = value
  }
})
