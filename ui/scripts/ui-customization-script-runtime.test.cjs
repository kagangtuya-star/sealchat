// Run with: node scripts/ui-customization-script-runtime.test.cjs
// Real JS lifecycle, trust store, script controller and CSS runtime wiring.
const fs = require('node:fs')
const path = require('node:path')
const assert = require('node:assert/strict')
const ts = require('typescript')
require.extensions['.ts'] = (module, filename) => module._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText, filename)
const root = path.join(__dirname, '../src/services/ui-customization')
const { createScriptRuntime, wrapScriptModule } = require(path.join(root, 'jsRuntime.ts'))
const { createTrustStore, UI_CUSTOMIZATION_TRUST_KEY } = require(path.join(root, 'trust.ts'))
const { createScriptController } = require(path.join(root, 'scripts.ts'))
const { createUICustomizationRuntime } = require(path.join(root, 'cssRuntime.ts'))
const { resolveCustomizationContext } = require(path.join(root, 'resolver.ts'))
const { surfaceCoveredByGlobal, UI_SURFACES } = require(path.join(root, 'surfaces.ts'))
const { buildPackageDocument, buildBundleDocument, readTransferFile } = require(path.join(root, 'transfer.ts'))

// Scripts report into this log through a global the test owns.
const log = []
global.__scLog = log
const tick = async () => { for (let i = 0; i < 8; i++) await Promise.resolve() }

function createHost() {
  const listeners = new Map()
  const errors = []
  const notices = []
  const host = {
    ctx: { surface: 'chat', worldId: 'A', channelId: 'c', observer: false, palette: 'night' },
    context: () => host.ctx,
    subscribe(event, handler) {
      if (!listeners.has(event)) listeners.set(event, new Set())
      listeners.get(event).add(handler)
      return () => listeners.get(event).delete(handler)
    },
    emit(event, payload) { for (const h of [...(listeners.get(event) || [])]) h(payload) },
    count: event => listeners.get(event)?.size || 0,
    navigate: async target => { log.push(`navigate:${target}`) },
    notify: (type, message) => notices.push(`${type}:${message}`),
    // Same module semantics as a Blob import, evaluated in this realm.
    importModule: async source => {
      const module = { exports: {} }
      new Function('module', source.replace(/export\s+default\s+/, 'module.exports.default = '))(module)
      return module.exports
    },
    reportError: (pkg, phase, error) => errors.push(`${pkg.id}:${phase}:${error.message}`),
    errors,
    notices,
  }
  return host
}

const script = (id, body, scopeType = 'platform', jsHash = `${id}-h1`) => ({ id, scopeType, name: id, js: body, jsHash })
const trace = id => `__scLog.push('activate:${id}'); sc.cleanup(() => __scLog.push('dispose:${id}'))`

