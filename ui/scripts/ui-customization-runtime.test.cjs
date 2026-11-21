// Run with: node scripts/ui-customization-runtime.test.cjs
// Real resolver/runtime, a small DOM fixture, and deferred HTTP responses.
const fs = require('node:fs')
const path = require('node:path')
const assert = require('node:assert/strict')
const ts = require('typescript')
require.extensions['.ts'] = (module, filename) => module._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText, filename)
const root = path.join(__dirname, '../src/services/ui-customization')
const { resolveCustomizationContext, hasSafeUIParameter } = require(path.join(root, 'resolver.ts'))
const { createUICustomizationRuntime } = require(path.join(root, 'cssRuntime.ts'))
const { detectChatEmbedHost, resolveEffectiveCustomizationSurface } = require(path.join(root, 'embedSurface.ts'))

// The shared URL protocol needs no new option or query parameter.
const { buildEmbedChatWindowUrl } = require(path.join(root, '../../views/theater/host/theater-floating-window.ts'))
global.window = { location: { href: 'https://chat.example.com/app/?x=1#/w/world-1/ch-1' } }
const embedURL = 'https://chat.example.com/app/?x=1#/embed?worldId=world-1&channelId=ch-2&scopeWorldId=world-1&viewport=mobile&audioOwner=0&toolbar=1'
assert.equal(buildEmbedChatWindowUrl('world-1', 'ch-2'), `${embedURL}&floatingChat=1`)
assert.equal(buildEmbedChatWindowUrl('world-1', 'ch-2', false), embedURL)
assert.equal(new URLSearchParams(new URL(buildEmbedChatWindowUrl('world-1', 'ch-2')).hash.split('?')[1]).has('workbenchChat'), false)
delete global.window

function workbenchFrame({ panelType = 'chat', marker = 'root', framed = true } = {}) {
  const parent = {
    location: { search: '', hash: '' },
    document: {
      documentElement: { dataset: {} },
      querySelector(selector) {
        assert.equal(selector, '[data-sc-ui="workbench"], html[data-sc-workbench]')
        return marker === 'root' || marker === 'html' ? {} : null
      },
    },
  }
  // A parent-realm element must work without the child's HTMLIFrameElement class.
  const frameElement = require('node:vm').runInNewContext('({})')
  frameElement.closest = selector => {
    assert.equal(selector, '[data-sc-ui="workbench-panel"][data-sc-panel-type="chat"]')
    return panelType === 'chat' ? {} : null
  }
  frameElement.matches = () => false
  return { parent, frameElement: framed ? frameElement : null }
}

const chatHosts = [
  ['inline-split', 'views/chat/components/InlineChatSplitWindow.vue', ['inline-chat-split', 'inline-chat-split__iframe']],
  ['split-A', 'views/split/SplitView.vue', ['sc-split-panes', 'sc-split-iframe is-chat']],
  ['split-B', 'views/split/SplitView.vue', ['sc-split-panes', 'sc-split-iframe is-chat']],
  ['theater-chat', 'views/theater/host/TheaterView.vue', ['theater-host', 'theater-host-chat', 'theater-host-chat-frame']],
  ...['stage', 'viewport', 'workbench'].map(mode => [`floating-${mode}`, 'views/theater/host/TheaterFloatingHost.vue',
    [`theater-floating-host theater-floating-host--${mode}`, 'theater-floating-window is-chat', 'theater-floating-window__frame']]),
]

function builtInFrame(classes) {
  const fixture = workbenchFrame({ panelType: '', marker: '' })
  const elements = classes.map(value => new Set(value.split(' ')))
  // Minimal class/descendant selector fixture, with the frame in the parent realm.
  fixture.frameElement.matches = selectors => selectors.split(',').some(selector => {
    const terms = selector.trim().split(/\s+/).map(term => term.slice(1).split('.')).reverse()
    let index = elements.length - 1
    const match = term => term.every(name => elements[index].has(name))
    if (!match(terms.shift())) return false
    for (const term of terms) {
      do { index-- } while (index >= 0 && !match(term))
      if (index < 0) return false
    }
    return true
  })
  return fixture
}

