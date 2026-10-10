// One continuous PCM16 decoder/resampler, shared with offline fixture tests.
// The server strips the validated WAV header; binary frames carry PCM, not SSE.
export class PCMStream {
  constructor(inputRate, channels, outputRate) {
    if (!Number.isInteger(inputRate) || inputRate < 8000 || inputRate > 96000 || ![1, 2].includes(channels) || outputRate < 8000 || outputRate > 192000) throw new Error('Unsupported PCM format')
    this.rate = inputRate
    this.channels = channels
    this.step = inputRate / outputRate
    this.capacity = inputRate * 3
    this.ring = Array.from({ length: channels }, () => new Float32Array(this.capacity))
    this.read = 0
    this.write = 0
    this.position = 0
    this.pending = new Uint8Array(0)
    this.started = false
    this.ended = false
  }
  push(bytes) {
    if (this.ended) throw new Error('PCM after end')
    const data = new Uint8Array(this.pending.length + bytes.byteLength)
    data.set(this.pending)
    data.set(new Uint8Array(bytes), this.pending.length)
    const frames = Math.floor(data.length / (this.channels * 2))
    if (this.write - this.read + frames > this.capacity) throw new Error('PCM backlog exceeds limit')
    const view = new DataView(data.buffer)
    for (let i = 0; i < frames; i++) {
      for (let ch = 0; ch < this.channels; ch++) this.ring[ch][this.write % this.capacity] = view.getInt16((i * this.channels + ch) * 2, true) / 32768
      this.write++
    }
    this.pending = data.slice(frames * this.channels * 2)
  }
  end() {
    if (this.pending.length) throw new Error('Incomplete PCM sample')
    this.ended = true
  }
  render(length) {
    const output = Array.from({ length: this.channels }, () => new Float32Array(length))
    if (!this.started && (this.write - this.read >= this.rate * 0.3 || this.ended)) this.started = true
    if (!this.started) return { output, done: false, underrun: false }
    for (let i = 0; i < length; i++) {
      const index = Math.floor(this.position)
      if (index >= this.write) return { output, done: this.ended, underrun: !this.ended }
      if (index + 1 >= this.write && !this.ended) return { output, done: false, underrun: true }
      const fraction = this.position - index
      for (let ch = 0; ch < this.channels; ch++) {
        const a = this.ring[ch][index % this.capacity]
        const b = this.ring[ch][Math.min(index + 1, this.write - 1) % this.capacity]
        output[ch][i] = a + (b - a) * fraction
      }
      this.position += this.step
      this.read = Math.min(Math.floor(this.position), this.write)
    }
    return { output, done: this.ended && this.position >= this.write, underrun: false }
  }
}

if (typeof registerProcessor === 'function') {
  class SpeechPCMProcessor extends AudioWorkletProcessor {
    constructor(options) {
      super()
      const { inputRate, channels } = options.processorOptions
      this.decoder = new PCMStream(inputRate, channels, sampleRate)
      this.stopped = false
      this.port.onmessage = ({ data }) => {
        if (this.stopped) return
        try {
          if (data.type === 'audio') this.decoder.push(data.bytes)
          if (data.type === 'end') this.decoder.end()
          if (data.type === 'stop') this.stopped = true
        } catch { this.fail() }
      }
    }
    fail() { this.stopped = true; this.port.postMessage({ type: 'desynced' }) }
    process(inputs, outputs) {
      if (this.stopped) return false
      const result = this.decoder.render(outputs[0][0].length)
      result.output.forEach((channel, i) => outputs[0][i]?.set(channel))
      if (result.underrun) { this.fail(); return false }
      if (result.done) { this.port.postMessage({ type: 'played' }); return false }
      return true
    }
  }
  registerProcessor('sealchat-speech-pcm', SpeechPCMProcessor)
}
