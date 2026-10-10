// Existing local TypeScript/Vue/Node harness; no additional test dependencies.
// node scripts/theater-stage-surface-embeds-regression.cjs [--browser]
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const Module = require('node:module')
const ts = require('typescript')
const root = path.resolve(__dirname, '..')
const originalLoad = Module._load
const originalResolve = Module._resolveFilename
Module._load = function (request, ...args) {
  if (request === '@/stores/_config') return { api: {} }
  if (request === '@/stores/chat') return { chatEvent: { on() {}, off() {} } }
  return originalLoad.call(this, request, ...args)
}
Module._resolveFilename = function (request, parent, ...args) {
  if (request.startsWith('@/')) request = path.join(root, 'src', request.slice(2))
  return originalResolve.call(this, request, parent, ...args)
}
require.extensions['.ts'] = (module, filename) => module._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename, compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, esModuleInterop: true },
}).outputText, filename)
global.crypto ||= require('node:crypto').webcrypto

const { createTheaterStageStore } = require('../src/views/theater/stage/StageStore.ts')
const { normalizeStageSurfaceEmbeds } = require('../src/views/theater/shared/stage-types.ts')
const { theaterSyncTesting: sync } = require('../src/views/theater/sync/TheaterSyncClient.ts')
const { stageSceneReadResultSchema } = require('../src/views/theater/bridge/theater-bridge-protocol.ts')
const empty = { background: null, foreground: null }
assert.deepEqual(normalizeStageSurfaceEmbeds(undefined), empty)
assert.deepEqual(normalizeStageSurfaceEmbeds({ background: { type: 'image' }, foreground: [] }), empty)
assert.deepEqual(normalizeStageSurfaceEmbeds({ background: { type: 'iframe', iframe: { url: ' https://example.com ', scale: 99 } } }).background,
  { type: 'iframe', iframe: { url: 'https://example.com', scale: 5 }, interactive: false })
