import { reactive } from 'vue'
import { speechAPI, speechError } from './api'
import { useUserStore } from '@/stores/user'
import { SpeechEpoch, type SpeechEnd } from './runtime'

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
// Completion of the playback that is current now; reported once, after reset.
let finish: ((result: SpeechEnd) => void) | undefined

function stop(result: SpeechEnd = 'stopped') {
  const done = finish
  finish = undefined
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
  if (done) queueMicrotask(() => done(result))
}

async function startPCM(value: { epoch: number; messageId: string; media: { codec: string; container: string; sampleRate: number; channelCount: number } }, onEnd?: (result: SpeechEnd) => void) {
  if (window.parent !== window) return
  if (!state.automatic) { if (onEnd) queueMicrotask(() => onEnd('failed')); return }
  stop()
  finish = onEnd
  if (value.media.codec !== 'pcm_s16le' || value.media.container !== 'wav') { stop('failed'); state.error = '实时音频格式不受支持'; return }
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
      if (data.type === 'played') stop('played')
      if (data.type === 'desynced') { stop('failed'); state.error = '实时语音已不同步，将在完整文件就绪后播放。' }
    }
    worklet.connect(context!.destination)
    for (const bytes of pendingPCM) worklet.port.postMessage({ type: 'audio', bytes }, [bytes])
    pendingPCM = []; pendingBytes = 0
    if (inputEnded) worklet.port.postMessage({ type: 'end' })
    state.loading = false; state.playing = true
  } catch (error) { if (epoch.current(current)) { stop('failed'); state.error = speechError(error) } }
}

function pcmPacket(packet: ArrayBuffer) {
  if (packet.byteLength < 8 || liveEpoch < 0) return
  const view = new DataView(packet)
  if (view.getUint32(0) !== liveEpoch) return
  if (view.getUint32(4) !== liveSequence++ || inputEnded) { stop('failed'); state.error = '实时语音包不连续，将在完整文件就绪后播放。'; return }
  const bytes = packet.slice(8)
  if (worklet) worklet.port.postMessage({ type: 'audio', bytes }, [bytes])
  else {
    pendingBytes += bytes.byteLength
    if (pendingBytes > 384000) { stop('failed'); state.error = '实时解码器加载过慢，将在完整文件就绪后播放。'; return }
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
  // Some browsers resolve without a user gesture yet stay suspended.
  if (context.state !== 'running') throw new Error('浏览器尚未允许播放音频，请点击页面后重试。')
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
async function play(kind: 'messages' | 'resources', id: string, automatic = false, onEnd?: (result: SpeechEnd) => void) {
  if (window.parent !== window) {
    window.parent.postMessage({ type: 'sealchat:tts-intent', action: 'play', kind, id, userId: useUserStore().info.id }, window.location.origin)
    return
  }
  const key = `${kind}:${id}`
  // Clicking the playing item stops it; that never changes automatic listening.
  if (!automatic && state.key === key) { stop(); return }
  // Manual replay only pauses automatic listening. Every end (natural, stop,
  // ticket/download/decode failure) resumes the preference; resumePreferred
  // keeps it paused while another playback is loading or playing.
  if (!automatic) state.automatic = false
  stop()
  finish = (result) => {
    onEnd?.(result)
    if (!automatic) void resumePreferred()
  }
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
    source.onended = () => { if (epoch.current(current)) stop('played') }
    source.start()
    state.loading = false
    state.playing = true
  } catch (error) {
    if (!epoch.current(current)) return
    stop('failed')
    state.error = speechError(error)
  }
}

export const speechPlayer = { state, play, stop, unlock, setAutomatic, resumePreferred, startPCM, pcmPacket, endPCM }
