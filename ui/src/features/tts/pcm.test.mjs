import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'

const source = readFileSync(new URL('./pcm-worklet.js', import.meta.url), 'utf8')
const { PCMStream } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

// Deterministic, non-silent 440 Hz PCM fixture; not a supplier/container claim.
function tone(rate, channels, seconds = 1) {
  const bytes = new Uint8Array(rate * channels * 2 * seconds)
  const view = new DataView(bytes.buffer)
  for (let frame = 0; frame < rate * seconds; frame++) for (let ch = 0; ch < channels; ch++) view.setInt16((frame * channels + ch) * 2, Math.round(Math.sin(frame * 2 * Math.PI * 440 / rate) * 16000 * (ch ? -1 : 1)), true)
  return bytes
}
function drain(decoder) {
  const channels = Array.from({ length: decoder.channels }, () => [])
  for (let i = 0; i < 2000; i++) {
    const { output, done, underrun } = decoder.render(128)
    assert.equal(underrun, false)
    output.forEach((data, ch) => channels[ch].push(...data))
    if (done) return channels
  }
  assert.fail('decoder never flushed')
}
test('arbitrary byte boundaries preserve continuous mono and stereo PCM', () => {
  for (const channels of [1, 2]) {
    const bytes = tone(24000, channels)
    const whole = new PCMStream(24000, channels, 48000)
    whole.push(bytes.buffer); whole.end()
    const chunks = new PCMStream(24000, channels, 48000)
    for (let i = 0; i < bytes.length; i += 137) chunks.push(bytes.slice(i, i + 137).buffer)
    chunks.end()
    assert.deepEqual(drain(chunks), drain(whole))
  }
})
test('resampling preserves non-silent waveform and duration at 44.1/48 kHz', () => {
  for (const outputRate of [44100, 48000]) {
    const decoder = new PCMStream(24000, 1, outputRate)
    decoder.push(tone(24000, 1).buffer); decoder.end()
    const output = drain(decoder)[0]
    assert.ok(output.length >= outputRate && output.length < outputRate + 128)
    assert.ok(output.some(sample => Math.abs(sample) > 0.4))
    for (let i = 1; i < outputRate - 2; i++) assert.ok(Math.abs(output[i] - output[i - 1]) < 0.05)
  }
})
test('prebuffer, bounded backlog, truncated frames and fresh utterance state', () => {
  const decoder = new PCMStream(24000, 1, 48000)
  decoder.push(tone(24000, 1, 0.1).buffer)
  assert.equal(decoder.render(128).output[0].every(v => v === 0), true)
  assert.throws(() => decoder.push(tone(24000, 1, 3).buffer))
  const truncated = new PCMStream(24000, 2, 48000)
  truncated.push(new Uint8Array([1]).buffer)
  assert.throws(() => truncated.end())
  const fresh = new PCMStream(24000, 1, 48000)
  fresh.end()
  assert.equal(fresh.render(128).done, true)
})

test('actual worklet stop rejects late packets and does not resume the cancelled utterance', () => {
  let Processor
  class Base {
    constructor() { this.port = { onmessage: null, postMessage() {} } }
  }
  runInNewContext(source.replace('export class PCMStream', 'class PCMStream'), {
    AudioWorkletProcessor: Base, sampleRate: 48000,
    registerProcessor(name, value) { assert.equal(name, 'sealchat-speech-pcm'); Processor = value },
  })
  const processor = new Processor({ processorOptions: { inputRate: 24000, channels: 1 } })
  processor.port.onmessage({ data: { type: 'audio', bytes: tone(24000, 1).buffer } })
  processor.port.onmessage({ data: { type: 'stop' } })
  processor.port.onmessage({ data: { type: 'audio', bytes: tone(24000, 1).buffer } })
  const output = [[new Float32Array(128)]]
  assert.equal(processor.process([], output), false)
  assert.equal(output[0][0].every(v => v === 0), true)
})

test('paced packets decode continuously before input end', () => {
  const decoder = new PCMStream(24000, 1, 48000)
  const bytes = tone(24000, 1)
  decoder.push(bytes.slice(0, 14400).buffer) // 300ms prebuffer
  const received = []
  for (let position = 14400; position < bytes.length; position += 4800) {
    const result = decoder.render(4800)
    assert.equal(result.underrun, false)
    received.push(...result.output[0])
    decoder.push(bytes.slice(position, position + 4800).buffer)
  }
  assert.ok(received.some(value => Math.abs(value) > 0.4))
  decoder.end()
  const result = drain(decoder)[0]
  assert.ok(received.length + result.length >= 48000)
})