const store = createTheaterStageStore()
const legacy = store.getSnapshot()
delete legacy.liveState.surfaceEmbeds
Object.values(legacy.scenes).forEach(scene => delete scene.state.surfaceEmbeds)
store.replaceState(legacy)
assert.deepEqual(store.state.liveState.surfaceEmbeds, empty)
const before = sync.documentFromWorkspace(store.getSnapshot())
assert.equal(store.setSceneSurfaceEmbed('background', { type: 'iframe', iframe: { url: 'javascript:alert(1)', scale: 1 }, interactive: true }), false)
const config = { type: 'iframe', iframe: { url: 'https://example.com/page', scale: 0.75 }, interactive: true }
assert.equal(store.setSceneSurfaceEmbed('background', config), true)
assert.equal(store.setSceneSurfaceEmbed('foreground', { ...config, interactive: false }), true)
store.setSceneImage('background', 'https://example.com/background.png')
store.setSceneImage('foreground', 'https://example.com/foreground.png')
const document = sync.documentFromWorkspace(store.getSnapshot())
const mutations = sync.diffDocuments(before, document)
assert.ok(mutations.some(mutation => mutation.type === 'scene.update' && mutation.payload.fields.state.surfaceEmbeds))
assert.equal(mutations.some(mutation => mutation.type.startsWith('object.')), false, 'surfaces do not enter StageObject/Render Bands')
const workspace = sync.workspaceFromDocument(sync.normalizeDocument(JSON.parse(JSON.stringify(document))))
assert.deepEqual(workspace.liveState.surfaceEmbeds, store.getSnapshot().liveState.surfaceEmbeds)
assert.ok(workspace.liveState.background && workspace.liveState.foreground, 'images coexist with webpages')
const bridged = stageSceneReadResultSchema.parse({ ok: true, state: store.getSnapshot() })
assert.deepEqual(bridged.state.liveState.surfaceEmbeds, workspace.liveState.surfaceEmbeds)
const oldBridge = store.getSnapshot()
delete oldBridge.liveState.surfaceEmbeds
Object.values(oldBridge.scenes).forEach(scene => delete scene.state.surfaceEmbeds)
assert.deepEqual(stageSceneReadResultSchema.parse({ ok: true, state: oldBridge }).state.liveState.surfaceEmbeds, empty)
const invalidBridge = store.getSnapshot()
invalidBridge.liveState.surfaceEmbeds.background.sandbox = 'allow-top-navigation'
assert.equal(stageSceneReadResultSchema.safeParse({ ok: true, state: invalidBridge }).success, false)
const sceneId = store.state.activeSceneId
const otherSceneId = store.scenes.value.find(scene => scene.id !== sceneId).id
store.selectScene(otherSceneId)
assert.deepEqual(store.state.liveState.surfaceEmbeds, empty)
store.selectScene(sceneId)
assert.deepEqual(store.getSnapshot().liveState.surfaceEmbeds, workspace.liveState.surfaceEmbeds)
const reentered = createTheaterStageStore()
reentered.replaceState(workspace)
assert.deepEqual(reentered.getSnapshot().liveState.surfaceEmbeds, workspace.liveState.surfaceEmbeds)
const url = reentered.state.liveState.surfaceEmbeds.background.iframe.url
assert.equal(reentered.patchSceneSurfaceEmbed('background', { interactive: false }), true)
assert.equal(reentered.state.liveState.surfaceEmbeds.background.iframe.url, url)
const object = reentered.addObject('iframe')
assert.equal(reentered.undo(), true)
assert.equal(reentered.activeObjects.value[object.id], undefined)
assert.equal(reentered.undo(), true)
assert.equal(reentered.state.liveState.surfaceEmbeds.background.interactive, true)
assert.equal(reentered.removeSceneSurfaceEmbed('foreground'), true)
assert.equal(reentered.state.liveState.surfaceEmbeds.foreground, null)
assert.equal(reentered.undo(), true)
assert.ok(reentered.state.liveState.surfaceEmbeds.foreground)
assert.equal(reentered.patchSceneSurfaceEmbed('foreground', { iframe: { scale: 100 } }), true)
assert.equal(reentered.state.liveState.surfaceEmbeds.foreground.iframe.scale, 5)
// Retain the existing ordinary iframe object/sync/store regression coverage.
require('./theater-stage-store-runtime.spec.ts')
console.log('theater surface embed state/store/sync/strict Bridge regression checks passed')

if (process.argv.includes('--browser')) {
  runBrowser().catch(error => { console.error(error); process.exitCode = 1 })
}

