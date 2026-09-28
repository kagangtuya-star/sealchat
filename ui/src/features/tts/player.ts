import { reactive } from 'vue'
import { speechAPI, speechError } from './api'
import { useUserStore } from '@/stores/user'
import { SpeechEpoch } from './runtime'

// One coordinator per main application. It never creates synthesis jobs.
// Unset preference defaults to on; only an explicit user "false" keeps it off.
function storedPreference() {
  try {
    const value = localStorage.getItem('sealchat.tts.autoPlayback')
    return value === null ? true : value === 'true'
  } catch { return true }
}
const state = reactive({ key: '', loading: false, playing: false, error: '', automatic: false, preferred: storedPreference() })
let context: AudioContext | undefined
let source: AudioBufferSourceNode | undefined
const epoch = new SpeechEpoch()
let request: AbortController | undefined
let worklet: AudioWorkletNode | undefined
let workletLoaded: Promise<void> | undefined
let liveEpoch = -1
let liveSequence = 0
let pendingPCM: ArrayBuffer[] = []
let pendingBytes = 0
let inputEnded = false

function stop() {
  epoch.invalidate()
  request?.abort()
  request = undefined
  if (worklet) { worklet.port.postMessage({ type: 'stop' }); worklet.port.onmessage = null; worklet.disconnect(); worklet = undefined }
  liveEpoch = -1
  pendingPCM = []; pendingBytes = 0; inputEnded = false
  if (source) { source.onended = null; source.stop(); source.disconnect(); source = undefined }
  state.key = ''
  state.loading = false
  state.playing = false
}

async function startPCM(value: { epoch: number; messageId: string; media: { codec: string; container: string; sampleRate: number; channelCount: number } }) {
  if (!state.automatic || window.parent !== window) return
  stop()
  if (value.media.codec !== 'pcm_s16le' || value.media.container !== 'wav') { state.error = '实时音频格式不受支持'; return }
  const current = epoch.capture()
  liveEpoch = value.epoch; liveSequence = 0
  state.key = `messages:${value.messageId}`; state.loading = true; state.error = ''
  try {
    await unlock()
    if (!epoch.current(current)) return
    if (!context!.audioWorklet) throw new Error('当前环境不支持 AudioWorklet，请关闭自动播放并手动重放文件。')
    workletLoaded ??= context!.audioWorklet.addModule(new URL('./pcm-worklet.js', import.meta.url).href).catch(error => { workletLoaded = undefined; throw error })
    await workletLoaded
    if (!epoch.current(current)) return
    worklet = new AudioWorkletNode(context!, 'sealchat-speech-pcm', { numberOfInputs: 0, numberOfOutputs: 1, outputChannelCount: [value.media.channelCount], processorOptions: { inputRate: value.media.sampleRate, channels: value.media.channelCount } })
    worklet.port.onmessage = ({ data }) => {
      if (!epoch.current(current)) return
      if (data.type === 'played') stop()
      if (data.type === 'desynced') { stop(); state.error = '实时语音已不同步，等待下一段；不会从头自动重播。' }
    }
    worklet.connect(context!.destination)
    for (const bytes of pendingPCM) worklet.port.postMessage({ type: 'audio', bytes }, [bytes])
    pendingPCM = []; pendingBytes = 0
    if (inputEnded) worklet.port.postMessage({ type: 'end' })
    state.loading = false; state.playing = true
  } catch (error) { if (epoch.current(current)) { stop(); state.error = speechError(error) } }
}

function pcmPacket(packet: ArrayBuffer) {
  if (packet.byteLength < 8 || liveEpoch < 0) return
  const view = new DataView(packet)
  if (view.getUint32(0) !== liveEpoch) return
  if (view.getUint32(4) !== liveSequence++ || inputEnded) { stop(); state.error = '实时语音包不连续，等待下一段。'; return }
  const bytes = packet.slice(8)
  if (worklet) worklet.port.postMessage({ type: 'audio', bytes }, [bytes])
  else {
    pendingBytes += bytes.byteLength
    if (pendingBytes > 384000) { stop(); state.error = '实时解码器加载过慢，已跳过本段。'; return }
    pendingPCM.push(bytes)
  }
}
function endPCM(value: number) {
  if (value !== liveEpoch) return
  inputEnded = true
  worklet?.port.postMessage({ type: 'end' })
}

async function unlock() {
  context ??= new AudioContext()
  await context.resume()
}
async function setAutomatic(enabled: boolean) {
  const current = epoch.capture()
  if (enabled) {
    await unlock()
    if (!epoch.current(current)) return
  }
  state.preferred = enabled
  try { localStorage.setItem('sealchat.tts.autoPlayback', String(enabled)) } catch { /* Storage is optional. */ }
  state.automatic = enabled
  if (!enabled) stop()
}
// Silently unlocks audio on the first page gesture when the stored preference is on.
// It never changes the preference; an explicit user "off" is respected.
let resuming: Promise<void> | undefined
function resumePreferred() {
  if (!state.preferred || state.automatic) return Promise.resolve()
  // Manual replay pauses automatic listening; resume only after it finishes.
  if (state.loading || state.playing) return Promise.resolve()
  resuming ??= (async () => {
    try {
      await unlock()
      if (!state.preferred || state.automatic || state.loading || state.playing) return
      state.automatic = true
      state.error = ''
    } catch { /* Stay pending; the next gesture retries. */ }
    finally { resuming = undefined }
  })()
  return resuming
}
async function play(kind: 'messages' | 'resources', id: string, automatic = false) {
  if (window.parent !== window) {
    window.parent.postMessage({ type: 'sealchat:tts-intent', action: 'play', kind, id, userId: useUserStore().info.id }, window.location.origin)
    return
  }
  if (!automatic) state.automatic = false
  const key = `${kind}:${id}`
  if (state.key === key) { stop(); return }
  stop()
  const current = epoch.capture()
  const controller = new AbortController()
  request = controller
  state.key = key
  state.error = ''
  state.loading = true
  try {
    await unlock()
    const url = await speechAPI.ticket(kind, id)
    if (!epoch.current(current)) return
    const blob = await speechAPI.audio(url, controller.signal)
    if (!epoch.current(current)) return
    // Only complete authorized files reach decodeAudioData; never SSE fragments.
    const decoded = await context!.decodeAudioData(await blob.arrayBuffer())
    if (!epoch.current(current)) return
    source = context!.createBufferSource()
    source.buffer = decoded
    source.connect(context!.destination)
    source.onended = () => { if (epoch.current(current)) stop() }
    source.start()
    state.loading = false
    state.playing = true
  } catch (error) {
    if (!epoch.current(current)) return
    stop()
    state.error = speechError(error)
  }
}

export const speechPlayer = { state, play, stop, unlock, setAutomatic, resumePreferred, startPCM, pcmPacket, endPCM }