async function testLifecycle() {
  assert.match(wrapScriptModule('sc.notify("info","x")'), /export default async function activate\(sc\)/)
  assert.equal(wrapScriptModule('export default () => {}'), 'export default () => {}')

  const host = createHost()
  const runtime = createScriptRuntime(host)
  log.length = 0
  await runtime.reconcile([script('p1', trace('p1')), script('w1', trace('w1'), 'world')])
  assert.deepEqual(log, ['activate:p1', 'activate:w1'], 'platform JS activates before world JS')

  // Update: old version disposes before the new one activates.
  log.length = 0
  await runtime.reconcile([script('p1', trace('p1')), script('w1', trace('w1') + ';__scLog.push("v2")', 'world', 'w1-h2')])
  assert.deepEqual(log, ['dispose:w1', 'activate:w1', 'v2'])

  // Unchanged packages are not re-run; removal disposes exactly once, reverse order.
  log.length = 0
  await runtime.reconcile([script('p1', trace('p1')), script('w1', trace('w1') + ';__scLog.push("v2")', 'world', 'w1-h2')])
  assert.deepEqual(log, [])
  await runtime.reconcile([])
  assert.deepEqual(log, ['dispose:w1', 'dispose:p1'])
  await runtime.reconcile([])
  assert.equal(log.filter(x => x === 'dispose:p1').length, 1, 'dispose ran more than once')

  // A throwing package is isolated; its partial registrations are released.
  log.length = 0
  await runtime.reconcile([
    script('bad', `sc.cleanup(() => __scLog.push('bad-partial')); throw new Error('boom')`),
    script('good', trace('good')),
  ])
  assert.deepEqual(log, ['bad-partial', 'activate:good'])
  assert.ok(host.errors.includes('bad:activate:boom'))
  assert.deepEqual(runtime.activeIds(), ['good'])
  // Failed package is not retried until its hash changes.
  log.length = 0
  await runtime.reconcile([script('bad', `throw new Error('boom')`), script('good', trace('good'))])
  assert.deepEqual(log, [])

  // Failing cleanups do not block the rest; returned dispose runs last.
  log.length = 0
  await runtime.reconcile([script('multi', `
    sc.cleanup(() => __scLog.push('c1'))
    sc.cleanup(() => { throw new Error('cleanup-fail') })
    sc.cleanup(() => __scLog.push('c3'))
    return () => __scLog.push('returned')`), script('after', trace('after'))])
  log.length = 0
  await runtime.reconcile([])
  assert.deepEqual(log, ['dispose:after', 'c3', 'c1', 'returned'])
  assert.ok(host.errors.includes('multi:cleanup:cleanup-fail'))

  // Event subscriptions, timers and observers are released on dispose.
  const timers = new Set()
  const observers = new Set()
  host.window = {
    setTimeout: (fn) => { const id = Symbol('t'); timers.add(id); return id },
    clearTimeout: id => timers.delete(id),
    setInterval: (fn) => { const id = Symbol('i'); timers.add(id); return id },
    clearInterval: id => timers.delete(id),
    MutationObserver: class { constructor(cb) { this.cb = cb } observe() { observers.add(this) } disconnect() { observers.delete(this) } },
  }
  const target = { handlers: new Set(), addEventListener(t, h) { this.handlers.add(h) }, removeEventListener(t, h) { this.handlers.delete(h) } }
  global.__scTarget = target
  const runtime2 = createScriptRuntime(host)
  await runtime2.reconcile([script('events', `
    sc.on('message-created', p => __scLog.push('msg:' + p.messageId))
    sc.on('channel-change', () => { throw new Error('handler-fail') })
    sc.listen(__scTarget, 'click', () => {})
    sc.timeout(() => {}, 10); sc.interval(() => {}, 10)
    sc.observe({}, { childList: true }, () => {})
    const ctx = sc.context()
    __scLog.push('ctx:' + ctx.surface + ':' + ctx.worldId + ':' + sc.apiVersion)
    sc.notify('success', 'hello'); sc.navigate('/A/c')`)])
  log.length = 0
  host.emit('message-created', { messageId: 'm1' })
  host.emit('channel-change', {})
  assert.deepEqual(log, ['msg:m1'])
  assert.ok(host.errors.includes('events:event:handler-fail'), 'event handler error not isolated')
  assert.equal(target.handlers.size, 1); assert.equal(timers.size, 2); assert.equal(observers.size, 1)
  assert.ok(host.notices.includes('success:hello'))
  await runtime2.dispose()
  assert.equal(host.count('message-created'), 0, 'event listener leaked')
  assert.equal(target.handlers.size, 0, 'DOM listener leaked')
  assert.equal(timers.size, 0, 'timer leaked')
  assert.equal(observers.size, 0, 'observer leaked')
  await runtime2.reconcile([script('late', trace('late'))])
  assert.deepEqual(runtime2.activeIds(), [], 'disposed runtime activated a script')

  // dispose during an async activate: late cleanup + returned dispose still run.
  const runtime3 = createScriptRuntime(host)
  global.__scGate = null
  const pending = runtime3.reconcile([script('slow', `
    await new Promise(r => { __scGate = r })
    sc.cleanup(() => __scLog.push('late-cleanup'))
    return () => __scLog.push('late-return')`)])
  await tick()
  log.length = 0
  const disposing = runtime3.dispose()
  global.__scGate()
  await pending; await disposing; await tick()
  assert.deepEqual(log.sort(), ['late-cleanup', 'late-return'])
}

function memoryStorage() {
  const data = new Map()
  return { getItem: k => data.has(k) ? data.get(k) : null, setItem: (k, v) => data.set(k, String(v)), data }
}