// Keep the runtime's DOM allowlist tied to the real host templates.
for (const [name, file, classes] of chatHosts) {
  const source = fs.readFileSync(path.join(root, '../..', file), 'utf8')
  for (const classNames of classes) {
    for (const className of classNames.split(' ')) {
      if (className === 'is-chat' || className.startsWith('theater-floating-host--')) continue
      assert.ok(source.includes(`class="${className}"`), `${name}: host class ${className} changed`)
    }
  }
  assert.equal(detectChatEmbedHost('embed', builtInFrame(classes)), true, name)
  assert.equal(detectChatEmbedHost('theater', builtInFrame(classes)), false, `${name}: non-embed was promoted`)
  assert.equal(detectChatEmbedHost('embed', builtInFrame(classes.slice(-1))), false, `${name}: missing host was accepted`)
  const crossOrigin = builtInFrame(classes)
  Object.defineProperty(crossOrigin.parent, 'document', { get() { throw new DOMException('Cross-origin', 'SecurityError') } })
  assert.equal(detectChatEmbedHost('embed', crossOrigin), false, `${name}: cross-origin was accepted`)
}
const floatingTemplate = fs.readFileSync(path.join(root, '../../views/theater/host/TheaterFloatingHost.vue'), 'utf8')
assert.match(floatingTemplate, /'is-chat': item\.source === 'chat'/)
const splitTemplate = fs.readFileSync(path.join(root, '../../views/split/SplitView.vue'), 'utf8')
for (const pane of ['A', 'B']) assert.ok(splitTemplate.includes(`:class="{ 'is-chat': pane${pane}.mode === 'chat' }"`))
for (const classes of [
  ['theater-floating-host', 'theater-floating-window', 'theater-floating-window__frame'],
  ['theater-floating-host', 'theater-floating-window is-character', 'theater-floating-window__frame'],
  ['theater-iframe-frame', 'theater-iframe-visual-object__frame'],
  ['sc-split-panes', 'sc-split-iframe'],
  ['sc-split-panes', 'arbitrary-web-frame'],
]) assert.equal(detectChatEmbedHost('embed', builtInFrame(classes)), false, classes.join(' > '))

for (const [surface, workbench, expected] of [
  ['chat', false, 'chat'], ['embed', false, 'embed'], ['embed', true, 'chat'],
  ['theater', true, 'theater'], ['split', true, 'split'], ['internal', true, 'internal'],
]) assert.equal(resolveEffectiveCustomizationSurface(surface, workbench), expected)
assert.equal(detectChatEmbedHost('embed', undefined), false, 'SSR is not a Workbench')
const topWindow = {}; topWindow.parent = topWindow
assert.equal(detectChatEmbedHost('embed', topWindow), false, 'direct embed was promoted')
assert.equal(detectChatEmbedHost('embed', workbenchFrame()), true)
assert.equal(detectChatEmbedHost('embed', workbenchFrame({ marker: 'html' })), true)
assert.equal(detectChatEmbedHost('split', workbenchFrame()), false)
assert.equal(detectChatEmbedHost('embed', workbenchFrame({ framed: false })), false)
assert.equal(detectChatEmbedHost('embed', workbenchFrame({ marker: '' })), false, 'inactive Workbench was promoted')
for (const panelType of ['', 'web', 'theater', 'split', 'viewport', 'stage']) {
  assert.equal(detectChatEmbedHost('embed', workbenchFrame({ panelType })), false)
}
const crossOrigin = workbenchFrame()
Object.defineProperty(crossOrigin.parent, 'document', { get() { throw new DOMException('Cross-origin', 'SecurityError') } })
assert.equal(detectChatEmbedHost('embed', crossOrigin), false)

