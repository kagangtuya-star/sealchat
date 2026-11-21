// Runs actual Context, iframe transport, SDK, local adapter and script lifecycle.
// node scripts/channel-extension-context.test.cjs
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const Module = require('node:module')
const vm = require('node:vm')
const ts = require('typescript')
const { reactive } = require('vue')
const { MessageChannel } = require('node:worker_threads')
global.MessageChannel = MessageChannel
const listeners = new Map()
const bus = {
  on(name, fn) { const set = listeners.get(name) || new Set(); set.add(fn); listeners.set(name, set) },
  off(name, fn) { listeners.get(name)?.delete(fn) },
  emit(name, value) { for (const fn of [...(listeners.get(name) || [])]) fn(value) },
  count() { return [...listeners.values()].reduce((n, set) => n + set.size, 0) },
}
const windowListeners = new Set()
global.window = {
  location: { href: 'https://sealchat.test/' },
  addEventListener: (_, fn) => windowListeners.add(fn),
  removeEventListener: (_, fn) => windowListeners.delete(fn),
  dispatchEvent() {},
}
const src = path.join(__dirname, '../src')
const originalLoad = Module._load
Module._load = function (request, parent, ...args) {
  if (request === '@/stores/chat') return { chatEvent: bus }
  if (request === '@/composables/useAttachmentResolver') return { resolveAttachmentUrl: value => value || '' }
  if (request === '@/services/embed/channelEmbedImageUpload') return { uploadChannelEmbedImage: async () => ({ attachmentId: 'image' }) }
  return originalLoad.call(this, request, parent, ...args)
}
const originalResolve = Module._resolveFilename
Module._resolveFilename = function (request, parent, ...args) {
  return originalResolve.call(this, request.startsWith('@/') ? path.join(src, request.slice(2)) : request, parent, ...args)
}
require.extensions['.ts'] = (mod, filename) => mod._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText, filename)
const core = require(path.join(src, 'bridge/channelExtensionContext.ts'))
const sessions = []
const createContext = core.createChannelExtensionContext
core.createChannelExtensionContext = deps => {
  const ctx = createContext(deps)
  const open = ctx.openSession
  ctx.openSession = emit => { const session = open(emit); sessions.push({ scope: deps.scope, session }); return session }
  return ctx
}
const { createChannelExtensionClient } = require(path.join(src, 'bridge/channelExtensionClient.ts'))
const { createChannelEmbedHost } = require(path.join(src, 'bridge/channelEmbedHost.ts'))
const { ChannelEmbedClient } = require(path.join(src, 'bridge/channelEmbedSdk.ts'))
const protocol = require(path.join(src, 'bridge/channelEmbedProtocol.ts'))
const { createScriptRuntime } = require(path.join(src, 'services/ui-customization/jsRuntime.ts'))
const { createScriptController } = require(path.join(src, 'services/ui-customization/scripts.ts'))
const { createTrustStore } = require(path.join(src, 'services/ui-customization/trust.ts'))
const tick = () => new Promise(resolve => setImmediate(resolve))
const caps = ['context.read', 'user.read', 'members.read', 'characters.read', 'permissions.read', 'storage.read', 'storage.write', 'events.subscribe', 'events.publish', 'messages.send', 'characterCard.write', 'attachments.upload', 'theater.character.read', 'theater.dialogue.subscribe']

function fixture() {
  const calls = []
  const state = reactive({ permit: true })
  const chat = reactive({
    curChannel: { id: 'C', name: 'channel' }, currentWorldId: 'W', curMember: { userId: 'U' }, observerMode: false, connectState: 'connected',
    activeChannelIdentity: {}, curChannelUsers: [],
    hasChannelPermission: async () => state.permit,
    sendAPI: async (action, params) => { calls.push([action, params]); return { data: action.endsWith('snapshot') ? { seq: 0, documents: [] } : action.endsWith('.get') ? { value: 42 } : { eventId: 'accepted' } } },
    messageCreate: async (...args) => { calls.push(['message.create', args]); return { id: 'message' } },
  })
  const user = reactive({ info: { id: 'U', nick: 'member' }, token: 'token' })
  const deps = { chat, user, worldId: 'W', channelId: 'C', scope: { type: 'ui-customization', id: 'P' }, capabilities: caps, characterCard: { getCharacterApiStatus: () => ({ available: false }), patchActiveCardAttrs: async () => ({ updated: true }) }, characterSnapshot: {} }
  return { deps, calls, state }
}