async function runBrowser() {
  const http = require('node:http')
  const { spawn } = require('node:child_process')
  const { build } = require('esbuild')
  const { parse, compileScript, compileStyle } = require('@vue/compiler-sfc')
  const browser = process.env.THEATER_TEST_BROWSER || (process.platform === 'win32'
    ? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe' : '/usr/bin/chromium')
  if (!fs.existsSync(browser)) throw new Error(`Browser unavailable: ${browser}`)
  const { outputFiles } = await build({
    entryPoints: [path.join(__dirname, 'theater-stage-surface-embeds-browser.ts')],
    bundle: true, write: false, platform: 'browser', format: 'iife',
    define: { 'process.env.NODE_ENV': '"production"', __VUE_OPTIONS_API__: 'true', __VUE_PROD_DEVTOOLS__: 'false', __VUE_PROD_HYDRATION_MISMATCH_DETAILS__: 'false' },
    plugins: [{ name: 'surface-test-sfc', setup(builder) {
      builder.onResolve({ filter: /^@\/stores\/(chat|iform|utils)$/ }, args => ({ path: args.path, namespace: 'test-stores' }))
      builder.onResolve({ filter: /^@\/components\/iform\/IFormEmbedFrame.vue$/ }, () => ({ path: 'iform', namespace: 'test-iform' }))
      builder.onResolve({ filter: /^@\/components\/rich-text\/RichTextContent.vue$/ }, () => ({ path: 'rich-text', namespace: 'test-text' }))
      builder.onResolve({ filter: /^@\// }, args => ({ path: path.join(root, 'src', args.path.slice(2)) + (path.extname(args.path) ? '' : '.ts') }))
      builder.onLoad({ filter: /.*/, namespace: 'test-stores' }, args => ({ contents: args.path.endsWith('/chat')
        ? 'import {reactive} from "vue"; export const chat=reactive({currentWorldId:"world",curChannel:{id:"channel"}}); export const useChatStore=()=>chat'
        : args.path.endsWith('/iform')
          ? 'import {reactive} from "vue"; const store=reactive({formsByChannel:{channel:[{id:"form",url:location.origin+"/page"}]},bootstrap(){},hasLoadedForms(){return true},async ensureForms(){}}); export const useIFormStore=()=>store'
          : 'export const useUtilsStore=()=>({config:{}})', loader: 'js', resolveDir: root }))
      builder.onLoad({ filter: /.*/, namespace: 'test-iform' }, () => ({ contents: 'import {defineComponent,h,onBeforeUnmount} from "vue"; export default defineComponent({props:["form"],setup(props){window.iformMounts=(window.iformMounts||0)+1;onBeforeUnmount(()=>window.iformStops=(window.iformStops||0)+1);return()=>h("div",{class:"iform-frame"},h("iframe",{class:"iform-frame__iframe",src:props.form.url}))}})', loader: 'js', resolveDir: root }))
      builder.onLoad({ filter: /.*/, namespace: 'test-text' }, () => ({ contents: 'export default {render(){return null}}', loader: 'js', resolveDir: root }))
      builder.onLoad({ filter: /\.vue$/ }, args => {
        const { descriptor } = parse(fs.readFileSync(args.path, 'utf8'), { filename: args.path })
        const id = 'test-' + path.basename(args.path).replace(/\W/g, '')
        const script = compileScript(descriptor, { id, inlineTemplate: true, genDefaultAs: '__sfc__' })
        const css = descriptor.styles.map(style => compileStyle({ source: style.content, filename: args.path, id: `data-v-${id}`, scoped: style.scoped }).code).join('\n')
        return { contents: script.content + `\n__sfc__.__scopeId=${JSON.stringify(`data-v-${id}`)};const style=document.createElement('style');style.textContent=${JSON.stringify(css)};document.head.append(style);export default __sfc__;`, loader: 'ts', resolveDir: path.dirname(args.path) }
      })
    }}],
  })
  const server = http.createServer((request, response) => {
    response.setHeader('Content-Type', 'text/html')
    response.end(request.url === '/page' ? '<button onclick="window.clicks=(window.clicks||0)+1">click</button><canvas></canvas><script>window.pageReady=true</script>'
      : '<div id="app"></div><pre id="result">pending</pre><script>window.onerror=(message)=>document.querySelector("#result").textContent=String(message);</script><script>' + outputFiles[0].text.replace(/<\/script/gi, '<\\/script') + '</script>')
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  const temp = fs.mkdtempSync(path.join(require('node:os').tmpdir(), 'theater-surface-browser-'))
  try {
    const child = spawn(browser, ['--headless', '--disable-gpu', '--no-sandbox', '--no-first-run', `--user-data-dir=${temp}`, '--dump-dom', '--virtual-time-budget=5000', `http://127.0.0.1:${server.address().port}/`])
    let output = '', errors = ''
    child.stdout.on('data', data => output += data)
    child.stderr.on('data', data => errors += data)
    const code = await new Promise(resolve => child.on('close', resolve))
    const report = output.match(/<pre id="result">([\s\S]*?)<\/pre>/)?.[1]
    assert.equal(code, 0, errors.slice(-1000))
    assert.equal(report, 'PASS: surface DOM layering, pointer ownership, camera, iframe identity, local test state, IForm context and cleanup', report || output.slice(-2000))
    console.log(report)
  } finally {
    server.close()
    // Retain the browser profile in OS temp; do not delete user files.
  }
}
