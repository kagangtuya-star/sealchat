import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'
import * as vue from 'vue'
import { parse, compileScript } from '@vue/compiler-sfc'

// Runs the real SpeechHost setup() against stub stores, API and player to check
// subscription lifecycle and queue wiring; no DOM, browser audio or server.
const moduleURL = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const transpile = source => ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText
const runtimeURL = moduleURL(transpile(readFileSync(new URL('./runtime.ts', import.meta.url), 'utf8')))
const { descriptor } = parse(readFileSync(new URL('./SpeechHost.vue', import.meta.url), 'utf8'))
const stubs = { vue: 'vue', '@/stores/user': 'user', '@/stores/chat': 'chat', './store': 'store', './player': 'player', './api': 'api', '@/stores/_config': 'config' }
const hostCode = transpile(compileScript(descriptor, { id: 'speech-host', inlineTemplate: true }).content)
  .replace(/^import (\{[^}]*\}) from ['"]([^'"]+)['"];$/gm, (line, names, from) => {
    if (from === './runtime') return `import ${names} from '${runtimeURL}';`
    assert.ok(stubs[from], `unexpected import ${from}`)
    return `const ${names.replace(/ as /g, ': ')} = globalThis.__hostTest['${stubs[from]}'];`
  })

const sockets = []
class FakeSocket {
  constructor(url) { this.url = String(url); this.closed = false; sockets.push(this) }
  close() { this.closed = true }
  open() { this.onopen?.() }
  frame(value) { this.onmessage?.({ data: JSON.stringify(value) }) }
  drop() { this.closed = true; this.onclose?.() }
}
const calls = { tickets: [], queue: [], play: [], pcm: [], stops: 0 }
const serverQueue = { items: [] }
const messageStates = {}
const user = vue.reactive({ info: { id: 'u' } })
const chat = vue.reactive({ curChannel: { id: 'c1' } })
const speech = vue.reactive({ scopeChannel: '', messageStates: {}, visible: false, temporary: {}, quota: null, reset() {}, async refresh() {}, open() {}, setTemporary() {} })
const state = vue.reactive({ preferred: true, automatic: true, loading: false, playing: false, key: '', error: '' })
const player = {
  state,
  play(kind, id, automatic, onEnd) { calls.play.push({ id, automatic, onEnd }); state.key = `messages:${id}`; state.playing = true },
  startPCM(value, onEnd) { calls.pcm.push({ id: value.messageId, onEnd }); state.key = `messages:${value.messageId}`; state.playing = true },
  stop() { calls.stops++; state.key = ''; state.loading = false; state.playing = false },
  endPCM() {}, pcmPacket() {}, async resumePreferred() {},
}
function finish(entry, result) {
  state.key = ''; state.playing = false
  entry.onEnd(result)
}
globalThis.window = globalThis
globalThis.parent = globalThis
globalThis.location = { href: 'http://chat.test/', origin: 'http://chat.test' }
globalThis.addEventListener = () => {}
globalThis.removeEventListener = () => {}
globalThis.document = { querySelectorAll: () => [] }
globalThis.WebSocket = FakeSocket
globalThis.__hostTest = {
  vue,
  user: { useUserStore: () => user },
  chat: { useChatStore: () => chat },
  store: { useSpeechStore: () => speech },
  player: { speechPlayer: player },
  config: { api: { defaults: { baseURL: 'http://chat.test/' } } },
  api: {
    speechAPI: {
      async wsTicket(channelId) { calls.tickets.push(channelId); return { ticket: `t${calls.tickets.length}`, path: 'api/v1/tts/ws' } },
      async queue(channelId) { calls.queue.push(channelId); return serverQueue },
      async message(id) { return messageStates[id] ?? null },
      async states() { return [] },
    },
  },
}
const { default: SpeechHost } = await import(moduleURL(hostCode))
// The stubs resolve at once, so draining microtasks settles every await (and works under mocked timers).
async function settle() { for (let i = 0; i < 50; i++) await Promise.resolve() }
const originalWarn = console.warn
console.warn = () => {} // Lifecycle hooks warn outside a component instance.
let mounted
function mount() {
  mounted = vue.effectScope()
  mounted.run(() => SpeechHost.setup({}, { expose() {}, emit() {}, attrs: {}, slots: {} }))
  // Mounting pauses automatic playback until a gesture; tests model an unlocked page.
  state.automatic = true
  calls.stops = 0
  return mounted
}
test.afterEach(() => mounted?.stop())
test.after(() => { console.warn = originalWarn })
test.beforeEach(() => {
  sockets.length = 0
  Object.assign(calls, { tickets: [], queue: [], play: [], pcm: [], stops: 0 })
  serverQueue.items = []
  for (const key of Object.keys(messageStates)) delete messageStates[key]
  Object.assign(state, { preferred: true, automatic: true, loading: false, playing: false, key: '', error: '' })
  user.info.id = 'u'
  chat.curChannel = { id: 'c1' }
  speech.scopeChannel = ''
})