async function testTrustAndController() {
  const storage = memoryStorage()
  const trust = createTrustStore(storage, () => 42)
  const host = createHost()
  const runtime = createScriptRuntime(host)
  let pending = []
  let worldId = 'A'
  const controller = createScriptController({ runtime, trust, getWorldId: () => worldId, onPending: p => { pending = p } })
  const pkg = (id, scopeType, js, jsHash, extra = {}) => ({ id, scopeType, name: id, sortOrder: 0, createdAt: 1, css: '', js, jsHash, requiresTrust: scopeType === 'world' && !!js, ...extra })
  log.length = 0
  await controller.apply([pkg('plat', 'platform', trace('plat'), 'ph'), pkg('world', 'world', trace('world'), 'wh1')])
  assert.deepEqual(log, ['activate:plat'], 'untrusted world JS executed')
  assert.deepEqual(pending.map(p => p.id), ['world'])

  await controller.trust(['world'])
  assert.deepEqual(log, ['activate:plat', 'activate:world'])
  assert.deepEqual(pending, [])
  const stored = JSON.parse(storage.data.get(UI_CUSTOMIZATION_TRUST_KEY))
  assert.deepEqual(stored.world, { jsHash: 'wh1', decision: 'trusted', trustedAt: 42, worldId: 'A', name: 'world' })

  // Same package, new hash: stops and returns to untrusted.
  log.length = 0
  await controller.apply([pkg('plat', 'platform', trace('plat'), 'ph'), pkg('world', 'world', trace('world') + ';1', 'wh2')])
  assert.deepEqual(log, ['dispose:world'])
  assert.deepEqual(pending.map(p => p.jsHash), ['wh2'])
  assert.equal(trust.isTrusted('world', 'wh2'), false)
  await controller.trust(['world'])
  assert.deepEqual(log, ['dispose:world', 'activate:world'], 're-trusting the new hash must run it')

  // Server stripped JS (policy closed): nothing runs, nothing pending.
  log.length = 0
  await controller.apply([pkg('plat', 'platform', trace('plat'), 'ph'), pkg('world', 'world', '', '', { requiresTrust: false })])
  assert.deepEqual(log, ['dispose:world'])
  assert.deepEqual(pending, [])

  // Denial persists for that hash and does not prompt again.
  const denied = pkg('other', 'world', trace('other'), 'oh')
  await controller.apply([denied])
  assert.deepEqual(pending.map(p => p.id), ['other'])
  await controller.deny(['other'])
  assert.deepEqual(pending, [])
  await controller.apply([denied])
  assert.deepEqual(pending, [], 'denied script prompted again')
  assert.equal(trust.decisionOf('other', 'oh2'), null, 'denial survived a script change')
  // "Later" hides only for this page session.
  await controller.apply([pkg('later', 'world', trace('later'), 'lh')])
  await controller.postpone(['later'])
  assert.deepEqual(pending, [])
  assert.equal(trust.decisionOf('later', 'lh'), null)
  assert.deepEqual(trust.entriesForWorld('A').map(e => e.id).sort(), ['other', 'world'])
  trust.forget(['other'])
  assert.equal(trust.decisionOf('other', 'oh'), null)
  storage.setItem(UI_CUSTOMIZATION_TRUST_KEY, 'not-json')
  assert.equal(trust.isTrusted('world', 'wh2'), false, 'corrupt storage must fail closed')
  await controller.dispose()
}

function domFixture() {
  const head = { children: [], appendChild(node) { node.remove(); this.children.push(node); node.parent = this; return node } }
  return {
    head, defaultView: {}, documentElement: { dataset: {} },
    createElement: tagName => ({
      tagName, dataset: {}, textContent: '', parent: null,
      remove() { if (this.parent) { this.parent.children.splice(this.parent.children.indexOf(this), 1); this.parent = null } },
    }),
  }
}