const chat = { currentWorldId: 'A', channelId: 'a-channel', channelWorldId: 'A', observerMode: false, observerWorldId: '', observerChannelId: '', observerSlug: '' }
const route = (name, params = {}, query = {}, meta = {}) => ({ name, params, query, meta })
const context = (name, params, query, state = chat) => resolveCustomizationContext(route(name, params, query), state)
assert.equal(context('world-detail', { worldId: 'B' }).worldId, 'B')
assert.equal(context('world-private-hint', { worldId: 'B' }).surface, 'world-detail')
assert.equal(context('world-private-hint', { worldId: 'B' }).worldId, '', 'private hint used route world as authorization')
assert.equal(context('home').surface, 'chat')
assert.equal(context('home').worldId, 'A')
assert.equal(context('home').channelId, 'a-channel')
assert.equal(context('world-channel', { worldId: 'B' }).worldId, 'B')
const observerChannel = context('world-channel', { worldId: 'B', channelId: 'ob-channel' }, {}, { ...chat, observerMode: true, observerWorldId: 'B', observerChannelId: 'ob-channel', observerSlug: 'ob-private' })
assert.equal(observerChannel.surface, 'observer')
assert.equal(observerChannel.slug, 'ob-private', 'observer redirect lost its public slug')
assert.equal(observerChannel.channelId, 'ob-channel')
assert.equal(context('world-channel', { worldId: 'B' }).channelId, '')
assert.equal(context('world-channel', { worldId: 'B', channelId: 'b-channel' }).channelId, 'b-channel')
for (const name of ['world-lobby', 'about', 'user-signin', 'user-signup', 'password-recovery', 'user-password-reset', 'status']) {
  assert.equal(context(name).worldId, '', `${name} inherited chat world`)
}
assert.equal(context('world-lobby').surface, 'lobby')
assert.equal(context('user-signin').surface, 'auth')
assert.equal(context('split', {}, { scopeWorldId: 'B', worldId: 'A' }).worldId, 'B')
assert.equal(context('split', {}, { scopeWorldId: 'B::preset:ic-ooc', worldId: 'B' }).worldId, 'B')
assert.equal(context('embed', {}, { worldId: 'B', channelId: 'b-channel' }).worldId, 'B')
assert.equal(context('embed').surface, 'embed')
assert.equal(context('embed').publicSurface, true)
assert.equal(context('embed', {}, { worldId: 'B' }, { ...chat, embedReady: true }).worldId, 'A')
assert.equal(context('theater', {}, { worldId: 'B' }).worldId, 'B')
assert.equal(context('observer-entry', { slug: 'ob' }).worldId, '')
const observer = { ...chat, observerMode: true, observerWorldId: 'B', observerChannelId: 'b-channel', observerSlug: 'ob' }
assert.equal(context('observer-entry', { slug: 'ob' }, {}, observer).worldId, 'B')
assert.equal(context('observer-entry', { slug: 'new-ob' }, {}, observer).worldId, '')
assert.equal(context('invite-consume', { slug: 'invite' }).worldId, '')
assert.equal(context('invite-consume', { slug: 'invite' }, {}, { ...chat, invite: { slug: 'invite', worldId: 'B' } }).worldId, 'B')
assert.equal(context('log-public', { slug: 'public' }).surface, 'log-public')
assert.equal(context('log-public', { slug: 'public' }).worldId, '')
assert.equal(context('log-layout', { slug: 'layout' }).worldId, '')
assert.equal(resolveCustomizationContext(route('internal-surface', {}, {}, { internalSurface: true }), chat).surface, 'internal')
assert.equal(hasSafeUIParameter({ search: '?safe-ui=1', hash: '#/A' }), true)
assert.equal(hasSafeUIParameter({ search: '', hash: '#/A?safe-ui=1' }), true)
assert.equal(hasSafeUIParameter({ search: '?safe-ui=0', hash: '#/A' }), false)