test('a page mounting with preference, account and channel ready subscribes at once', async () => {
  const scope = mount()
  await settle()
  assert.deepEqual(calls.tickets, ['c1'])
  assert.equal(sockets.length, 1)
  assert.match(sockets[0].url, /^ws:\/\/chat\.test\/api\/v1\/tts\/ws\?ticket=t1&mode=pcm$/)
  sockets[0].open()
  await settle()
  assert.deepEqual(calls.queue, ['c1'])
  scope.stop()
  assert.equal(sockets[0].closed, true)
})

test('nothing subscribes before an account or channel exists, then subscribes once they do', async () => {
  user.info.id = ''
  const scope = mount()
  await settle()
  assert.equal(calls.tickets.length, 0)
  user.info.id = 'u'
  await settle()
  assert.deepEqual(calls.tickets, ['c1'])
  scope.stop()
})

test('an unexpected close reconnects with a new ticket, one socket at a time, and recovers without replays', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const scope = mount()
  await settle()
  sockets[0].open()
  await settle()
  sockets[0].frame({ type: 'start', epoch: 1, messageId: 'a', mode: 'file' })
  finish(calls.play[0], 'played')
  sockets[0].drop()
  assert.match(state.error, /重新连接/)
  t.mock.timers.tick(999)
  assert.equal(calls.tickets.length, 1)
  t.mock.timers.tick(1)
  await settle()
  assert.deepEqual(calls.tickets, ['c1', 'c1'])
  assert.equal(sockets.length, 2)
  assert.match(sockets[1].url, /ticket=t2/)
  // The server still lists a finished message and a successor: only B is new.
  serverQueue.items = [{ id: 'j1', messageId: 'a', status: 'archiving' }, { id: 'j2', messageId: 'b', status: 'running' }]
  messageStates.b = { status: 'ready' }
  sockets[1].open()
  await settle()
  await settle()
  assert.equal(state.error, '')
  assert.deepEqual(calls.play.map(entry => entry.id), ['a', 'b'])
  // A restarted server numbers epochs from 1 again.
  finish(calls.play[1], 'played')
  sockets[1].frame({ type: 'start', epoch: 1, messageId: 'c', mode: 'file' })
  assert.deepEqual(calls.play.map(entry => entry.id), ['a', 'b', 'c'])
  scope.stop()
  t.mock.timers.tick(20000)
  assert.equal(calls.tickets.length, 2)
})