async function testRuntimeWiring() {
  const host = createHost()
  const runtime = createScriptRuntime(host)
  const trust = createTrustStore(memoryStorage())
  const ctx = (name, params = {}) => resolveCustomizationContext({ name, params, query: {}, meta: {} }, {
    currentWorldId: 'A', channelId: '', observerMode: false, observerWorldId: '', observerChannelId: '', observerSlug: '',
  })
  let current = ctx('world-channel', { worldId: 'A' })
  const controller = createScriptController({ runtime, trust, getWorldId: () => current.worldId, onPending: () => {} })
  const packagesFor = c => {
    if (c.surface === 'internal') throw new Error('internal must not request')
    // Platform script intentionally has no CSS: interim context switches must
    // preserve JS-only platform packages until the new response is accepted.
    const list = [{ id: 'plat', scopeType: 'platform', name: 'plat', sortOrder: 0, createdAt: 1, css: '', js: trace('plat'), jsHash: 'p' }]
    if (c.surface === 'chat') list.push({ id: `world-${c.worldId}`, scopeType: 'world', name: 'w', sortOrder: 0, createdAt: 1, css: 'b{}', js: trace(`world-${c.worldId}`), jsHash: c.worldId, requiresTrust: true })
    return list
  }
  let gate = null
  const css = createUICustomizationRuntime({
    document: domFixture(),
    loadPackages: async c => { if (gate) await gate.promise; return { ...c, packages: packagesFor(c) } },
    onPackages: (packages, interim) => { void controller.apply(packages, interim) },
  })
  trust.trust([{ id: 'world-A', jsHash: 'A', worldId: 'A', name: 'w' }, { id: 'world-B', jsHash: 'B', worldId: 'B', name: 'w' }])
  log.length = 0
  await css.reconcile(current, false); await tick()
  assert.deepEqual(log, ['activate:plat', 'activate:world-A'])

  // World switch: world A disposes before B's request resolves; platform survives.
  log.length = 0
  let release
  gate = { promise: new Promise(r => { release = r }) }
  current = ctx('world-channel', { worldId: 'B' })
  const switching = css.reconcile(current, false); await tick()
  assert.deepEqual(log, ['dispose:world-A'])
  gate = null; release(); await switching; await tick()
  assert.deepEqual(log, ['dispose:world-A', 'activate:world-B'])

  // Surface switch to a page without the world package disposes it.
  log.length = 0
  current = ctx('world-lobby')
  await css.reconcile(current, false); await tick()
  assert.deepEqual(log, ['dispose:world-B'])

  // A user/login change disposes platform scripts too, before refetching.
  log.length = 0
  gate = { promise: new Promise(r => { release = r }) }
  const relogin = css.reconcile(current, false, 'other-user'); await tick()
  assert.deepEqual(log, ['dispose:plat'], 'platform script survived a user change')
  gate = null; release(); await relogin; await tick()
  assert.deepEqual(log, ['dispose:plat', 'activate:plat'])

  // Safe Mode and internal dispose everything without requests.
  log.length = 0
  await css.reconcile(current, true); await tick()
  assert.deepEqual(log, ['dispose:plat'])
  await css.reconcile({ ...current, surface: 'internal' }, false); await tick()
  assert.deepEqual(runtime.activeIds(), [])
  css.dispose(); await controller.dispose()

  // global covers ordinary surfaces only.
  for (const s of ['observer', 'embed', 'log-public', 'internal', 'global']) assert.equal(surfaceCoveredByGlobal(s), false, s)
  for (const s of UI_SURFACES.filter(s => !['observer', 'embed', 'log-public', 'internal', 'global'].includes(s))) assert.equal(surfaceCoveredByGlobal(s), true, s)
}

async function testTransfer() {
  const full = { id: 'db-id', scopeType: 'world', scopeId: 'W', name: 'Skin', description: 'd', enabled: true, sortOrder: 3, surfaces: ['chat', 'theater'], css: 'a{}', js: 'x()', jsHash: 'h', createdBy: 'u', updatedBy: 'u', createdAt: 1, updatedAt: 2 }
  const doc = buildPackageDocument(full)
  assert.deepEqual(doc, { format: 'sealchat.ui-package', formatVersion: 1, runtimeApiVersion: 2, manifest: { name: 'Skin', description: 'd' }, targets: ['chat', 'theater'], css: 'a{}', js: 'x()', channelCapabilities: [] })
  const text = JSON.stringify(buildBundleDocument([full, full]))
  for (const banned of ['db-id', '"W"', 'scopeId', 'worldId', 'createdBy', 'updatedBy', 'jsHash', 'createdAt', 'updatedAt', 'trust']) {
    assert.ok(!text.includes(banned), `export leaked ${banned}`)
  }
  assert.equal(JSON.parse(text).format, 'sealchat.ui-bundle')
  await assert.rejects(readTransferFile({ text: async () => 'nope' }), /有效的 JSON/)
  await assert.rejects(readTransferFile({ text: async () => '[1]' }), /对象/)
  assert.deepEqual(await readTransferFile({ text: async () => '{"format":"x"}' }), { format: 'x' })
  const { zipSync, strToU8 } = require('fflate')
  const zip = zipSync({
    'a.ui-package.json': strToU8(JSON.stringify(doc)),
    'nested/b.ui-package.json': strToU8(JSON.stringify(doc)),
    'notes.json': strToU8(JSON.stringify({ note: true })),
  })
  const zipped = await readTransferFile({
    name: 'themes.zip',
    text: async () => '',
    arrayBuffer: async () => zip.buffer.slice(zip.byteOffset, zip.byteOffset + zip.byteLength),
    size: zip.byteLength,
  })
  assert.equal(zipped.format, 'sealchat.ui-bundle')
  assert.equal(zipped.packages.length, 2)
  await assert.rejects(readTransferFile({
    name: 'empty.zip',
    text: async () => '',
    arrayBuffer: async () => zipSync({ 'notes.json': strToU8('{"note":true}') }).buffer,
  }), /未找到/)
}

async function run() {
  await testLifecycle()
  await testTrustAndController()
  await testRuntimeWiring()
  await testTransfer()
  console.log('UI customization script runtime, trust and transfer tests passed')
}
run().catch(error => { console.error(error); process.exitCode = 1 })