async function run() {
  // Both public entry points create sessions in the same real Context dispatcher.
  const f = fixture()
  const local = createChannelExtensionClient(f.deps)
  const form = { id: 'F', url: 'https://form.test/', bridgePolicy: { enabled: true, capabilities: caps, allowedOrigins: [] } }
  let iframeClient
  const source = { postMessage(ack, _origin, ports) { if (ack.ok) iframeClient = new ChannelEmbedClient(ports[0], ack.sessionId, ack.contextVersion, ack.capabilities) } }
  const iframe = { src: form.url, contentWindow: source }
  const host = createChannelEmbedHost({ ...f.deps, form, iframe })
  for (const handler of windowListeners) await handler({ source, origin: 'https://form.test', data: { type: protocol.CHANNEL_EMBED_HANDSHAKE, version: 1, nonce: 'nonce-at-least-sixteen' } })
  assert.ok(iframeClient)
  assert.deepEqual(sessions.slice(-2).map(s => s.scope.type).sort(), ['iform', 'ui-customization'])
  assert.deepEqual(await local.user.getCurrent(), await iframeClient.user.getCurrent())
  assert.deepEqual((await local.context.get()).channel, (await iframeClient.context.get()).channel)
  assert.equal(local.capabilities.includes('theater.character.read'), false)
  assert.equal(local.capabilities.includes('theater.dialogue.subscribe'), false, 'absent providers must not advertise capability')
  await local.storage.set('board', 1)
  await iframeClient.storage.set('board', 2)
  const writes = f.calls.filter(([action]) => action === 'channel.extension.storage.set')
  assert.deepEqual(writes.map(([, params]) => params.scope), [{ type: 'ui-customization', id: 'P' }, { type: 'iform', id: 'F' }])
  // The shipped, unchanged JS SDK v1 talks to the refactored iframe host.
  const childListeners = new Set()
  const child = {
    addEventListener: (_, fn) => childListeners.add(fn), removeEventListener: (_, fn) => childListeners.delete(fn), dispatchEvent() {},
    postMessage(data, _origin, ports) { for (const fn of childListeners) fn({ source: child.parent, origin: 'https://sealchat.test', data, ports: ports || [] }) },
  }
  child.parent = { postMessage(data) { for (const fn of windowListeners) void fn({ source: child, origin: 'https://form.test', data }) } }
  vm.runInNewContext(fs.readFileSync(path.join(src, '../../api/embed/channel-embed-sdk.js'), 'utf8'), { window: child, setTimeout, clearTimeout, setInterval, clearInterval })
  const legacyHost = createChannelEmbedHost({ ...f.deps, form, iframe: { src: form.url, contentWindow: child } })
  const legacyClient = await child.SealChatEmbed.connect({ targetOrigin: 'https://sealchat.test' })
  assert.equal((await legacyClient.user.getCurrent()).id, 'U')
  await legacyClient.storage.set('legacy', true)
  await legacyClient.events.publish('legacy', { ok: true })
  await legacyClient.messages.send({ text: 'SDK v1' })
  assert.ok(f.calls.some(([action, args]) => action === 'message.create' && args[3].startsWith('iform_embed:F:')))
  legacyClient.close(); legacyHost.stop()
  await assert.rejects(local.request('storage.set', { key: '' }), error => error.code === 'INVALID_PARAMS')
  const session = sessions.find(s => s.scope.id === 'P').session
  await assert.rejects(session.request('storage.set', { key: 'k', value: 1 }, 0), /CONTEXT_CHANGED/)
  f.state.permit = false
  await assert.rejects(local.messages.send({ text: 'denied' }), error => error.code === 'CAPABILITY_DENIED')
  await assert.rejects(local.events.publish('turn', true), error => error.code === 'CAPABILITY_DENIED')
  await local.storage.set('no-send-permission', { score: 1 })
  assert.equal((await local.storage.get('no-send-permission')).value, 42)
  await local.storage.delete('no-send-permission')
  assert.equal((await local.permissions.getCurrent()).canWriteStorage, true)
  await assert.rejects(iframeClient.storage.set('denied', 1), error => error.code === 'CAPABILITY_DENIED')
  await assert.rejects(iframeClient.storage.delete('denied'), error => error.code === 'CAPABILITY_DENIED')
  assert.equal((await iframeClient.storage.get('board')).value, 42)
  assert.equal((await iframeClient.permissions.getCurrent()).canWriteStorage, false)
  assert.equal((await local.characterCard.updateAttrs({ hp: 1 })).updated, true)
  assert.equal((await local.attachments.uploadImage(new Blob(['image'], { type: 'image/png' }))).attachmentId, 'image')
  f.state.permit = true
  f.deps.chat.observerMode = true
  await assert.rejects(local.storage.set('k', 1), error => error.code === 'CAPABILITY_DENIED')
  f.deps.chat.observerMode = false

  // Scope/topic filtering, metadata value fetch, sequence gaps and reconnect.
  const seen = []
  local.events.on('turn', payload => seen.push(payload))
  const storage = []
  local.storage.onChanged(payload => storage.push(payload))
  await tick()
  const gateway = (scopeType, scopeId, topic, payload, extra = {}) => bus.emit('channel-iform-embed', { channelIFormEmbed: { channelId: 'C', scopeType, scopeId, topic, payload, op: 'event', ...extra } })
  gateway('iform', 'P', 'turn', 'wrong-type')
  gateway('ui-customization', 'other', 'turn', 'wrong-package')
  gateway('ui-customization', 'P', 'other', 'wrong-topic')
  gateway('ui-customization', 'P', 'turn', 'ok')
  assert.deepEqual(seen, ['ok'])
  gateway('ui-customization', 'P', '', null, { op: 'set', seq: 1, key: 'k', revision: 1 })
  await tick()
  assert.equal(storage.at(-1).value, 42)
  f.deps.chat.sendAPI = async (action, params) => { f.calls.push([action, params]); return { data: { seq: 5, documents: [] } } }
  gateway('ui-customization', 'P', '', null, { op: 'set', seq: 5, key: 'k', revision: 2 })
  await tick()
  assert.equal(storage.at(-1).kind, 'resynced')
  bus.emit('connected')
  await tick()
  assert.ok(f.calls.some(([action, params]) => action === 'channel.extension.events.subscribe' && params.topic === 'turn'))
  let closed = 0
  local.onClosed(() => closed++)
  f.deps.chat.curChannel.id = 'different'
  assert.equal(closed, 1, 'channel switch closes synchronously')
  await assert.rejects(local.user.getCurrent(), error => error.code === 'SESSION_EXPIRED')
  host.stop(); iframeClient.close(); local.close()
  assert.equal(bus.count(), 0)
  assert.equal(windowListeners.size, 0)
  for (const mutate of [f => { f.deps.chat.currentWorldId = 'other' }, f => { f.deps.chat.currentWorldId = '' }, f => { f.deps.user.info.id = 'other' }, f => { f.deps.user.token = 'other' }]) {
    const next = fixture(); const client = createChannelExtensionClient(next.deps); let done = false
    client.onClosed(() => { done = true }); mutate(next); assert.equal(done, true); client.close()
  }
  // Optional providers dispose exactly once, and undeclared context never leaks.
  const restricted = fixture(); const restrictedEvents = []; let providerDisposals = 0
  const restrictedContext = createContext({ ...restricted.deps, capabilities: ['storage.read'], theaterCharacterSource: { getSnapshot: () => ({}), subscribe: () => () => providerDisposals++ } })
  const restrictedSession = restrictedContext.openSession(event => restrictedEvents.push(event))
  await assert.rejects(restrictedSession.request('storage.set', { key: 'k', value: 1 }), /CAPABILITY_DENIED/)
  restricted.deps.chat.curChannel.name = 'renamed'; bus.emit('channel-updated')
  await tick()
  assert.equal(restrictedEvents.some(event => event.topic === 'context.changed'), false)
  await assert.rejects(restrictedSession.request('context.get'), /CAPABILITY_DENIED/)
  restrictedContext.dispose(); restrictedContext.dispose(); assert.equal(providerDisposals, 1)
  let dialogueDisposals = 0
  const dialogue = createContext({ ...fixture().deps, capabilities: ['theater.dialogue.subscribe'], theaterDialogueSource: { subscribe: () => () => dialogueDisposals++ } })
  const dialogueSession = dialogue.openSession(() => {})
  await dialogueSession.request('theater.dialogue.subscribe', { identityId: 'I' })
  dialogue.dispose(); dialogueSession.dispose(); dialogue.dispose()
  assert.equal(dialogueDisposals, 1)
  const readOnly = createChannelExtensionClient({ ...fixture().deps, readOnly: () => true })
  await assert.rejects(readOnly.events.publish('turn', true), error => error.code === 'CAPABILITY_DENIED')
  await assert.rejects(readOnly.storage.set('k', 1), error => error.code === 'CAPABILITY_DENIED')
  await assert.rejects(readOnly.storage.delete('k'), error => error.code === 'CAPABILITY_DENIED')
  assert.equal((await readOnly.storage.get('k')).value, 42)
  readOnly.close(); assert.equal(bus.count(), 0)

  const revoked = fixture()
  revoked.deps.chat.sendAPI = async () => { throw new Error('CAPABILITY_DENIED: package is not enabled in this world') }
  const revokedClient = createChannelExtensionClient(revoked.deps)
  await assert.rejects(revokedClient.storage.get('k'), error => error.code === 'CAPABILITY_DENIED' && error.message === 'package is not enabled in this world')
  revokedClient.close(); assert.equal(bus.count(), 0)

  // Script cleanup immediately owns clients, even while activate awaits forever.
  const f2 = fixture()
  let sc
  let release
  const runtime = createScriptRuntime({
    context: () => ({ surface: 'chat', worldId: 'W', channelId: 'C' }), channel: pkg => createChannelExtensionClient({ ...f2.deps, capabilities: pkg.channelCapabilities || [] }),
    importModule: async () => ({ default: api => { sc = api; api.channel(); return new Promise(resolve => { release = resolve }) } }),
    reportError: (_pkg, _phase, err) => { throw err },
  })
  const pkg = { id: 'P', name: 'P', scopeType: 'world', js: 'script', jsHash: 'hash', channelCapabilities: caps }
  const activating = runtime.reconcile([pkg]); await tick()
  const owned = sc.channel(); let ownedClosed = false; owned.onClosed(() => { ownedClosed = true })
  const unloading = runtime.reconcile([])
  assert.equal(ownedClosed, true, 'unload cannot wait on async script activation')
  assert.equal(sc.channel(), null)
  release(); await activating; await unloading; await runtime.dispose()
  assert.equal(bus.count(), 0)

  // Safe-mode/context invalidation closes clients synchronously; absent context is null.
  let available = true
  const apiRuntime = createScriptRuntime({
    context: () => ({ surface: 'chat', worldId: 'W', channelId: 'C' }),
    channel: () => available ? createChannelExtensionClient(fixture().deps) : null,
    importModule: async () => ({ default: api => { sc = api } }), reportError() {},
  })
  await apiRuntime.reconcile([pkg])
  const safeClient = sc.channel(); let safeClosed = false; safeClient.onClosed(() => { safeClosed = true })
  safeClient.onClosed(() => assert.equal(sc.channel(), null, 'close callbacks cannot reopen a Context during invalidation'))
  available = false; apiRuntime.invalidateChannelContexts()
  assert.equal(safeClosed, true); assert.equal(sc.channel(), null)
  await apiRuntime.dispose(); assert.equal(bus.count(), 0)

  // Superseded unloads cannot leave the same package installed but inert.
  let activations = 0
  const quickRuntime = createScriptRuntime({ context: () => ({}), importModule: async () => ({ default: () => { activations++ } }), reportError() {} })
  await quickRuntime.reconcile([pkg])
  const removal = quickRuntime.reconcile([])
  const restoration = quickRuntime.reconcile([pkg])
  await removal; await restoration
  assert.equal(activations, 2); assert.deepEqual(quickRuntime.activeIds(), ['P'])
  await quickRuntime.dispose()

  // Legacy hash trust survives empty declarations; adding/removing abilities invalidates it.
  const memory = new Map()
  const trust = createTrustStore({ getItem: key => memory.get(key), setItem: (key, value) => memory.set(key, value) })
  trust.trust([{ id: 'P', jsHash: 'hash', name: 'P', worldId: 'W' }])
  let pending; let runnable
  const controller = createScriptController({ runtime: { reconcile: async p => { runnable = p }, dispose: async () => {} }, trust, getWorldId: () => 'W', onPending: p => { pending = p } })
  const legacy = { ...pkg, channelCapabilities: [], requiresTrust: true }
  await controller.apply([legacy]); assert.equal(runnable.length, 1)
  await controller.apply([{ ...legacy, channelCapabilities: ['storage.write'] }]); assert.equal(runnable.length, 0); assert.equal(pending.length, 1)
  await controller.trust(['P']); assert.equal(runnable.length, 1)
  await controller.apply([legacy]); assert.equal(runnable.length, 0); assert.equal(pending.length, 1)
  console.log('channel extension Context/SDK/transport/lifecycle/trust regressions passed')
}
run().catch(error => { console.error(error); process.exitCode = 1 })