test('a busy page queues successors and drains them in order after natural completion', async () => {
  const scope = mount()
  await settle()
  const socket = sockets[0]
  socket.open()
  await settle()
  socket.frame({ type: 'start', epoch: 1, messageId: 'a', mode: 'pcm', media: { codec: 'pcm_s16le', container: 'wav', sampleRate: 24000, channelCount: 1 } })
  socket.frame({ type: 'start', epoch: 2, messageId: 'b', mode: 'pcm', media: {} })
  socket.frame({ type: 'start', epoch: 3, messageId: 'c', mode: 'pcm', media: {} })
  assert.deepEqual(calls.pcm.map(entry => entry.id), ['a'])
  assert.equal(calls.stops, 0)
  // B's archive becomes ready; A is still speaking and must not be cut off.
  messageStates.b = { status: 'ready' }
  socket.frame({ type: 'start', epoch: 4, messageId: 'b', mode: 'file' })
  await settle()
  assert.equal(calls.play.length, 0)
  finish(calls.pcm[0], 'played')
  await settle()
  assert.deepEqual(calls.play.map(entry => entry.id), ['b'])
  // A duplicate announcement for B while it plays does nothing.
  socket.frame({ type: 'start', epoch: 5, messageId: 'b', mode: 'file' })
  messageStates.c = { status: 'ready' }
  finish(calls.play[0], 'played')
  await settle()
  assert.deepEqual(calls.play.map(entry => entry.id), ['b', 'c'])
  scope.stop()
})

test('a cancel frame dismisses a queued message without stopping the current one', async () => {
  const scope = mount()
  await settle()
  const socket = sockets[0]
  socket.open()
  await settle()
  socket.frame({ type: 'start', epoch: 1, messageId: 'a', mode: 'file' })
  socket.frame({ type: 'start', epoch: 2, messageId: 'b', mode: 'pcm', media: {} })
  socket.frame({ type: 'cancel', epoch: 2 })
  assert.equal(calls.stops, 0)
  messageStates.b = { status: 'ready' }
  finish(calls.play[0], 'played')
  await settle()
  assert.deepEqual(calls.play.map(entry => entry.id), ['a'])
  scope.stop()
})

test('a message replayed by hand while still owed is not played again automatically', async () => {
  const scope = mount()
  await settle()
  const socket = sockets[0]
  socket.open()
  await settle()
  socket.frame({ type: 'start', epoch: 1, messageId: 'a', mode: 'file' })
  socket.frame({ type: 'start', epoch: 2, messageId: 'b', mode: 'pcm', media: {} })
  messageStates.b = { status: 'ready' }
  await settle()
  const auto = calls.play[0]
  player.play('messages', 'b', false, () => {})
  auto.onEnd('stopped')
  await settle()
  state.key = ''; state.playing = false
  await settle()
  assert.deepEqual(calls.play.map(entry => [entry.id, entry.automatic]), [['a', true], ['b', false]])
  scope.stop()
})

test('channel change stops playback, closes the socket and ignores late callbacks', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const scope = mount()
  await settle()
  const old = sockets[0]
  old.open()
  await settle()
  old.frame({ type: 'start', epoch: 1, messageId: 'a', mode: 'file' })
  old.frame({ type: 'start', epoch: 2, messageId: 'b', mode: 'file' })
  chat.curChannel = { id: 'c2' }
  await settle()
  assert.equal(old.closed, true)
  assert.ok(calls.stops > 0)
  assert.deepEqual(calls.tickets, ['c1', 'c2'])
  // Late events from the old channel restart nothing.
  messageStates.b = { status: 'ready' }
  calls.play[0].onEnd('played')
  old.frame({ type: 'start', epoch: 3, messageId: 'x', mode: 'file' })
  old.onclose?.()
  t.mock.timers.tick(20000)
  await settle()
  assert.deepEqual(calls.play.map(entry => entry.id), ['a'])
  assert.deepEqual(calls.tickets, ['c1', 'c2'])
  scope.stop()
})

test('turning the preference off closes the socket and stops reconnecting', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const scope = mount()
  await settle()
  sockets[0].open()
  sockets[0].drop()
  state.preferred = false
  t.mock.timers.tick(20000)
  await settle()
  assert.equal(calls.tickets.length, 1)
  scope.stop()
})
