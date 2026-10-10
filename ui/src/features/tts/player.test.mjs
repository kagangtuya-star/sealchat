import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'
import * as vue from 'vue'

// Loads the real player with its app imports replaced by small stubs and a fake
// Web Audio graph. It checks completion semantics only, not browser playback.
function compile(file) {
  const source = readFileSync(new URL(file, import.meta.url), 'utf8')
  return ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText
}
const moduleURL = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const runtimeURL = moduleURL(compile('./runtime.ts'))
const stubs = { vue: 'vue', './api': 'api', '@/stores/user': 'user' }
const playerCode = compile('./player.ts').replace(/^import (\{[^}]*\}) from '([^']+)';$/gm, (line, names, from) => {
  if (from === './runtime') return `import ${names} from '${runtimeURL}';`
  return `const ${names.replace(/ as /g, ': ')} = globalThis.__speechTest['${stubs[from]}'];`
}).replaceAll('import.meta.url', JSON.stringify(import.meta.url))

const sources = []
const audio = { ticketFails: false, contextState: 'running', worklet: undefined }
class FakeContext {
  destination = {}
  audioWorklet = { addModule: async () => {} }
  get state() { return audio.contextState }
  async resume() {}
  async decodeAudioData() { return {} }
  createBufferSource() {
    const source = { connect() {}, disconnect() {}, start() {}, stop() {}, onended: null }
    sources.push(source)
    return source
  }
}
class FakeWorklet {
  constructor() { this.port = { postMessage() {}, onmessage: null }; audio.worklet = this }
  connect() {}
  disconnect() {}
}
globalThis.window = globalThis
globalThis.parent = globalThis
globalThis.localStorage = { getItem: () => null, setItem() {} }
globalThis.AudioContext = FakeContext
globalThis.AudioWorkletNode = FakeWorklet
globalThis.__speechTest = {
  vue,
  user: { useUserStore: () => ({ info: { id: 'u' } }) },
  api: {
    speechError: error => error instanceof Error ? error.message : 'error',
    speechAPI: {
      async ticket() { if (audio.ticketFails) throw new Error('ticket'); return 'url' },
      async audio() { return { arrayBuffer: async () => new ArrayBuffer(8) } },
    },
  },
}
const { speechPlayer } = await import(moduleURL(playerCode))
const settle = () => new Promise(resolve => setTimeout(resolve, 0))
function recorder() {
  const results = []
  return { results, onEnd: result => results.push(result) }
}
async function reset() {
  speechPlayer.stop()
  await settle()
  Object.assign(audio, { ticketFails: false, contextState: 'running' })
  speechPlayer.state.preferred = true
  speechPlayer.state.automatic = true
  speechPlayer.state.error = ''
}
const pcm = { epoch: 1, messageId: 'm', media: { codec: 'pcm_s16le', container: 'wav', sampleRate: 24000, channelCount: 1 } }

test('file playback reports played once, only on natural end', async () => {
  await reset()
  const { results, onEnd } = recorder()
  await speechPlayer.play('messages', 'a', true, onEnd)
  assert.equal(speechPlayer.state.playing, true)
  sources.at(-1).onended()
  await settle()
  assert.deepEqual(results, ['played'])
  assert.equal(speechPlayer.state.key, '')
  sources.at(-1).onended?.()
  await settle()
  assert.deepEqual(results, ['played'])
})

test('stop and a replacing playback report stopped, never played', async () => {
  await reset()
  const first = recorder()
  await speechPlayer.play('messages', 'a', true, first.onEnd)
  const stale = sources.at(-1)
  const second = recorder()
  await speechPlayer.play('messages', 'b', true, second.onEnd)
  await settle()
  assert.deepEqual(first.results, ['stopped'])
  assert.equal(stale.onended, null)
  speechPlayer.stop()
  await settle()
  assert.deepEqual(second.results, ['stopped'])
})

test('ticket failure reports failed', async () => {
  await reset()
  audio.ticketFails = true
  const { results, onEnd } = recorder()
  await speechPlayer.play('messages', 'a', true, onEnd)
  await settle()
  assert.deepEqual(results, ['failed'])
  assert.equal(speechPlayer.state.error, 'ticket')
  assert.equal(speechPlayer.state.automatic, true)
})

test('manual replay pauses automatic; natural end, stop and failure all resume it', async () => {
  await reset()
  await speechPlayer.play('resources', 'r')
  assert.equal(speechPlayer.state.automatic, false)
  sources.at(-1).onended()
  await settle()
  assert.equal(speechPlayer.state.automatic, true)

  await speechPlayer.play('resources', 'r')
  await speechPlayer.play('resources', 'r') // Clicking the playing item stops it.
  await settle()
  assert.equal(speechPlayer.state.key, '')
  assert.equal(speechPlayer.state.automatic, true)

  audio.ticketFails = true
  await speechPlayer.play('resources', 'r')
  await settle()
  assert.equal(speechPlayer.state.automatic, true)
})

test('an explicit off preference and a still-locked context keep automatic off', async () => {
  await reset()
  speechPlayer.state.preferred = false
  await speechPlayer.play('resources', 'r')
  sources.at(-1).onended()
  await settle()
  assert.equal(speechPlayer.state.automatic, false)

  await reset()
  await speechPlayer.play('resources', 'r')
  audio.contextState = 'suspended'
  sources.at(-1).onended()
  await settle()
  assert.equal(speechPlayer.state.preferred, true)
  assert.equal(speechPlayer.state.automatic, false)
})

test('stopping an automatic item by click does not pause automatic listening', async () => {
  await reset()
  const { results, onEnd } = recorder()
  await speechPlayer.play('messages', 'a', true, onEnd)
  await speechPlayer.play('messages', 'a')
  await settle()
  assert.deepEqual(results, ['stopped'])
  assert.equal(speechPlayer.state.automatic, true)
})

test('realtime playback distinguishes speaker completion from desync', async () => {
  await reset()
  const played = recorder()
  await speechPlayer.startPCM(pcm, played.onEnd)
  assert.equal(speechPlayer.state.playing, true)
  audio.worklet.port.onmessage({ data: { type: 'played' } })
  await settle()
  assert.deepEqual(played.results, ['played'])

  const broken = recorder()
  await speechPlayer.startPCM({ ...pcm, epoch: 2 }, broken.onEnd)
  audio.worklet.port.onmessage({ data: { type: 'desynced' } })
  await settle()
  assert.deepEqual(broken.results, ['failed'])

  const paused = recorder()
  speechPlayer.state.automatic = false
  await speechPlayer.startPCM({ ...pcm, epoch: 3 }, paused.onEnd)
  await settle()
  assert.deepEqual(paused.results, ['failed'])
})