function domFixture() {
  const observers = new Set()
  const changed = () => queueMicrotask(() => { for (const observer of observers) observer.callback() })
  const head = {
    children: [],
    appendChild(node) {
      node.remove()
      this.children.push(node)
      node.parent = this
      changed()
      return node
    },
  }
  return {
    documentElement: { dataset: {} },
    head, defaultView: { MutationObserver: class {
      constructor(callback) { this.callback = callback }
      observe() { observers.add(this) }
      disconnect() { observers.delete(this) }
    } },
    createElement(tagName) {
      return {
        tagName, dataset: {}, writes: 0, text: '', parent: null,
        get textContent() { return this.text },
        set textContent(value) { this.writes++; this.text = value },
        remove() {
          if (!this.parent) return
          this.parent.children.splice(this.parent.children.indexOf(this), 1)
          this.parent = null
          changed()
        },
      }
    },
  }
}
const pkg = (id, scopeType, sortOrder = 0, css = id) => ({ id, scopeType, sortOrder, css, createdAt: 1, name: id })
const deferred = () => {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
async function run() {
  const doc = domFixture()
  const theme = doc.createElement('style'); theme.textContent = 'existing theme'; doc.head.appendChild(theme)
  let packages = [pkg('world', 'world'), pkg('late', 'platform', 5), pkg('first', 'platform', 0)]
  let requests = 0
  let pending
  const runtime = createUICustomizationRuntime({ document: doc, loadPackages: async ctx => {
    requests++
    return pending ? pending.promise : { ...ctx, packages }
  } })
  const ids = () => doc.head.children.filter(n => n.dataset.scCustomizationId).map(n => n.dataset.scCustomizationId)
  const a = context('world-channel', { worldId: 'A' })
  await runtime.reconcile(a, false)
  assert.deepEqual(ids(), ['first', 'late', 'world'])
  assert.equal(doc.documentElement.dataset.scUiCustomized, 'true')
  assert.equal(doc.head.children[0], theme)
  const nodes = [...doc.head.children]
  const first = nodes[1], world = nodes[3]
  packages = [pkg('world', 'world', 0, 'updated'), pkg('late', 'platform', 5), pkg('first', 'platform', 0)]
  await runtime.reconcile(a, false)
  assert.equal(doc.head.children[1], first)
  assert.equal(doc.head.children[3], world)
  assert.equal(first.writes, 1, 'unrelated package rewritten')
  assert.equal(world.writes, 2)
  const lazyTheme = doc.createElement('style'); lazyTheme.textContent = 'lazy base style'
  doc.head.appendChild(lazyTheme); await Promise.resolve()
  assert.deepEqual(doc.head.children.slice(0, 2), [theme, lazyTheme], 'late base styles escaped above customization')
  assert.deepEqual(ids(), ['first', 'late', 'world'])

  pending = deferred()
  const switchWorld = runtime.reconcile(context('world-channel', { worldId: 'B' }), false)
  assert.deepEqual(ids(), ['first', 'late'], 'old world must unload before the request finishes')
  pending.resolve({ packages: [pkg('world-b', 'world'), pkg('first', 'platform')] })
  await switchWorld
  assert.deepEqual(ids(), ['first', 'world-b'])
  const stale = deferred(); pending = stale
  const oldRequest = runtime.reconcile(a, false)
  pending = undefined
  packages = [pkg('lobby', 'platform')]
  await runtime.reconcile(context('world-lobby'), false)
  assert.deepEqual(ids(), ['lobby'])
  stale.resolve({ packages: [pkg('stale-world', 'world')] }); await oldRequest
  assert.deepEqual(ids(), ['lobby'], 'stale response restored old styles')
  const beforeSafe = requests
  await runtime.reconcile(a, true)
  assert.deepEqual(ids(), [])
  assert.equal(doc.documentElement.dataset.scUiCustomized, undefined, 'safe mode left overlay theme active')
  assert.equal(requests, beforeSafe, 'safe mode performed a package request')
  await runtime.reconcile(a, false)
  assert.equal(doc.documentElement.dataset.scUiCustomized, 'true')
  const beforeInternal = requests
  await runtime.reconcile({ ...a, surface: 'internal' }, false)
  assert.equal(requests, beforeInternal, 'internal performed a package request')
  assert.equal(doc.documentElement.dataset.scUiCustomized, undefined, 'internal surface left overlay theme active')

  packages = [pkg('empty', 'platform', 0, '')]
  await runtime.reconcile(a, false)
  assert.deepEqual(ids(), [])
  assert.equal(doc.documentElement.dataset.scUiCustomized, undefined, 'script-only package activated CSS baseline')
  packages = [pkg('restored', 'world')]
  await runtime.reconcile(a, false)
  pending = deferred()
  const failed = runtime.reconcile(a, false)
  pending.reject(new Error('denied')); await failed
  assert.deepEqual(ids(), [], 'failed authorization left styles mounted')
  assert.equal(doc.documentElement.dataset.scUiCustomized, undefined)
  assert.equal(theme.textContent, 'existing theme', 'existing theme changed')
  assert.equal(doc.head.children[0], theme)
  pending = deferred()
  const afterDispose = runtime.reconcile(a, false)
  runtime.dispose()
  pending.resolve({ packages: [pkg('after-dispose', 'platform')] }); await afterDispose
  assert.deepEqual(ids(), [], 'disposed runtime accepted a pending response')
  assert.equal(doc.documentElement.dataset.scUiCustomized, undefined)
  await testInstaller()
  console.log('UI customization resolver and CSS lifecycle tests passed')
}

async function testInstaller() {
  const { reactive, ref, nextTick } = require('vue')
  const Module = require('node:module')
  const state = reactive({ ...chat, curChannel: { id: 'a-channel' }, lastConsumedWorldInvite: null })
  const user = reactive({ info: { id: 'user' }, token: 'token' })
  const display = reactive({ palette: 'night', layout: 'compact' })
  const utils = reactive({ config: null })
  const requests = []
  const scripts = []
  const clients = []
  let cleanups = 0
  let chatPackages = false
  const realJSRuntime = require(path.join(root, 'jsRuntime.ts'))
  const chatEvent = { all: new Map(), on(n, h) { this.all.set(n, h) }, off(n) { this.all.delete(n) } }
  const stubs = {
    '@/stores/chat': { useChatStore: () => state, chatEvent },
    'naive-ui': { createDiscreteApi: () => ({ message: {} }), darkTheme: {} },
    '@/composables/useAttachmentResolver': { resolveAttachmentUrl: () => '' },
    '@/bridge/sealchatBridgeSerializer': { buildBridgeMessagePayload: input => input },
    '@/stores/user': { useUserStore: () => user },
    '@/stores/display': { useDisplayStore: () => display },
    '@/stores/utils': { useUtilsStore: () => utils },
    '@/stores/characterCard': { useCharacterCardStore: () => ({}) },
    '@/stores/channelCharacterSnapshot': { useChannelCharacterSnapshotStore: () => ({}) },
    '@/bridge/channelExtensionClient': { createChannelExtensionClient: options => {
      assert.equal(chatPackages, true, 'CSS-only fixture must not open a channel client')
      const client = { options, closed: false, close() { this.closed = true }, onClosed() {} }
      clients.push(client)
      return client
    } },
    './jsRuntime': { ...realJSRuntime, createBlobModuleImporter: () => async source => {
      assert.match(source, /workbench-script/)
      return { default(sc) { scripts.push(sc); sc.cleanup(() => { cleanups++ }) } }
    } },
    './api': { loadUICustomizations: async ctx => {
      requests.push(ctx)
      if (chatPackages) return { ...ctx, packages: ctx.surface === 'chat' ? [
        { ...pkg('chat-platform', 'platform'), js: '// workbench-script', jsHash: 'platform-js', channelCapabilities: ['context.read'] },
        { ...pkg('chat-world', 'world'), js: '// workbench-script', jsHash: 'world-js', requiresTrust: true, channelCapabilities: ['context.read'] },
      ] : [] }
      const worldId = ctx.surface === 'log-layout' ? 'LOG-WORLD' : ctx.worldId
      const world = worldId ? pkg(worldId, 'world') : null
      if (ctx.surface === 'log-layout' && world) Object.assign(world, { js: 'void 0', jsHash: 'log-js', requiresTrust: true })
      return { ...ctx, worldId, packages: [pkg(`platform-${ctx.surface}`, 'platform'), ...(world ? [world] : [])] }
    } },
  }
  const original = Module._resolveFilename
  Module._resolveFilename = function(request, parent, ...args) {
    if (request in stubs) return `customization-stub:${request}`
    return original.call(this, request, parent, ...args)
  }
  for (const [key, exports] of Object.entries(stubs)) Module._cache[`customization-stub:${key}`] = { exports, loaded: true }
  const { installUICustomizationRuntime, isUICustomizationSafeSession, setUICustomizationSafeSession, UI_CUSTOMIZATION_SAFE_SESSION_KEY, useUICustomizationTrustPrompt } = require(path.join(root, 'runtime.ts'))
  const local = new Map()
  const boot = async (location, parentSafe = false, frame = null) => {
    const doc = domFixture(); doc.documentElement = { dataset: {} }
    const listeners = new Map()
    const session = new Map()
    const win = {
      location, document: doc,
      localStorage: { getItem: key => local.get(key) ?? null, setItem: (key, value) => local.set(key, value) },
      sessionStorage: { getItem: key => session.has(key) ? session.get(key) : null, setItem: (key, value) => session.set(key, String(value)) },
      addEventListener: (name, fn) => listeners.set(name, fn), removeEventListener: name => listeners.delete(name),
      dispatchEvent: event => { listeners.get(event.type)?.(event); return true },
    }
    win.parent = parentSafe ? { location: { search: '', hash: '' }, document: { documentElement: { dataset: { scSafeUi: 'true' } } } } : win
    if (frame) { win.parent = frame.parent; win.frameElement = frame.frameElement }
    global.window = win; global.document = doc
    const initial = deferred()
    let onNavigation
    const router = {
      currentRoute: ref({ ...route('world-channel', { worldId: 'B' }), fullPath: '/B' }),
      isReady: () => initial.promise,
      afterEach: fn => { onNavigation = fn; return () => { onNavigation = null } },
    }
    const runtime = installUICustomizationRuntime({ pinia: {}, router })
    assert.equal(doc.head.children.length, 0, 'installed before router readiness')
    return { doc, win, router, runtime, initial, navigate: name => {
      router.currentRoute.value = { ...route(name), fullPath: `/${name}` }
      onNavigation?.({}, {}, undefined)
    } }
  }
  const tick = async () => { await nextTick(); await Promise.resolve(); await Promise.resolve() }
  let test = await boot({ search: '?safe-ui=1', hash: '#/B' })
  test.win.location.search = '' // A guard drops the URL parameter before ready.
  test.initial.resolve(); await tick()
  assert.equal(requests.length, 0, 'safe boot requested packages')
  assert.equal(test.doc.head.children.length, 0)
  test.navigate('world-lobby'); await tick()
  assert.equal(requests.length, 0, 'navigation escaped safe boot')
  test.runtime.dispose()

  test = await boot({ search: '', hash: '#/B' }, true)
  test.initial.resolve(); await tick()
  assert.equal(requests.length, 0, 'iframe ignored parent safe mode after its URL changed')
  test.runtime.dispose()

  test = await boot({ search: '', hash: '#/B' })
  test.initial.resolve(); await tick()
  assert.equal(requests.at(-1).worldId, 'B')
  const count = requests.length
  const nodes = [...test.doc.head.children]
  display.palette = 'day'; display.layout = 'bubble'; await tick()
  assert.equal(test.doc.documentElement.dataset.scPalette, 'day')
  assert.equal(test.doc.documentElement.dataset.scLayout, 'bubble')
  assert.equal(requests.length, count, 'theme switch refetched packages')
  assert.deepEqual(test.doc.head.children, nodes, 'theme switch replaced CSS nodes')
  const beforeSessionSafe = requests.length
  setUICustomizationSafeSession(true); await tick()
  assert.equal(isUICustomizationSafeSession(), true)
  assert.equal(test.doc.head.children.length, 0, 'session safe mode left customization mounted')
  assert.equal(requests.length, beforeSessionSafe, 'session safe mode requested packages')
  setUICustomizationSafeSession(false); await tick()
  assert.equal(isUICustomizationSafeSession(), false)
  assert.equal(requests.length, beforeSessionSafe + 1, 'leaving session safe mode did not refresh packages')
  assert.ok(test.doc.head.children.length > 0, 'leaving session safe mode did not restore customization')
  // Same-origin iframe documents receive sessionStorage changes through a
  // storage event rather than this window's custom refresh event.
  const beforeStorageSafe = requests.length
  test.win.sessionStorage.setItem(UI_CUSTOMIZATION_SAFE_SESSION_KEY, '1')
  test.win.dispatchEvent({ type: 'storage', key: UI_CUSTOMIZATION_SAFE_SESSION_KEY }); await tick()
  assert.equal(test.doc.head.children.length, 0, 'storage event did not disable iframe customization')
  assert.equal(requests.length, beforeStorageSafe, 'storage safe mode requested packages')
  test.win.sessionStorage.setItem(UI_CUSTOMIZATION_SAFE_SESSION_KEY, '0')
  test.win.dispatchEvent({ type: 'storage', key: UI_CUSTOMIZATION_SAFE_SESSION_KEY }); await tick()
  assert.equal(requests.length, beforeStorageSafe + 1, 'storage event did not restore iframe customization')
  // LOG layout gets its world only after the server resolves the layout slug.
  // Trust metadata must use that authoritative world rather than the empty route context.
  test.router.currentRoute.value = { ...route('log-layout', { slug: 'layout' }), fullPath: '/log/layout/layout' }
  await tick()
  assert.equal(requests.at(-1).surface, 'log-layout')
  assert.equal(requests.at(-1).worldId, '')
  assert.equal(useUICustomizationTrustPrompt().pending.value.at(-1)?.worldId, 'LOG-WORLD')
  test.router.currentRoute.value = { ...route('internal-surface', {}, {}, { internalSurface: true }), fullPath: '/internal/a/b' }
  await tick()
  assert.equal(test.doc.head.children.length, 0)
  assert.equal(requests.length, beforeStorageSafe + 2, 'internal surface performed a package request')
  test.runtime.dispose()
  assert.equal(chatEvent.all.size, 0, 'message listeners leaked after dispose')

  // Exercise the actual installer -> CSS runtime -> trust controller -> JS runtime.
  chatPackages = true
  const frames = [
    ['workbench-panel', () => workbenchFrame()],
    ...chatHosts.map(([name, , classes]) => [name, () => builtInFrame(classes)]),
  ]
  for (const [name, makeFrame] of frames) {
    scripts.length = 0; clients.length = 0; cleanups = 0; local.clear()
    const worldId = `world-${name}`, channelId = `channel-${name}`
    state.currentWorldId = worldId; state.curChannel = { id: channelId }
    const frame = makeFrame()
    frame.parent.channelId = 'host-channel' // Must never supply the child's context.
    test = await boot({ search: '', hash: '#/embed' }, false, frame)
    test.router.currentRoute.value = { ...route('embed', {}, { worldId, channelId: 'initial-channel' }), fullPath: '/embed' }
    test.initial.resolve(); await tick()
    await new Promise(resolve => setImmediate(resolve))
    assert.equal(requests.at(-1).surface, 'chat', name)
    assert.equal(requests.at(-1).publicSurface, false)
    assert.equal(requests.at(-1).worldId, worldId)
    assert.equal(requests.at(-1).channelId, channelId, `${name}: inherited the host/initial channel`)
    assert.equal(test.doc.documentElement.dataset.scSurface, 'chat')
    assert.deepEqual(test.doc.head.children.map(n => n.dataset.scCustomizationId), ['chat-platform', 'chat-world'])
    assert.equal(scripts.length, 1, `${name}: untrusted world script ran`)
    assert.equal(useUICustomizationTrustPrompt().pending.value[0].worldId, worldId)
    await useUICustomizationTrustPrompt().trust(['chat-world'])
    assert.equal(scripts.length, 2, `${name}: trusted chat script did not run`)
    for (const sc of scripts) {
      assert.equal(sc.context().surface, 'chat')
      assert.equal(sc.context().worldId, worldId)
      assert.equal(sc.context().channelId, channelId)
      const { options } = sc.channel()
      assert.equal(options.worldId, worldId)
      assert.equal(options.channelId, channelId)
      assert.deepEqual(options.capabilities, ['context.read'])
      assert.equal(options.isCurrent(), true)
    }
    const beforeSafe = requests.length
    setUICustomizationSafeSession(true); await tick()
    await new Promise(resolve => setImmediate(resolve))
    assert.equal(requests.length, beforeSafe)
    assert.equal(test.doc.head.children.length, 0)
    assert.equal(cleanups, 2, `${name}: Safe UI did not clean up chat scripts`)
    assert.ok(clients.every(c => c.closed && !c.options.isCurrent()))
    assert.ok(scripts.every(sc => sc.channel() === null))
    await test.runtime.refresh(); await tick()
    assert.equal(test.doc.head.children.length, 0, `${name}: detection restored CSS in Safe UI`)
    assert.equal(scripts.length, 2, `${name}: detection restored JS in Safe UI`)
    test.runtime.dispose()

    for (const safe of ['iframe-url', 'parent-url', 'parent-root']) {
      const safeFrame = makeFrame()
      if (safe === 'parent-url') safeFrame.parent.location.hash = '#/w/layout?safe-ui=1'
      if (safe === 'parent-root') safeFrame.parent.document.documentElement.dataset.scSafeUi = 'true'
      test = await boot({ search: safe === 'iframe-url' ? '?safe-ui=1' : '', hash: '#/embed' }, false, safeFrame)
      test.router.currentRoute.value = { ...route('embed'), fullPath: '/embed' }
      const count = requests.length
      test.initial.resolve(); await tick()
      assert.equal(test.doc.documentElement.dataset.scSurface, 'chat')
      assert.equal(requests.length, count, `${name}: ${safe} requested packages`)
      assert.equal(test.doc.head.children.length, 0)
      assert.equal(scripts.length, 2)
      test.runtime.dispose()
    }
  }
  for (const frame of [null, workbenchFrame({ panelType: '' }), workbenchFrame({ panelType: 'theater' })]) {
    test = await boot({ search: '', hash: '#/embed' }, false, frame)
    test.router.currentRoute.value = { ...route('embed'), fullPath: '/embed' }
    test.initial.resolve(); await tick()
    assert.equal(requests.at(-1).surface, 'embed')
    assert.equal(requests.at(-1).publicSurface, true)
    assert.equal(test.doc.documentElement.dataset.scSurface, 'embed')
    assert.equal(scripts.length, 2)
    test.runtime.dispose()
  }
  delete global.window; delete global.document
  Module._resolveFilename = original
}
run().catch(error => { console.error(error); process.exitCode = 1 })
